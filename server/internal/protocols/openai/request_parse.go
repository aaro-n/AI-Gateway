package openai

import (
	"ai-gateway/internal/core/unified"
	"encoding/json"
	"fmt"
	"strings"
)

// ToUnified — OpenAI 请求 → UnifiedRequest（基本透传，OpenAI 是中间表示的基础）
// =============================================================================

func (p *OpenAIProvider) ToUnified(body []byte, modelID string) (*unified.Request, error) {
	var raw struct {
		Model               string            `json:"model"`
		Messages            []json.RawMessage `json:"messages"`
		MaxTokens           int               `json:"max_tokens"`
		MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
		Temperature         *float64          `json:"temperature,omitempty"`
		TopP                *float64          `json:"top_p,omitempty"`
		TopK                *int              `json:"top_k,omitempty"`
		Seed                *int              `json:"seed,omitempty"`
		FrequencyPenalty    *float64          `json:"frequency_penalty,omitempty"`
		PresencePenalty     *float64          `json:"presence_penalty,omitempty"`
		Stream              bool              `json:"stream,omitempty"`
		Tools               json.RawMessage   `json:"tools,omitempty"`
		ToolChoice          json.RawMessage   `json:"tool_choice,omitempty"`
		ResponseFormat      json.RawMessage   `json:"response_format,omitempty"`
		Stop                []string          `json:"stop,omitempty"`
		ReasoningEffort     string            `json:"reasoning_effort,omitempty"`
		Modalities          []string          `json:"modalities,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse openai body: %w", err)
	}

	// 合并 system 消息为 SystemPrompt（便于跨协议转换）
	msgs := make([]unified.Message, 0, len(raw.Messages))
	systemParts := make([]string, 0)
	for _, rawMsg := range raw.Messages {
		// 一次解析到包含 extra fields 的结构体，避免重复 Unmarshal
		var msgWithExtras struct {
			Role             string             `json:"role"`
			Content          json.RawMessage    `json:"content"`
			ReasoningContent string             `json:"reasoning_content,omitempty"`
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
			ToolCalls:        msgWithExtras.ToolCalls,
			ToolCallID:       msgWithExtras.ToolCallID,
			Name:             msgWithExtras.Name,
		}
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
		TopK:             raw.TopK,
		Seed:             raw.Seed,
		FrequencyPenalty: raw.FrequencyPenalty,
		PresencePenalty:  raw.PresencePenalty,
		Stream:           raw.Stream,
		ToolChoice:       raw.ToolChoice,
		ResponseFormat:   raw.ResponseFormat,
		Stop:             raw.Stop,
		ReasoningEffort:  raw.ReasoningEffort,
		Modalities:       raw.Modalities,
		SourceProtocol:   "openai",
	}
	if len(systemParts) > 0 {
		req.SystemPrompt = strings.Join(systemParts, "\n")
	}
	if len(raw.Tools) > 0 {
		req.Tools = raw.Tools
	}
	return req, nil
}

// =============================================================================
