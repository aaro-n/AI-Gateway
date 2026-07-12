package deepseek

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"strings"

	"ai-gateway/internal/core/streamutil"
	"ai-gateway/internal/core/unified"
)

// =============================================================================
// 流式：DeepSeek SSE → unified.StreamEvent chan
// =============================================================================

func (p *DeepSeekProvider) streamDeepSeekToUnified(ctx context.Context, body io.ReadCloser) <-chan unified.StreamEvent {
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
			if data == "[DONE]" {
				streamutil.SendEvent(ctx, ch, unified.StreamEvent{Type: unified.EventDone})
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
				log.Printf("[DeepSeek stream] failed to unmarshal SSE chunk: %v, data=%s", err, streamutil.Truncate(data, 200))
				continue
			}
			if chunk.Usage != nil {
				if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
					Type: unified.EventUsage,
					Usage: &unified.Usage{
						CachedTokens:    chunk.Usage.PromptTokensDetails.CachedTokens,
						CacheHitTokens:  chunk.Usage.PromptCacheHitTokens,
						CacheMissTokens: chunk.Usage.PromptCacheMissTokens,
						InputTokens:     chunk.Usage.PromptTokens - chunk.Usage.PromptTokensDetails.CachedTokens,
						OutputTokens:    chunk.Usage.CompletionTokens,
					},
				}) {
					return
				}
			}
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" || len(delta.ToolCalls) > 0 || delta.Role != "" || delta.ReasoningContent != "" {
					if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
						Type: unified.EventChunk,
						Delta: &unified.Delta{
							Role:             delta.Role,
							Content:          delta.Content,
							ReasoningContent: delta.ReasoningContent,
							ToolCalls:        delta.ToolCalls,
						},
					}) {
						return
					}
				}
				if chunk.Choices[0].FinishReason != "" {
					if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
						Type:         unified.EventDone,
						FinishReason: chunk.Choices[0].FinishReason,
					}) {
						return
					}
				}
			}
		}
	}()
	return ch
}
