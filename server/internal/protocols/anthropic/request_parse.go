package anthropic

import (
	"ai-gateway/internal/core/unified"
	"encoding/json"
	"fmt"
	"strings"
)

// =============================================================================
// ToUnified — Anthropic 请求 → UnifiedRequest
// =============================================================================

func (p *AnthropicProvider) ToUnified(body []byte, modelID string) (*unified.Request, error) {
	var raw struct {
		Model       string            `json:"model"`
		MaxTokens   int               `json:"max_tokens"`
		System      json.RawMessage   `json:"system,omitempty"`
		Messages    []json.RawMessage `json:"messages"`
		Tools       json.RawMessage   `json:"tools,omitempty"`
		Stream      bool              `json:"stream,omitempty"`
		Temperature *float64          `json:"temperature,omitempty"`
		TopP        *float64          `json:"top_p,omitempty"`
		TopK        *int              `json:"top_k,omitempty"`
		Stop        []string          `json:"stop_sequences,omitempty"`
		Thinking    *struct {
			Type         string `json:"type"`
			BudgetTokens *int   `json:"budget_tokens,omitempty"`
		} `json:"thinking,omitempty"`
		ToolChoice json.RawMessage `json:"tool_choice,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse anthropic body: %w", err)
	}

	// 解析 system（可能是 string 或 content blocks）— 同时提取 cache_control
	systemPrompt := ""
	var systemCacheControl map[string]any
	if len(raw.System) > 0 {
		var s string
		if json.Unmarshal(raw.System, &s) == nil {
			systemPrompt = s
		} else {
			var blocks []struct {
				Type         string         `json:"type"`
				Text         string         `json:"text"`
				CacheControl map[string]any `json:"cache_control,omitempty"`
			}
			if json.Unmarshal(raw.System, &blocks) == nil {
				var sb strings.Builder
				for _, b := range blocks {
					if b.Type == "text" {
						sb.WriteString(b.Text)
						if b.CacheControl != nil {
							systemCacheControl = b.CacheControl
						}
					}
				}
				systemPrompt = sb.String()
			}
		}
	}

	// 转换 messages
	msgs := make([]unified.Message, 0, len(raw.Messages))
	for _, rawMsg := range raw.Messages {
		var m struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(rawMsg, &m); err != nil {
			return nil, fmt.Errorf("parse anthropic message: %w", err)
		}
		um := unified.Message{Role: m.Role}

		// Anthropic content 可能是 string 或 content blocks
		// 统一转为 OpenAI 风格：assistant 的 tool_use → tool_calls，user 的 tool_result → tool role
		var s string
		if json.Unmarshal(m.Content, &s) == nil {
			um.Content = unified.StringContent(s)
			msgs = append(msgs, um)
			continue
		}

		type anthropicSource struct {
			Type      string `json:"type"`
			MediaType string `json:"media_type"`
			Data      string `json:"data"`
		}
		type anthropicBlock struct {
			Type         string           `json:"type"`
			Text         string           `json:"text,omitempty"`
			ID           string           `json:"id,omitempty"`
			Name         string           `json:"name,omitempty"`
			Input        json.RawMessage  `json:"input,omitempty"`
			ToolUseID    string           `json:"tool_use_id,omitempty"`
			Content      json.RawMessage  `json:"content,omitempty"`
			Source       *anthropicSource `json:"source,omitempty"`
			CacheControl map[string]any   `json:"cache_control,omitempty"`
		}

		var blocks []anthropicBlock
		if json.Unmarshal(m.Content, &blocks) != nil {
			um.Content = m.Content
			msgs = append(msgs, um)
			continue
		}

		// 分离 tool_use / tool_result / text / image — 保留 cache_control
		textParts := make([]string, 0)
		var toolCalls []unified.ToolCall
		var toolResults []unified.Message
		var unifiedBlocks []unified.ContentBlock
		var hasImage bool

		for _, b := range blocks {
			cc := b.CacheControl
			switch b.Type {
			case "text":
				textParts = append(textParts, b.Text)
				cb := unified.ContentBlock{Type: "text", Text: b.Text}
				if cc != nil {
					cb.TransformerMetadata = map[string]any{"cache_control": cc}
				}
				unifiedBlocks = append(unifiedBlocks, cb)
			case "image":
				if b.Source != nil && b.Source.Type == "base64" {
					hasImage = true
					unifiedBlocks = append(unifiedBlocks, unified.ContentBlock{
						Type: "image_url",
						ImageURL: &unified.ImageURL{
							URL: fmt.Sprintf("data:%s;base64,%s", b.Source.MediaType, b.Source.Data),
						},
					})
				}
			case "tool_use":
				args, _ := json.Marshal(b.Input)
				toolCalls = append(toolCalls, unified.ToolCall{
					ID:   b.ID,
					Type: "function",
					Function: unified.FunctionCall{
						Name:      b.Name,
						Arguments: string(args),
					},
				})
			case "tool_result":
				trMeta := map[string]any{}
				if cc != nil {
					trMeta["cache_control"] = cc
				}
				toolResults = append(toolResults, unified.Message{
					Role:                "tool",
					ToolCallID:          b.ToolUseID,
					Content:             b.Content,
					TransformerMetadata: trMeta,
				})
			}
		}

		if hasImage {
			um.Content = unified.BlocksContent(unifiedBlocks)
		} else if len(textParts) > 0 {
			um.Content = unified.StringContent(strings.Join(textParts, "\n"))
		}

		if len(toolCalls) > 0 {
			um.ToolCalls = toolCalls
		}
		msgs = append(msgs, um)
		msgs = append(msgs, toolResults...)
	}

	req := &unified.Request{
		Model:          modelID,
		Messages:       msgs,
		SystemPrompt:   systemPrompt,
		MaxTokens:      raw.MaxTokens,
		Temperature:    raw.Temperature,
		TopP:           raw.TopP,
		TopK:           raw.TopK,
		Stream:         raw.Stream,
		Stop:           raw.Stop,
		SourceProtocol: "anthropic",
	}
	if len(raw.Tools) > 0 {
		req.Tools = raw.Tools
	}
	// 透传 thinking 配置（Claude extended thinking）
	if raw.Thinking != nil {
		req.ReasoningEffort = raw.Thinking.Type
		req.ReasoningBudget = raw.Thinking.BudgetTokens
		req.TransformerMetadata = map[string]any{
			"thinking_type": raw.Thinking.Type,
		}
		if raw.Thinking.BudgetTokens != nil {
			req.TransformerMetadata["thinking_budget_tokens"] = *raw.Thinking.BudgetTokens
		}
	}
	// 透传 tool_choice（Anthropic 原生格式, e.g. {"type":"auto"} / {"type":"any"} / {"type":"tool","name":"xxx"}）
	if len(raw.ToolChoice) > 0 {
		req.ToolChoice = raw.ToolChoice
	}
	// 透传 system cache_control
	if systemCacheControl != nil {
		if req.TransformerMetadata == nil {
			req.TransformerMetadata = map[string]any{}
		}
		req.TransformerMetadata["system_cache_control"] = systemCacheControl
	}
	return req, nil
}
