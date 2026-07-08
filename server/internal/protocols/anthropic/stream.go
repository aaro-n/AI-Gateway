package anthropic

import (
	"bufio"
	"io"
	"encoding/json"
	"ai-gateway/internal/core/reasonmap"
	"strings"
	"ai-gateway/internal/core/unified"
)

// =============================================================================
// 流式：Anthropic SSE → unified.StreamEvent chan
// =============================================================================

func (p *AnthropicProvider) streamAnthropicToUnified(body io.ReadCloser) <-chan unified.StreamEvent {
	ch := make(chan unified.StreamEvent, 32)
	go func() {
		defer body.Close()
		defer close(ch)
		reader := bufio.NewReader(body)
		var inputTokens int
		var messageID string
		var messageModel string
		// 跟踪 content_block_start 以关联 input_json_delta 到正确的 tool
		type blockInfo struct {
			blockType string // "text", "thinking", "tool_use"
			toolName  string
			toolID    string
		}
		blocks := make(map[int]*blockInfo) // index → block info
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
			var event struct {
				Type         string          `json:"type"`
				Index        *int            `json:"index"`
				Message      json.RawMessage `json:"message"`
				Delta        json.RawMessage `json:"delta"`
				Usage        json.RawMessage `json:"usage"`
				ContentBlock json.RawMessage `json:"content_block"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "message_start":
				if len(event.Message) > 0 {
					var msg struct {
						ID    string `json:"id"`
						Model string `json:"model"`
						Usage struct {
							InputTokens int `json:"input_tokens"`
						} `json:"usage"`
					}
					if json.Unmarshal(event.Message, &msg) == nil {
						messageID = msg.ID
						messageModel = msg.Model
						inputTokens = msg.Usage.InputTokens
					}
				}
			case "content_block_start":
				if event.Index != nil && len(event.ContentBlock) > 0 {
					var cb struct {
						Type string `json:"type"`
						ID   string `json:"id"`
						Name string `json:"name"`
					}
					if json.Unmarshal(event.ContentBlock, &cb) == nil {
						blocks[*event.Index] = &blockInfo{
							blockType: cb.Type,
							toolName:  cb.Name,
							toolID:    cb.ID,
						}
					}
				}
			case "content_block_delta":
				idx := -1
				if event.Index != nil {
					idx = *event.Index
				}
				if len(event.Delta) > 0 {
					var delta struct {
						Type        string `json:"type"`
						Text        string `json:"text"`
						Thinking    string `json:"thinking"`
						Signature   string `json:"signature"`
						PartialJSON string `json:"partial_json"`
					}
					if json.Unmarshal(event.Delta, &delta) == nil {
						switch delta.Type {
						case "text_delta":
							ch <- unified.StreamEvent{
								Type:      unified.EventChunk,
								MessageID: messageID,
								Model:     messageModel,
								Delta: &unified.Delta{
									Content: delta.Text,
								},
							}
						case "thinking_delta":
							ch <- unified.StreamEvent{
								Type:      unified.EventChunk,
								MessageID: messageID,
								Model:     messageModel,
								Delta: &unified.Delta{
									ReasoningContent: delta.Thinking,
								},
							}
						case "signature_delta":
							ch <- unified.StreamEvent{
								Type:      unified.EventChunk,
								MessageID: messageID,
								Model:     messageModel,
								Delta: &unified.Delta{
									ReasoningSignature: &delta.Signature,
								},
							}
						case "input_json_delta":
							// 从 content_block_start 获取 tool name/ID
							toolCallID := ""
							if bi, ok := blocks[idx]; ok && bi.blockType == "tool_use" {
								toolCallID = bi.toolID
							}
							evt := unified.StreamEvent{
								Type:      unified.EventChunk,
								MessageID: messageID,
								Model:     messageModel,
								Delta: &unified.Delta{
									InputJSON: delta.PartialJSON,
								},
							}
							if toolCallID != "" {
								evt.Delta.TransformerMetadata = map[string]any{
									"tool_call_id": toolCallID,
								}
								// 也将 tool name 传到第一个 tool_call slot
								if bi, ok := blocks[idx]; ok && bi.toolName != "" {
									evt.Delta.TransformerMetadata["tool_name"] = bi.toolName
								}
							}
							ch <- evt
						}
					}
				}
			case "message_delta":
				if len(event.Usage) > 0 {
					var u struct {
						OutputTokens int `json:"output_tokens"`
					}
					if json.Unmarshal(event.Usage, &u) == nil {
						ch <- unified.StreamEvent{
							Type:      unified.EventUsage,
							MessageID: messageID,
							Model:     messageModel,
							Usage: &unified.Usage{
								InputTokens:  inputTokens,
								OutputTokens: u.OutputTokens,
							},
						}
					}
				}
				// message_delta 也包含 stop_reason
				if len(event.Delta) > 0 {
					var md struct {
						StopReason string `json:"stop_reason"`
					}
					if json.Unmarshal(event.Delta, &md) == nil {
						ch <- unified.StreamEvent{
							Type:         unified.EventDone,
							MessageID:    messageID,
							Model:        messageModel,
							FinishReason: reasonmap.AnthropicToUnified(md.StopReason),
						}
						return
					}
				}
			case "message_stop":
				ch <- unified.StreamEvent{
					Type:         unified.EventDone,
					MessageID:    messageID,
					Model:        messageModel,
					FinishReason: unified.FinishReasonStop,
				}
				return
			}
		}
	}()
	return ch
}


