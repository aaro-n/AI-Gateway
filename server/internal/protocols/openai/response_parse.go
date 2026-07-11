package openai

import (
	"ai-gateway/internal/core/unified"
	"encoding/json"
)

func (p *OpenAIProvider) parseOpenAIResponse(body []byte) (*unified.Response, error) {
	var raw struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role             string             `json:"role"`
				Content          string             `json:"content"`
				ReasoningContent string             `json:"reasoning_content"`
				ToolCalls        []unified.ToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage openAIUsageRaw `json:"usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	uresp := &unified.Response{
		ID:    raw.ID,
		Model: raw.Model,
		Usage: unified.Usage{
			CachedTokens: raw.Usage.PromptTokensDetails.CachedTokens,
			InputTokens:  raw.Usage.PromptTokens - raw.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens: raw.Usage.CompletionTokens,
		},
	}
	if len(raw.Choices) > 0 {
		uresp.Content = raw.Choices[0].Message.Content
		uresp.ReasoningContent = raw.Choices[0].Message.ReasoningContent
		uresp.ToolCalls = raw.Choices[0].Message.ToolCalls
		uresp.FinishReason = raw.Choices[0].FinishReason
	}
	return uresp, nil
}

// =============================================================================
// 流式：OpenAI SSE → unified.StreamEvent chan
// =============================================================================
