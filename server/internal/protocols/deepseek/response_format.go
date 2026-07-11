package deepseek

import (
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

// =============================================================================
// FormatUnified — Unified 响应/流 → DeepSeek 客户端格式（OpenAI 兼容）
// =============================================================================

func (p *DeepSeekProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		// 非流式
		usage.InputTokens = resp.Usage.InputTokens
		usage.OutputTokens = resp.Usage.OutputTokens
		usage.CachedTokens = resp.Usage.CachedTokens
		usage.CacheHitTokens = resp.Usage.CacheHitTokens
		usage.CacheMissTokens = resp.Usage.CacheMissTokens

		usageMap := map[string]interface{}{
			"prompt_tokens":     resp.Usage.InputTokens,
			"completion_tokens": resp.Usage.OutputTokens,
			"total_tokens":      resp.Usage.TotalTokens(),
		}
		if resp.Usage.CacheHitTokens > 0 || resp.Usage.CacheMissTokens > 0 {
			usageMap["prompt_cache_hit_tokens"] = resp.Usage.CacheHitTokens
			usageMap["prompt_cache_miss_tokens"] = resp.Usage.CacheMissTokens
		}
		if resp.Usage.CachedTokens > 0 {
			usageMap["prompt_tokens_details"] = map[string]interface{}{
				"cached_tokens": resp.Usage.CachedTokens,
			}
		}

		deepseekResp := map[string]interface{}{
			"id":     resp.ID,
			"object": "chat.completion",
			"model":  resp.Model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       p.buildDeepSeekMessage(resp),
					"finish_reason": resp.FinishReason,
				},
			},
			"usage": usageMap,
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		body, _ := json.Marshal(deepseekResp)
		_, err := c.Writer.Write(body)
		return err
	}

	// 流式：将 reasoning_content 包裹为 <think>...</think>
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var id string
	var thinkStarted bool // 是否已输出 <think> 开头
	var thinkEnded bool   // 是否已输出 </think> 结尾
	for ev := range events {
		switch ev.Type {
		case unified.EventChunk:
			if ev.Delta != nil {
				if id == "" {
					id = "chatcmpl-unified"
				}
				deltaMap := map[string]interface{}{}
				if ev.Delta.Role != "" {
					deltaMap["role"] = ev.Delta.Role
				}

				// 支持原生 reasoning_content
				if ev.Delta.ReasoningContent != "" {
					deltaMap["reasoning_content"] = ev.Delta.ReasoningContent
				}

				// 处理 reasoning_content 混合 content：包裹为 <think>...</think>
				var contentText string
				if ev.Delta.ReasoningContent != "" {
					thinkText := ev.Delta.ReasoningContent
					if !thinkStarted {
						thinkText = "<think>" + thinkText
						thinkStarted = true
					}
					contentText = thinkText
				}
				if ev.Delta.Content != "" {
					if thinkStarted && !thinkEnded {
						contentText += "</think>\n" + ev.Delta.Content
						thinkEnded = true
					} else {
						contentText += ev.Delta.Content
					}
				}
				if contentText != "" {
					deltaMap["content"] = contentText
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
				usage.CacheHitTokens = ev.Usage.CacheHitTokens
				usage.CacheMissTokens = ev.Usage.CacheMissTokens
			}
		case unified.EventDone:
			if thinkStarted && !thinkEnded {
				// 兜底输出闭合标签
				chunk := map[string]interface{}{
					"id":     id,
					"object": "chat.completion.chunk",
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]interface{}{
								"content": "</think>\n",
							},
							"finish_reason": nil,
						},
					},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
				thinkEnded = true
			}

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

func (p *DeepSeekProvider) buildDeepSeekMessage(resp *unified.Response) map[string]interface{} {
	msg := map[string]interface{}{"role": "assistant"}
	content := resp.Content
	// 将 reasoning_content 包裹为 <think>...</think> 前置到 content（兼容标准 OpenAI 客户端）
	if resp.ReasoningContent != "" {
		content = wrapThinkTag(resp.ReasoningContent, content)
	}
	if content != "" {
		msg["content"] = content
	}
	if resp.ReasoningContent != "" {
		msg["reasoning_content"] = resp.ReasoningContent
	}
	if len(resp.ToolCalls) > 0 {
		msg["tool_calls"] = resp.ToolCalls
	}
	return msg
}
