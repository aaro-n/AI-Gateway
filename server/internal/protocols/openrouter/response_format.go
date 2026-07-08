package openrouter

import (
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/streamutil"
	"ai-gateway/internal/core/unified"
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// FormatUnified — Unified 响应/流 → OpenRouter/OpenAI 客户端格式
// =============================================================================

func (p *OpenRouterProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		usage.InputTokens = resp.Usage.InputTokens
		usage.OutputTokens = resp.Usage.OutputTokens
		usage.CachedTokens = resp.Usage.CachedTokens

		openAIResp := map[string]interface{}{
			"id":     resp.ID,
			"object": "chat.completion",
			"model":  resp.Model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       p.buildOpenRouterMessage(resp),
					"finish_reason": resp.FinishReason,
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     resp.Usage.InputTokens,
				"completion_tokens": resp.Usage.OutputTokens,
				"total_tokens":      resp.Usage.TotalTokens(),
			},
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		body, _ := json.Marshal(openAIResp)
		_, err := c.Writer.Write(body)
		return err
	}

	// 流式 SSE
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var id string
	for ev := range events {
		switch ev.Type {
		case unified.EventChunk:
			if ev.Delta != nil {
				if id == "" {
					id = "chatcmpl-openrouter"
				}
				deltaMap := map[string]interface{}{}
				if ev.Delta.Role != "" {
					deltaMap["role"] = ev.Delta.Role
				}
				if ev.Delta.Content != "" {
					deltaMap["content"] = ev.Delta.Content
				}
				if ev.Delta.ReasoningContent != "" {
					deltaMap["reasoning_content"] = ev.Delta.ReasoningContent
				}
				if len(ev.Delta.ToolCalls) > 0 {
					deltaMap["tool_calls"] = ev.Delta.ToolCalls
				}
				chunk := map[string]interface{}{
					"id":     id,
					"object": "chat.completion.chunk",
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"delta":         deltaMap,
							"finish_reason": nil,
						},
					},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
			}
		case unified.EventUsage:
			if ev.Usage != nil {
				usage.InputTokens = ev.Usage.InputTokens
				usage.OutputTokens = ev.Usage.OutputTokens
				usage.CachedTokens = ev.Usage.CachedTokens
			}
		case unified.EventDone:
			chunk := map[string]interface{}{
				"id":     id,
				"object": "chat.completion.chunk",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": ev.FinishReason,
					},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
			c.Writer.Flush()
		}
	}
	return nil
}

func (p *OpenRouterProvider) buildOpenRouterMessage(resp *unified.Response) map[string]interface{} {
	msg := map[string]interface{}{"role": "assistant"}
	if resp.Content != "" {
		msg["content"] = resp.Content
	}
	if resp.ReasoningContent != "" {
		msg["reasoning_content"] = resp.ReasoningContent
	}
	if len(resp.ToolCalls) > 0 {
		msg["tool_calls"] = resp.ToolCalls
	}
	return msg
}

// =============================================================================
// 内部：解析 OpenRouter 响应 → UnifiedResponse
// =============================================================================

type openRouterUsageRaw struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (p *OpenRouterProvider) parseOpenRouterResponse(body []byte) (*unified.Response, error) {
	var raw struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role             string             `json:"role"`
				Content          string             `json:"content"`
				ReasoningContent string             `json:"reasoning_content"`
				ToolCalls        []unified.ToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage openRouterUsageRaw `json:"usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	uresp := &unified.Response{
		ID:    raw.ID,
		Model: raw.Model,
		Usage: unified.Usage{
			CachedTokens: raw.Usage.PromptTokensDetails.CachedTokens,
			InputTokens:  raw.Usage.PromptTokens - raw.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens: raw.Usage.CompletionTokens,
		},
	}
	if len(raw.Choices) > 0 {
		uresp.Content = raw.Choices[0].Message.Content
		uresp.ReasoningContent = raw.Choices[0].Message.ReasoningContent
		uresp.ToolCalls = raw.Choices[0].Message.ToolCalls
		uresp.FinishReason = raw.Choices[0].FinishReason
	}
	return uresp, nil
}

// =============================================================================
// 流式：OpenRouter SSE → unified.StreamEvent chan
// =============================================================================

func (p *OpenRouterProvider) streamOpenRouterToUnified(ctx context.Context, body io.ReadCloser) <-chan unified.StreamEvent {
	ch := make(chan unified.StreamEvent, streamutil.BufferSize)
	go func() {
		defer body.Close()
		defer close(ch)
		reader := bufio.NewReader(body)
		for {
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
				streamutil.SendEvent(ctx, ch, unified.StreamEvent{Type: unified.EventDone, FinishReason: "stop"})
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
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
				Usage *openRouterUsageRaw `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			// 发送 usage 事件
			if chunk.Usage != nil {
				if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
					Type: unified.EventUsage,
					Usage: &unified.Usage{
						CachedTokens: chunk.Usage.PromptTokensDetails.CachedTokens,
						InputTokens:  chunk.Usage.PromptTokens - chunk.Usage.PromptTokensDetails.CachedTokens,
						OutputTokens: chunk.Usage.CompletionTokens,
					},
				}) {
					return
				}
			}

			// 发送 delta 事件
			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				delta := &unified.Delta{
					Role:             choice.Delta.Role,
					Content:          choice.Delta.Content,
					ReasoningContent: choice.Delta.ReasoningContent,
					ToolCalls:        choice.Delta.ToolCalls,
				}
				ev := unified.StreamEvent{Type: unified.EventChunk, Delta: delta}
				if choice.FinishReason != nil {
					ev.FinishReason = *choice.FinishReason
				}
				if !streamutil.SendEvent(ctx, ch, ev) {
					return
				}
			}
		}
	}()
	return ch
}
