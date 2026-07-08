package anthropic

import (
	"context"
	"log"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"

	"ai-gateway/internal/core/reasonmap"
	"ai-gateway/internal/core/streamutil"
	"ai-gateway/internal/core/unified"
)

// =============================================================================
// 流式：Anthropic SDK stream → unified.StreamEvent chan
// =============================================================================

func (p *AnthropicProvider) streamSDKToUnified(ctx context.Context, stream *ssestream.Stream[anthropic.MessageStreamEventUnion]) <-chan unified.StreamEvent {
	ch := make(chan unified.StreamEvent, streamutil.BufferSize)
	go func() {
		defer close(ch)
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

		for stream.Next() {
			// Check context before processing
			if ctx.Err() != nil {
				return
			}
			event := stream.Current()
			switch e := event.AsAny().(type) {
			case anthropic.MessageStartEvent:
				messageID = e.Message.ID
				messageModel = string(e.Message.Model)
				inputTokens = int(e.Message.Usage.InputTokens)

			case anthropic.ContentBlockStartEvent:
				idx := int(e.Index)
				cb := e.ContentBlock.AsAny()
				switch c := cb.(type) {
				case anthropic.TextBlock:
					blocks[idx] = &blockInfo{blockType: "text"}
				case anthropic.ThinkingBlock:
					blocks[idx] = &blockInfo{blockType: "thinking"}
				case anthropic.ToolUseBlock:
					blocks[idx] = &blockInfo{
						blockType: "tool_use",
						toolName:  c.Name,
						toolID:    c.ID,
					}
				default:
					blocks[idx] = &blockInfo{blockType: "unknown"}
				}

			case anthropic.ContentBlockDeltaEvent:
				idx := int(e.Index)
				delta := e.Delta
				switch delta.Type {
				case "text_delta":
					if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
						Type:      unified.EventChunk,
						MessageID: messageID,
						Model:     messageModel,
						Delta:     &unified.Delta{Content: delta.Text},
					}) {
						return
					}
				case "thinking_delta":
					if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
						Type:      unified.EventChunk,
						MessageID: messageID,
						Model:     messageModel,
						Delta:     &unified.Delta{ReasoningContent: delta.Thinking},
					}) {
						return
					}
				case "signature_delta":
					if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
						Type:      unified.EventChunk,
						MessageID: messageID,
						Model:     messageModel,
						Delta:     &unified.Delta{ReasoningSignature: &delta.Signature},
					}) {
						return
					}
				case "input_json_delta":
					toolCallID := ""
					if bi, ok := blocks[idx]; ok && bi.blockType == "tool_use" {
						toolCallID = bi.toolID
					}
					evt := unified.StreamEvent{
						Type:      unified.EventChunk,
						MessageID: messageID,
						Model:     messageModel,
						Delta:     &unified.Delta{InputJSON: delta.PartialJSON},
					}
					if toolCallID != "" {
						evt.Delta.TransformerMetadata = map[string]any{
							"tool_call_id": toolCallID,
						}
						if bi, ok := blocks[idx]; ok && bi.toolName != "" {
							evt.Delta.TransformerMetadata["tool_name"] = bi.toolName
						}
					}
					if !streamutil.SendEvent(ctx, ch, evt) {
						return
					}
				}

			case anthropic.MessageDeltaEvent:
				if !streamutil.SendEvent(ctx, ch, unified.StreamEvent{
					Type:      unified.EventUsage,
					MessageID: messageID,
					Model:     messageModel,
					Usage: &unified.Usage{
						InputTokens:     inputTokens,
						OutputTokens:    int(e.Usage.OutputTokens),
						CacheHitTokens:  int(e.Usage.CacheReadInputTokens),
						CacheMissTokens: int(e.Usage.CacheCreationInputTokens),
					},
				}) {
					return
				}
				streamutil.SendEvent(ctx, ch, unified.StreamEvent{
					Type:         unified.EventDone,
					MessageID:    messageID,
					Model:        messageModel,
					FinishReason: reasonmap.AnthropicToUnified(string(e.Delta.StopReason)),
				})
				return

			case anthropic.MessageStopEvent:
				streamutil.SendEvent(ctx, ch, unified.StreamEvent{
					Type:         unified.EventDone,
					MessageID:    messageID,
					Model:        messageModel,
					FinishReason: unified.FinishReasonStop,
				})
				return
			}
		}
		if err := stream.Err(); err != nil {
			log.Printf("[Anthropic SDK stream] error: %v", err)
			streamutil.SendEvent(ctx, ch, unified.StreamEvent{Type: unified.EventError})
		}
	}()
	return ch
}
