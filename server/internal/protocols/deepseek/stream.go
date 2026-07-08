package deepseek

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"

	"ai-gateway/internal/core/unified"
)

// =============================================================================
// 流式：DeepSeek SSE → unified.StreamEvent chan
// =============================================================================

func (p *DeepSeekProvider) streamDeepSeekToUnified(body io.ReadCloser) <-chan unified.StreamEvent {
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
				Usage *deepseekUsageRaw `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if chunk.Usage != nil {
				ch <- unified.StreamEvent{
					Type: unified.EventUsage,
					Usage: &unified.Usage{
						CachedTokens:    chunk.Usage.PromptTokensDetails.CachedTokens,
						CacheHitTokens:  chunk.Usage.PromptCacheHitTokens,
						CacheMissTokens: chunk.Usage.PromptCacheMissTokens,
						InputTokens:     chunk.Usage.PromptTokens - chunk.Usage.PromptTokensDetails.CachedTokens,
						OutputTokens:    chunk.Usage.CompletionTokens,
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

// deepseekModelDefaults provides known specs for DeepSeek models when the API doesn't return them.
var deepseekModelDefaults = map[string]struct {
	contextWindow int
	maxOutput     int
	vision        bool
}{
	"deepseek-chat":     {131072, 8192, false},
	"deepseek-reasoner": {131072, 8192, false},
	"deepseek-v3":       {131072, 8192, false},
	"deepseek-v4-flash": {131072, 16384, true},
	"deepseek-v4-pro":   {262144, 32768, true},
}

// detectDeepSeekVision checks if a DeepSeek model supports vision based on its ID.
func detectDeepSeekVision(modelID string) bool {
	if def, ok := deepseekModelDefaults[modelID]; ok {
		return def.vision
	}
	return false
}
