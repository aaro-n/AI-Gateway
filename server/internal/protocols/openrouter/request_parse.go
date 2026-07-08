package openrouter

import (
	"fmt"
	"encoding/json"
	"strings"
	"ai-gateway/internal/core/unified"
)

// =============================================================================
// ToUnified — OpenRouter 请求 → UnifiedRequest（OpenAI 兼容格式）
// =============================================================================

func (p *OpenRouterProvider) ToUnified(body []byte, modelID string) (*unified.Request, error) {
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
		return nil, fmt.Errorf("parse openrouter body: %w", err)
	}

	msgs := make([]unified.Message, 0, len(raw.Messages))
	systemParts := make([]string, 0)
	for _, rawMsg := range raw.Messages {
		var rawMap map[string]json.RawMessage
		if err := json.Unmarshal(rawMsg, &rawMap); err != nil {
			return nil, fmt.Errorf("parse message: %w", err)
		}
		var m unified.Message
		if err := json.Unmarshal(rawMsg, &m); err != nil {
			return nil, fmt.Errorf("parse message: %w", err)
		}
		if rc, ok := rawMap["reasoning_content"]; ok {
			var s string
			if json.Unmarshal(rc, &s) == nil {
				m.ReasoningContent = s
			}
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
		SourceProtocol:   "openrouter",
	}
	if len(systemParts) > 0 {
		req.SystemPrompt = strings.Join(systemParts, "\n")
	}
	if len(raw.Tools) > 0 {
		req.Tools = raw.Tools
	}
	return req, nil
}


