package openai

import (
	"bufio"
	"io"
	"encoding/json"
	"strings"
	"ai-gateway/internal/core/unified"
)

func (p *OpenAIProvider) streamOpenAIToUnified(body io.ReadCloser) <-chan unified.StreamEvent {
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
			if data == "[DONE]" {
				ch <- unified.StreamEvent{Type: unified.EventDone}
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Role             string             `json:"role"`
						Content          string             `json:"content"`
						ReasoningContent string             `json:"reasoning_content"`
						ToolCalls        []unified.ToolCall `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
				Usage *openAIUsageRaw `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if chunk.Usage != nil {
				ch <- unified.StreamEvent{
					Type: unified.EventUsage,
					Usage: &unified.Usage{
						CachedTokens: chunk.Usage.PromptTokensDetails.CachedTokens,
						InputTokens:  chunk.Usage.PromptTokens - chunk.Usage.PromptTokensDetails.CachedTokens,
						OutputTokens: chunk.Usage.CompletionTokens,
					},
				}
			}
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" || len(delta.ToolCalls) > 0 || delta.Role != "" || delta.ReasoningContent != "" {
					ch <- unified.StreamEvent{
						Type: unified.EventChunk,
						Delta: &unified.Delta{
							Role:             delta.Role,
							Content:          delta.Content,
							ReasoningContent: delta.ReasoningContent,
							ToolCalls:        delta.ToolCalls,
						},
					}
				}
				if chunk.Choices[0].FinishReason != "" {
					ch <- unified.StreamEvent{
						Type:         unified.EventDone,
						FinishReason: chunk.Choices[0].FinishReason,
					}
				}
			}
		}
	}()
	return ch
}

