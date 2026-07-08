package deepseek

import (
	"ai-gateway/internal/core/unified"
	"encoding/json"
	"fmt"
	"strings"
)

// =============================================================================
// ToUnified — DeepSeek 请求 → UnifiedRequest
// 关键：保留 assistant 消息中的 reasoning_content（多轮对话必须原样传回，否则报 400）
func (p *DeepSeekProvider) ToUnified(body []byte, modelID string) (*unified.Request, error) {
	var raw struct {
		Model               string            `json:"model"`
		Messages            []json.RawMessage `json:"messages"`
		MaxTokens           int               `json:"max_tokens"`
		MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
		Temperature         *float64          `json:"temperature,omitempty"`
		TopP                *float64          `json:"top_p,omitempty"`
		FrequencyPenalty    *float64          `json:"frequency_penalty,omitempty"`
		PresencePenalty     *float64          `json:"presence_penalty,omitempty"`
		Stream              bool              `json:"stream,omitempty"`
		Tools               json.RawMessage   `json:"tools,omitempty"`
		ToolChoice          json.RawMessage   `json:"tool_choice,omitempty"`
		ResponseFormat      json.RawMessage   `json:"response_format,omitempty"`
		Stop                []string          `json:"stop,omitempty"`
		ReasoningEffort     string            `json:"reasoning_effort,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse deepseek body: %w", err)
	}

	msgs := make([]unified.Message, 0, len(raw.Messages))
	systemParts := make([]string, 0)
	for _, rawMsg := range raw.Messages {
		// 一次解析到包含 extra fields 的结构体，避免重复 Unmarshal
		var msgWithExtras struct {
			Role             string             `json:"role"`
			Content          json.RawMessage    `json:"content"`
			ReasoningContent string             `json:"reasoning_content,omitempty"`
			Prefix           bool               `json:"prefix,omitempty"`
			ToolCalls        []unified.ToolCall `json:"tool_calls,omitempty"`
			ToolCallID       string             `json:"tool_call_id,omitempty"`
			Name             string             `json:"name,omitempty"`
		}
		if err := json.Unmarshal(rawMsg, &msgWithExtras); err != nil {
			return nil, fmt.Errorf("parse message: %w", err)
		}
		m := unified.Message{
			Role:             msgWithExtras.Role,
			Content:          msgWithExtras.Content,
			ReasoningContent: msgWithExtras.ReasoningContent,
			Prefix:           msgWithExtras.Prefix,
			ToolCalls:        msgWithExtras.ToolCalls,
			ToolCallID:       msgWithExtras.ToolCallID,
			Name:             msgWithExtras.Name,
		}
		// 如果 assistant 消息的 content 中包含 <think>...</think> 标签，
		// 剥离标签内容到 ReasoningContent（兼容标准 OpenAI 客户端回传的格式）
		if m.Role == "assistant" && m.ReasoningContent == "" {
			contentStr := unified.ContentString(m.Content)
			if reasoning, cleanContent := stripThinkTag(contentStr); reasoning != "" {
				m.ReasoningContent = reasoning
				m.Content = unified.StringContent(cleanContent)
			}
		}
		// 提取 prefix（DeepSeek Chat Prefix Completion）
		m.Prefix = msgWithExtras.Prefix
		if m.Role == "system" {
			systemParts = append(systemParts, unified.ContentString(m.Content))
			continue
		}
		msgs = append(msgs, m)
	}

	maxTokens := raw.MaxTokens
	if maxTokens == 0 && raw.MaxCompletionTokens != nil {
		maxTokens = *raw.MaxCompletionTokens
	}

	req := &unified.Request{
		Model:            modelID,
		Messages:         msgs,
		MaxTokens:        maxTokens,
		Temperature:      raw.Temperature,
		TopP:             raw.TopP,
		FrequencyPenalty: raw.FrequencyPenalty,
		PresencePenalty:  raw.PresencePenalty,
		Stream:           raw.Stream,
		ToolChoice:       raw.ToolChoice,
		ResponseFormat:   raw.ResponseFormat,
		Stop:             raw.Stop,
		ReasoningEffort:  raw.ReasoningEffort,
		SourceProtocol:   "deepseek",
	}
	if len(systemParts) > 0 {
		req.SystemPrompt = strings.Join(systemParts, "\n")
	}
	if len(raw.Tools) > 0 {
		req.Tools = raw.Tools
	}
	return req, nil
}
