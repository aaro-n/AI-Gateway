package gemini

import (
	"encoding/json"

	"github.com/google/uuid"

	"ai-gateway/internal/core/reasonmap"
	"ai-gateway/internal/core/unified"
)

// parseGeminiResponse 解析 Gemini 非流式响应
func (p *GeminiProvider) parseGeminiResponse(body []byte) (*unified.Response, error) {
	var raw struct {
		Candidates []struct {
			Content struct {
				Role  string            `json:"role"`
				Parts []json.RawMessage `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	uresp := &unified.Response{
		Usage: unified.Usage{
			InputTokens:  raw.UsageMetadata.PromptTokenCount,
			OutputTokens: raw.UsageMetadata.CandidatesTokenCount,
		},
	}

	if len(raw.Candidates) > 0 {
		var textContent string
		var reasoningContent string
		var toolCalls []unified.ToolCall
		var reasoningSig *string
		for _, partRaw := range raw.Candidates[0].Content.Parts {
			var part map[string]interface{}
			if json.Unmarshal(partRaw, &part) != nil {
				continue
			}
			if text, ok := part["text"].(string); ok {
				textContent += text
			}
			if thought, ok := part["thought"].(string); ok && thought != "" {
				reasoningContent += thought
			}
			if sig, ok := part["thoughtSignature"].(string); ok && sig != "" {
				reasoningSig = &sig
			}
			if fc, ok := part["functionCall"].(map[string]interface{}); ok {
				name, _ := fc["name"].(string)
				argsJSON, _ := json.Marshal(fc["args"])
				toolCalls = append(toolCalls, unified.ToolCall{
					ID:       "call_" + uuid.New().String()[:8],
					Type:     "function",
					Function: unified.FunctionCall{Name: name, Arguments: string(argsJSON)},
				})
			}
		}
		uresp.Content = textContent
		uresp.ReasoningContent = reasoningContent
		uresp.ReasoningSignature = reasoningSig
		uresp.ToolCalls = toolCalls
		uresp.FinishReason = reasonmap.GeminiToUnified(raw.Candidates[0].FinishReason)
	}
	return uresp, nil
}
