package anthropic

import (
	"ai-gateway/internal/core/reasonmap"
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// FormatUnified — Unified 响应/流 → Anthropic 客户端格式
// =============================================================================

func (p *AnthropicProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		// 非流式
		usage.InputTokens = resp.Usage.InputTokens
		usage.OutputTokens = resp.Usage.OutputTokens

		contentBlocks := make([]map[string]interface{}, 0)
		// thinking 块 — 从 ReasoningContent
		if resp.ReasoningContent != "" {
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type":     "thinking",
				"thinking": resp.ReasoningContent,
			})
		}
		if resp.Content != "" {
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type": "text",
				"text": resp.Content,
			})
		}
		for _, tc := range resp.ToolCalls {
			var input interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				input = tc.Function.Arguments
			}
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": input,
			})
		}

		stopReason := reasonmap.UnifiedToAnthropic(resp.FinishReason)
		// 恢复 stop_sequence 原值（如果有）
		if resp.TransformerMetadata != nil {
			if ss, ok := resp.TransformerMetadata["stop_sequence"].(string); ok && ss == "stop_sequence" {
				stopReason = "stop_sequence"
			}
		}

		anthropicResp := map[string]interface{}{
			"id":          resp.ID,
			"type":        "message",
			"role":        "assistant",
			"model":       resp.Model,
			"content":     contentBlocks,
			"stop_reason": stopReason,
			"usage": map[string]interface{}{
				"input_tokens":  resp.Usage.InputTokens,
				"output_tokens": resp.Usage.OutputTokens,
			},
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		body, _ := json.Marshal(anthropicResp)
		_, err := c.Writer.Write(body)
		return err
	}

	// 流式：Unified events → Anthropic SSE
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var inputTokens, outputTokens int
	var messageStarted bool
	var messageID string
	var messageModel string
	var blockIndex int
	var blockActive bool
	var currentBlockType string // "text" or "thinking" or "tool_use"

	// 并行 tool_use index 跟踪 (参考 New-API ToolCallBaseIndex)
	toolUseCount := 0
	toolBlocks := make(map[int]*struct {
		toolName string
		toolID   string
	})

	emitMessageStart := func() {
		if messageStarted {
			return
		}
		if messageID == "" {
			messageID = "msg_unified"
		}
		if messageModel == "" {
			messageModel = "unknown"
		}
		p.writeSSE(c, map[string]interface{}{
			"type": "message_start",
			"message": map[string]interface{}{
				"id":    messageID,
				"type":  "message",
				"role":  "assistant",
				"model": messageModel,
			},
		})
		messageStarted = true
	}

	ensureBlockStart := func(blockType string) {
		if !messageStarted {
			emitMessageStart()
		}
		if blockActive && currentBlockType == blockType {
			return
		}
		if blockActive {
			// 关闭当前 block
			p.writeSSE(c, map[string]interface{}{
				"type":  "content_block_stop",
				"index": blockIndex,
			})
			if currentBlockType == "tool_use" {
				toolUseCount++
			}
			blockIndex++
		}
		// content_block_start — 包含 block 类型及其特定字段
		cb := map[string]interface{}{"type": blockType}
		if blockType == "tool_use" {
			cb["name"] = ""
			cb["id"] = ""
			if bi, ok := toolBlocks[toolUseCount]; ok {
				if bi.toolName != "" {
					cb["name"] = bi.toolName
				}
				if bi.toolID != "" {
					cb["id"] = bi.toolID
				}
			}
		} else {
			cb[blockType] = ""
		}
		p.writeSSE(c, map[string]interface{}{
			"type":          "content_block_start",
			"index":         blockIndex,
			"content_block": cb,
		})
		blockActive = true
		currentBlockType = blockType
	}

	for ev := range events {
		// 从事件中提取 message 元信息
		if ev.MessageID != "" && messageID == "" {
			messageID = ev.MessageID
		}
		if ev.Model != "" && messageModel == "" {
			messageModel = ev.Model
		}
		switch ev.Type {
		case unified.EventChunk:
			if ev.Delta == nil {
				continue
			}
			// signature_delta — 直接 emit，不分 block（signature 总是跟随在 thinking_delta 之后）
			if ev.Delta.ReasoningSignature != nil {
				p.writeSSE(c, map[string]interface{}{
					"type":  "content_block_delta",
					"index": blockIndex,
					"delta": map[string]interface{}{
						"type":      "signature_delta",
						"signature": *ev.Delta.ReasoningSignature,
					},
				})
				continue
			}
			// OpenAI 风格的 ToolCalls（流式增量：index + id + function.{name,arguments}）
			if len(ev.Delta.ToolCalls) > 0 {
				for _, tc := range ev.Delta.ToolCalls {
					if tc.ID != "" {
						// 先记录 tool 元信息，再创建 content_block_start
						if _, ok := toolBlocks[toolUseCount]; !ok {
							toolBlocks[toolUseCount] = &struct {
								toolName string
								toolID   string
							}{}
						}
						if tc.Function.Name != "" {
							toolBlocks[toolUseCount].toolName = tc.Function.Name
						}
						toolBlocks[toolUseCount].toolID = tc.ID
					}
					if tc.Function.Arguments != "" {
						ensureBlockStart("tool_use")
						p.writeSSE(c, map[string]interface{}{
							"type":  "content_block_delta",
							"index": blockIndex,
							"delta": map[string]interface{}{
								"type":         "input_json_delta",
								"partial_json": tc.Function.Arguments,
							},
						})
					}
				}
			}
			// input_json_delta → tool_use block
			if ev.Delta.InputJSON != "" {
				ensureBlockStart("tool_use")
				delta := map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": ev.Delta.InputJSON,
				}
				p.writeSSE(c, map[string]interface{}{
					"type":  "content_block_delta",
					"index": blockIndex,
					"delta": delta,
				})
			}
			if ev.Delta.ReasoningContent != "" {
				ensureBlockStart("thinking")
				p.writeSSE(c, map[string]interface{}{
					"type":  "content_block_delta",
					"index": blockIndex,
					"delta": map[string]interface{}{
						"type":     "thinking_delta",
						"thinking": ev.Delta.ReasoningContent,
					},
				})
			}
			if ev.Delta.Content != "" {
				ensureBlockStart("text")
				p.writeSSE(c, map[string]interface{}{
					"type":  "content_block_delta",
					"index": blockIndex,
					"delta": map[string]interface{}{
						"type": "text_delta",
						"text": ev.Delta.Content,
					},
				})
			}
		case unified.EventUsage:
			if ev.Usage != nil {
				inputTokens = ev.Usage.InputTokens
				outputTokens = ev.Usage.OutputTokens
			}
		case unified.EventDone:
			// 确保 message_start（如果还没有任何 content block）
			if !messageStarted {
				emitMessageStart()
			}
			// 关闭当前 block
			if blockActive {
				p.writeSSE(c, map[string]interface{}{
					"type":  "content_block_stop",
					"index": blockIndex,
				})
				blockIndex++
				blockActive = false
			}
			// message_delta with stop_reason + usage
			stopReason := reasonmap.UnifiedToAnthropic(ev.FinishReason)
			msgDelta := map[string]interface{}{
				"type":  "message_delta",
				"delta": map[string]interface{}{"stop_reason": stopReason},
				"usage": map[string]interface{}{
					"output_tokens": outputTokens,
				},
			}
			// message_delta 需要 input_tokens（来自 message_start），合并 usage
			if inputTokens > 0 {
				msgDelta["usage"].(map[string]interface{})["input_tokens"] = inputTokens
			}
			p.writeSSE(c, msgDelta)
			// message_stop
			p.writeSSE(c, map[string]interface{}{"type": "message_stop"})
		}
	}
	usage.InputTokens = inputTokens
	usage.OutputTokens = outputTokens
	return nil
}

func (p *AnthropicProvider) writeSSE(c *gin.Context, event map[string]interface{}) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event["type"], data)
	c.Writer.Flush()
}
