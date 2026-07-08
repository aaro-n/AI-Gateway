package gemini

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"ai-gateway/internal/core/reasonmap"
	"ai-gateway/internal/core/unified"
)

// streamGeminiToUnified 将 Gemini SSE 流转换为 Unified StreamEvent channel
func (p *GeminiProvider) streamGeminiToUnified(body io.ReadCloser) <-chan unified.StreamEvent {
	ch := make(chan unified.StreamEvent, 32)
	go func() {
		defer body.Close()
		defer close(ch)
		reader := bufio.NewReader(body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					ch <- unified.StreamEvent{Type: unified.EventError}
				}
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			var chunk struct {
				Candidates []struct {
					Content struct {
						Parts []json.RawMessage `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				} `json:"candidates"`
				UsageMetadata *struct {
					PromptTokenCount     int `json:"promptTokenCount"`
					CandidatesTokenCount int `json:"candidatesTokenCount"`
				} `json:"usageMetadata"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if chunk.UsageMetadata != nil {
				ch <- unified.StreamEvent{
					Type: unified.EventUsage,
					Usage: &unified.Usage{
						InputTokens:  chunk.UsageMetadata.PromptTokenCount,
						OutputTokens: chunk.UsageMetadata.CandidatesTokenCount,
					},
				}
			}

			if len(chunk.Candidates) > 0 {
				var hasText bool
				var trailingSig string
				for _, partRaw := range chunk.Candidates[0].Content.Parts {
					var part map[string]interface{}
					if json.Unmarshal(partRaw, &part) != nil {
						continue
					}
					if text, ok := part["text"].(string); ok && text != "" {
						ch <- unified.StreamEvent{Type: unified.EventChunk, Delta: &unified.Delta{Content: text}}
						hasText = true
					}
					if thought, ok := part["thought"].(string); ok && thought != "" {
						ch <- unified.StreamEvent{Type: unified.EventChunk, Delta: &unified.Delta{ReasoningContent: thought}}
					}
					if sig, ok := part["thoughtSignature"].(string); ok && sig != "" {
						if hasText {
							trailingSig = sig
						} else {
							ch <- unified.StreamEvent{Type: unified.EventChunk, Delta: &unified.Delta{ReasoningSignature: &sig}}
						}
					}
					if fc, ok := part["functionCall"].(map[string]interface{}); ok {
						name, _ := fc["name"].(string)
						argsJSON, _ := json.Marshal(fc["args"])
						ch <- unified.StreamEvent{
							Type: unified.EventChunk,
							Delta: &unified.Delta{
								ToolCalls: []unified.ToolCall{{
									ID:       fmt.Sprintf("call_%s", name),
									Type:     "function",
									Function: unified.FunctionCall{Name: name, Arguments: string(argsJSON)},
								}},
							},
						}
					}
				}
				if trailingSig != "" {
					ch <- unified.StreamEvent{Type: unified.EventChunk, Delta: &unified.Delta{ReasoningSignature: &trailingSig}}
				}
				if chunk.Candidates[0].FinishReason != "" {
					ch <- unified.StreamEvent{
						Type:         unified.EventDone,
						FinishReason: reasonmap.GeminiToUnified(chunk.Candidates[0].FinishReason),
					}
				}
			}
		}
	}()
	return ch
}
