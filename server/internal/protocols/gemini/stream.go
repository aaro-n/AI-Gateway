package gemini

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/google/uuid"

	"ai-gateway/internal/core/reasonmap"
	"ai-gateway/internal/core/streamutil"
	"ai-gateway/internal/core/unified"
)

// sessionToolIDMap maintains a session-level mapping from function name to stable tool call ID.
// This ensures tool_result matching works correctly across multi-turn tool conversations.
var sessionToolIDMap = &sync.Map{} // map[string]string: "sessionKey:funcName" → "toolCallID"

// getOrCreateToolCallID returns a stable tool call ID for the given function name within a session.
// If a sessionKey is provided (e.g., request ID), IDs are scoped to that session.
func getOrCreateToolCallID(sessionKey, funcName string) string {
	key := sessionKey + ":" + funcName
	if id, ok := sessionToolIDMap.Load(key); ok {
		return id.(string)
	}
	id := "call_" + uuid.New().String()[:8]
	sessionToolIDMap.Store(key, id)
	return id
}

// streamGeminiToUnified 将 Gemini SSE 流转换为 Unified StreamEvent channel
func (p *GeminiProvider) streamGeminiToUnified(ctx context.Context, body io.ReadCloser) <-chan unified.StreamEvent {
	ch := make(chan unified.StreamEvent, streamutil.BufferSize)
	go func() {
		defer body.Close()
		defer close(ch)
		reader := bufio.NewReader(body)
		for {
			// Check context before blocking read
			if ctx.Err() != nil {
				return
			}
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					streamutil.SendEvent(ctx, ch, unified.StreamEvent{Type: unified.EventError})
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
				log.Printf("[Gemini stream] failed to unmarshal SSE chunk: %v, data=%s", err, streamutil.Truncate(data, 200))
				continue
			}

			if chunk.UsageMetadata != nil {
				if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
					Type: unified.EventUsage,
					Usage: &unified.Usage{
						InputTokens:  chunk.UsageMetadata.PromptTokenCount,
						OutputTokens: chunk.UsageMetadata.CandidatesTokenCount,
					},
				}) {
					return
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
						if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{Type: unified.EventChunk, Delta: &unified.Delta{Content: text}}) {
							return
						}
						hasText = true
					}
					if thought, ok := part["thought"].(string); ok && thought != "" {
						if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{Type: unified.EventChunk, Delta: &unified.Delta{ReasoningContent: thought}}) {
							return
						}
					}
					if sig, ok := part["thoughtSignature"].(string); ok && sig != "" {
						if hasText {
							trailingSig = sig
						} else {
							if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{Type: unified.EventChunk, Delta: &unified.Delta{ReasoningSignature: &sig}}) {
								return
							}
						}
					}
					if fc, ok := part["functionCall"].(map[string]interface{}); ok {
						name, _ := fc["name"].(string)
						argsJSON, _ := json.Marshal(fc["args"])
						// Use session-level stable tool call ID for proper multi-turn tool matching
						toolCallID := getOrCreateToolCallID("", name)
						if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
							Type: unified.EventChunk,
							Delta: &unified.Delta{
								ToolCalls: []unified.ToolCall{{
									ID:       toolCallID,
									Type:     "function",
									Function: unified.FunctionCall{Name: name, Arguments: string(argsJSON)},
								}},
							},
						}) {
							return
						}
					}
				}
				if trailingSig != "" {
					if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{Type: unified.EventChunk, Delta: &unified.Delta{ReasoningSignature: &trailingSig}}) {
						return
					}
				}
				if chunk.Candidates[0].FinishReason != "" {
					streamutil.SendEvent(ctx, ch, unified.StreamEvent{
						Type:         unified.EventDone,
						FinishReason: reasonmap.GeminiToUnified(chunk.Candidates[0].FinishReason),
					})
				}
			}
		}
	}()
	return ch
}
