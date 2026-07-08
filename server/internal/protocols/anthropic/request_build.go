package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"

	"ai-gateway/internal/core/reasonmap"
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
)

// =============================================================================
// FromUnified — UnifiedRequest → Anthropic 请求，发上游，返回统一响应
// =============================================================================

func (p *AnthropicProvider) FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error) {
	anthropicReq := p.unifiedToAnthropic(req)

	// 非流式：直接用 SDK Client.Post() → 获得连接池复用 + 类型安全响应
	if !req.Stream {
		var msg anthropic.Message
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		err := p.sdk.Post(ctx, "/v1/messages", anthropicReq, &msg)
		if err != nil {
			return nil, nil, convertSDKError(err)
		}
		uresp := sdkMessageToUnified(&msg)
		return uresp, nil, nil
	}

	// 流式：仍用手写 SSE 解析（SDK 的 NewStreaming 需要类型化 params），但用共享 httpClient
	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, nil, err
	}

	httpReq, err := http.NewRequest("POST", p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpPool.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, nil, &registry.HTTPError{StatusCode: resp.StatusCode, Body: respBody}
	}

	events := p.streamAnthropicToUnified(resp.Body)
	return nil, events, nil
}

// sdkMessageToUnified 将 SDK *anthropic.Message 转为 unified.Response。
func sdkMessageToUnified(msg *anthropic.Message) *unified.Response {
	uresp := &unified.Response{
		ID:    msg.ID,
		Model: string(msg.Model),
		Usage: unified.Usage{
			InputTokens:  int(msg.Usage.InputTokens),
			OutputTokens: int(msg.Usage.OutputTokens),
		},
	}

	var textContent string
	var reasoningContent string
	var toolCalls []unified.ToolCall
	for _, c := range msg.Content {
		switch c.Type {
		case "text":
			textContent += c.Text
		case "thinking":
			reasoningContent += c.Thinking
		case "tool_use":
			args, _ := json.Marshal(c.Input)
			toolCalls = append(toolCalls, unified.ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: unified.FunctionCall{
					Name:      c.Name,
					Arguments: string(args),
				},
			})
		}
	}
	uresp.Content = textContent
	uresp.ReasoningContent = reasoningContent
	uresp.ToolCalls = toolCalls
	uresp.FinishReason = reasonmap.AnthropicToUnified(string(msg.StopReason))
	if msg.StopReason == "stop_sequence" {
		uresp.TransformerMetadata = map[string]any{"stop_sequence": string(msg.StopReason)}
	}
	return uresp
}

// convertSDKError 将 SDK 返回的错误转为 registry.HTTPError。
func convertSDKError(err error) error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		body := ""
		if apiErr.Response != nil && apiErr.Response.Body != nil {
			b, _ := io.ReadAll(apiErr.Response.Body)
			body = string(b)
		}
		return &registry.HTTPError{StatusCode: apiErr.StatusCode, Body: []byte(body)}
	}
	return fmt.Errorf("anthropic API: %w", err)
}

