package openai

import (
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// FormatUnified — Unified 响应/流 → OpenAI 客户端格式
// =============================================================================

func (p *OpenAIProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		// 非流式
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
					"message":       p.buildOpenAIMessage(resp),
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

	// 流式
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var id string
	var toolCallIndex int
	var toolCallID string
	var toolCallName string
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
				if ev.Delta.Content != "" {
					deltaMap["content"] = ev.Delta.Content
				}
				if ev.Delta.ReasoningContent != "" {
					deltaMap["reasoning_content"] = ev.Delta.ReasoningContent
				}
				if len(ev.Delta.ToolCalls) > 0 {
					deltaMap["tool_calls"] = ev.Delta.ToolCalls
				}
				// Anthropic→OpenAI: InputJSON 转为 OpenAI 风格的 tool_calls 流式格式
				if ev.Delta.InputJSON != "" {
					// 从 TransformerMetadata 提取 tool_call_id 和 tool_name
					if ev.Delta.TransformerMetadata != nil {
						if tid, ok := ev.Delta.TransformerMetadata["tool_call_id"].(string); ok {
							toolCallID = tid
						}
						if tn, ok := ev.Delta.TransformerMetadata["tool_name"].(string); ok {
							toolCallName = tn
						}
					}
					if toolCallID == "" {
						toolCallID = fmt.Sprintf("call_%d", toolCallIndex)
					}
					if toolCallName == "" {
						toolCallName = "unknown"
					}
					// 第一个 InputJSON chunk：发送完整 tool_call（含 index/id/function.name）
					toolCalls := []map[string]interface{}{{
						"index": toolCallIndex,
						"id":    toolCallID,
						"type":  "function",
						"function": map[string]interface{}{
							"name":      toolCallName,
							"arguments": ev.Delta.InputJSON,
						},
					}}
					deltaMap["tool_calls"] = toolCalls
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

func (p *OpenAIProvider) buildOpenAIMessage(resp *unified.Response) map[string]interface{} {
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
// 内部：解析 OpenAI 响应 → UnifiedResponse
// =============================================================================

type openAIUsageRaw struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}
