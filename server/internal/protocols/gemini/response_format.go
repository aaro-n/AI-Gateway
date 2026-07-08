package gemini

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/reasonmap"
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
)

// FormatUnified 将 Unified 响应/流格式化为 Gemini 客户端格式

// accumulatedToolCall 跟踪流式 InputJSON delta 累积状态
type accumulatedToolCall struct {
	name    string
	jsonBuf string
}

func (p *GeminiProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		usage.InputTokens = resp.Usage.InputTokens
		usage.OutputTokens = resp.Usage.OutputTokens

		parts := make([]map[string]interface{}, 0)
		// 思考链 (thought part) — 来自 ReasoningContent
		if resp.ReasoningContent != "" {
			parts = append(parts, map[string]interface{}{"thought": resp.ReasoningContent})
		}
		if resp.ReasoningSignature != nil && *resp.ReasoningSignature != "" {
			parts = append(parts, map[string]interface{}{"thoughtSignature": *resp.ReasoningSignature})
		}
		if resp.Content != "" {
			parts = append(parts, map[string]interface{}{"text": resp.Content})
		}
		for _, tc := range resp.ToolCalls {
			var args interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = tc.Function.Arguments
			}
			parts = append(parts, map[string]interface{}{
				"functionCall": map[string]interface{}{"name": tc.Function.Name, "args": args},
			})
		}

		geminiResp := map[string]interface{}{
			"candidates": []map[string]interface{}{{
				"content":      map[string]interface{}{"role": "model", "parts": parts},
				"finishReason": reasonmap.UnifiedToGemini(resp.FinishReason),
			}},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     resp.Usage.InputTokens,
				"candidatesTokenCount": resp.Usage.OutputTokens,
				"totalTokenCount":      resp.Usage.TotalTokens(),
			},
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		body, _ := json.Marshal(geminiResp)
		_, err := c.Writer.Write(body)
		return err
	}

	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var inputTokens, outputTokens int
	accumulated := make(map[string]*accumulatedToolCall) // InputJSON 累积: tool_call_id → state
	for ev := range events {
		switch ev.Type {
		case unified.EventChunk:
			if ev.Delta == nil {
				continue
			}
			// 思考链增量
			if ev.Delta.ReasoningContent != "" {
				chunk := map[string]interface{}{
					"candidates": []map[string]interface{}{{
						"content": map[string]interface{}{"role": "model", "parts": []map[string]interface{}{{"thought": ev.Delta.ReasoningContent}}},
					}},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
			}
			if ev.Delta.ReasoningSignature != nil && *ev.Delta.ReasoningSignature != "" {
				chunk := map[string]interface{}{
					"candidates": []map[string]interface{}{{
						"content": map[string]interface{}{"role": "model", "parts": []map[string]interface{}{{"thoughtSignature": *ev.Delta.ReasoningSignature}}},
					}},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
			}
			// 文本增量
			if ev.Delta.Content != "" {
				chunk := map[string]interface{}{
					"candidates": []map[string]interface{}{{
						"content": map[string]interface{}{"role": "model", "parts": []map[string]interface{}{{"text": ev.Delta.Content}}},
					}},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
			}
			// OpenAI 风格的 ToolCalls（流式增量）
			for _, tc := range ev.Delta.ToolCalls {
				var args interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = tc.Function.Arguments
				}
				chunk := map[string]interface{}{
					"candidates": []map[string]interface{}{{
						"content": map[string]interface{}{"role": "model", "parts": []map[string]interface{}{{
							"functionCall": map[string]interface{}{"name": tc.Function.Name, "args": args},
						}}},
					}},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
			}
			// Anthropic 风格的 InputJSON（流式工具参数增量）
			if ev.Delta.InputJSON != "" {
				// 从 TransformerMetadata 获取 tool_call_id 和 tool_name（Anthropic stream.go 设置）
				toolCallID := "tool"
				toolName := "unknown"
				if meta := ev.Delta.TransformerMetadata; meta != nil {
					if id, ok := meta["tool_call_id"].(string); ok && id != "" {
						toolCallID = id
					}
					if name, ok := meta["tool_name"].(string); ok && name != "" {
						toolName = name
					}
				}
				acc, ok := accumulated[toolCallID]
				if !ok {
					acc = &accumulatedToolCall{name: toolName}
					accumulated[toolCallID] = acc
				}
				acc.jsonBuf += ev.Delta.InputJSON
			}
		case unified.EventUsage:
			if ev.Usage != nil {
				inputTokens = ev.Usage.InputTokens
				outputTokens = ev.Usage.OutputTokens
			}
		case unified.EventDone:
			parts := make([]interface{}, 0, len(accumulated))
			for _, acc := range accumulated {
				var args interface{}
				if err := json.Unmarshal([]byte(acc.jsonBuf), &args); err != nil {
					args = acc.jsonBuf // 无法解析则透传原始字符串
				}
				parts = append(parts, map[string]interface{}{
					"functionCall": map[string]interface{}{"name": acc.name, "args": args},
				})
			}
			chunk := map[string]interface{}{
				"candidates": []map[string]interface{}{{
					"content":      map[string]interface{}{"role": "model", "parts": parts},
					"finishReason": reasonmap.UnifiedToGemini(ev.FinishReason),
				}},
				"usageMetadata": map[string]interface{}{
					"promptTokenCount": inputTokens, "candidatesTokenCount": outputTokens, "totalTokenCount": inputTokens + outputTokens,
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		}
	}
	usage.InputTokens = inputTokens
	usage.OutputTokens = outputTokens
	return nil
}