// unifiedToAnthropic 将 UnifiedRequest 转为 Anthropic 请求体
func (p *AnthropicProvider) unifiedToAnthropic(req *unified.Request) map[string]interface{} {
	anthropicMsgs := make([]map[string]interface{}, 0, len(req.Messages))

	// 从 TransformerMetadata 恢复 cache_control 和 thinking
	meta := req.TransformerMetadata
	hasSystemCacheControl := meta != nil && meta["system_cache_control"] != nil

	for _, m := range req.Messages {
		if m.Role == "system" {
			continue // system 走顶层字段
		}

		if m.Role == "tool" {
			// OpenAI tool role → Anthropic user message with tool_result block
			trBlock := map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": m.ToolCallID,
				"content":     unified.ContentString(m.Content),
			}
			// 恢复 cache_control（从 TransformerMetadata）
			if m.TransformerMetadata != nil {

				if cc, ok := m.TransformerMetadata["cache_control"]; ok {
					trBlock["cache_control"] = cc
				}
			}
			anthropicMsgs = append(anthropicMsgs, map[string]interface{}{
				"role":    "user",
				"content": []map[string]interface{}{trBlock},
			})
			continue
		}

		// user / assistant — 保留 cache_control
		blocks := p.unifiedContentToAnthropicBlocks(m)
		// 从消息的 TransformerMetadata 恢复 cache_control，附加到第一个 text block
		if m.TransformerMetadata != nil {
			if cc, ok := m.TransformerMetadata["cache_control"]; ok {
				for i := range blocks {
					if blocks[i]["type"] == "text" {
						blocks[i]["cache_control"] = cc
						break
					}
				}
			}
		}
		msg := map[string]interface{}{
			"role":    m.Role,
			"content": blocks,
		}
		// assistant 的 tool_calls → tool_use blocks
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var input interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
					input = tc.Function.Arguments
				}
				blocks = append(blocks, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			msg["content"] = blocks
		}
		anthropicMsgs = append(anthropicMsgs, msg)
	}

	result := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   anthropicMsgs,
		"stream":     req.Stream,
	}
	if req.MaxTokens == 0 {
		result["max_tokens"] = 8192
	}
	if req.SystemPrompt != "" {
		result["system"] = req.SystemPrompt
	}
	if req.Temperature != nil {
		result["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		result["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		result["top_k"] = *req.TopK
	}
	if len(req.Stop) > 0 {
		result["stop_sequences"] = req.Stop
	}
	// 还原 thinking 配置
	thinkingType := ""
	if meta != nil {
		if t, ok := meta["thinking_type"].(string); ok {
			thinkingType = t
		}
	}
	if req.ReasoningEffort != "" && thinkingType == "" {
		thinkingType = req.ReasoningEffort
	}
	if thinkingType != "" && thinkingType != "none" {
		thinking := map[string]interface{}{"type": thinkingType}
		if req.ReasoningBudget != nil && *req.ReasoningBudget > 0 {
			thinking["budget_tokens"] = *req.ReasoningBudget
		} else if meta != nil {
			if bt, ok := meta["thinking_budget_tokens"].(float64); ok {
				thinking["budget_tokens"] = int(bt)
			} else if bt, ok := meta["thinking_budget_tokens"].(int); ok {
				thinking["budget_tokens"] = bt
			}
		}
		result["thinking"] = thinking
	}
	// 还原 tool_choice（优先 Anthropic 原生格式，兼容 simple string）
	if len(req.ToolChoice) > 0 {
		var raw interface{}
		if err := json.Unmarshal(req.ToolChoice, &raw); err == nil {
			result["tool_choice"] = raw
		}
	}
	// 还原 system prompt 的 cache_control（使用 Anthropic content block 数组格式）
	if req.SystemPrompt != "" {
		if hasSystemCacheControl {
			cc := meta["system_cache_control"]
			result["system"] = []map[string]interface{}{
				{"type": "text", "text": req.SystemPrompt, "cache_control": cc},
			}
		} else {
			result["system"] = req.SystemPrompt
		}
	}
	if len(req.Tools) > 0 {
		var unifiedTools []unified.Tool
		if err := json.Unmarshal(req.Tools, &unifiedTools); err == nil {
			tools := make([]map[string]interface{}, 0, len(unifiedTools))
			for _, t := range unifiedTools {
				if t.Function.Name != "" {
					tools = append(tools, map[string]interface{}{
						"name":         t.Function.Name,
						"description":  t.Function.Description,
						"input_schema": t.Function.Parameters,
					})
				}
			}
			if len(tools) > 0 {
				result["tools"] = tools
			}
		}
	}
	return result
}

// unifiedContentToAnthropicBlocks 将 Unified Message content 转为 Anthropic content blocks
func (p *AnthropicProvider) unifiedContentToAnthropicBlocks(m unified.Message) []map[string]interface{} {
	blocks := make([]map[string]interface{}, 0)

	// string content
	if s := unified.ContentString(m.Content); s != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "text",
			"text": s,
		})
		return blocks
	}

	// content blocks
	for _, b := range unified.ContentBlocks(m.Content) {
		switch b.Type {
		case "text":
			blocks = append(blocks, map[string]interface{}{
				"type": "text",
				"text": b.Text,
			})
		case "image_url":
			if b.ImageURL != nil {
				url := b.ImageURL.URL
				if strings.HasPrefix(url, "data:") {
					mediaType, data := parseDataURL(url)
					blocks = append(blocks, map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type":       "base64",
							"media_type": mediaType,
							"data":       data,
						},
					})
				} else {
					blocks = append(blocks, map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type": "url",
							"url":  url,
						},
					})
				}
			}
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": ""})
	}
	return blocks
}

func parseDataURL(url string) (mediaType, data string) {
	// data:image/jpeg;base64,xxxx
	if idx := strings.Index(url, ";"); idx > 0 {
		mediaType = strings.TrimPrefix(url[:idx], "data:")
		if comma := strings.Index(url, ","); comma > 0 {
			data = url[comma+1:]
		}
	}
	return
}
