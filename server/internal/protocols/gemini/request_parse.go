package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"ai-gateway/internal/core/unified"
)

// ToUnified 将 Gemini 请求体解析为 UnifiedRequest
func (p *GeminiProvider) ToUnified(body []byte, modelID string) (*unified.Request, error) {
	var raw struct {
		Contents          []json.RawMessage `json:"contents"`
		SystemInstruction json.RawMessage   `json:"systemInstruction,omitempty"`
		GenerationConfig  struct {
			Temperature     *float64 `json:"temperature,omitempty"`
			TopP            *float64 `json:"topP,omitempty"`
			MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
			StopSequences   []string `json:"stopSequences,omitempty"`
		} `json:"generationConfig,omitempty"`
		Tools      json.RawMessage `json:"tools,omitempty"`
		ToolConfig json.RawMessage `json:"toolConfig,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse gemini body: %w", err)
	}

	systemPrompt := ""
	if len(raw.SystemInstruction) > 0 {
		var si struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}
		if json.Unmarshal(raw.SystemInstruction, &si) == nil {
			var sb strings.Builder
			for _, part := range si.Parts {
				sb.WriteString(part.Text)
			}
			systemPrompt = sb.String()
		}
	}

	msgs := make([]unified.Message, 0, len(raw.Contents))
	for _, rawContent := range raw.Contents {
		var c struct {
			Role  string            `json:"role"`
			Parts []json.RawMessage `json:"parts"`
		}
		if err := json.Unmarshal(rawContent, &c); err != nil {
			return nil, fmt.Errorf("parse gemini content: %w", err)
		}
		role := "user"
		if c.Role == "model" {
			role = "assistant"
		}

		var blocks []unified.ContentBlock
		var hasImage bool
		var toolCalls []unified.ToolCall
		for _, partRaw := range c.Parts {
			var part map[string]interface{}
			if json.Unmarshal(partRaw, &part) != nil {
				continue
			}
			if text, ok := part["text"].(string); ok {
				blocks = append(blocks, unified.ContentBlock{Type: "text", Text: text})
			} else if inlineData, ok := part["inlineData"].(map[string]interface{}); ok {
				mimeType, _ := inlineData["mimeType"].(string)
				data, _ := inlineData["data"].(string)
				if mimeType != "" && data != "" {
					hasImage = true
					blocks = append(blocks, unified.ContentBlock{
						Type:     "image_url",
						ImageURL: &unified.ImageURL{URL: fmt.Sprintf("data:%s;base64,%s", mimeType, data)},
					})
				}
			} else if fc, ok := part["functionCall"].(map[string]interface{}); ok {
				name, _ := fc["name"].(string)
				if args, ok := fc["args"]; ok {
					argsJSON, _ := json.Marshal(args)
					toolCalls = append(toolCalls, unified.ToolCall{
						ID:       fmt.Sprintf("call_%s", name),
						Type:     "function",
						Function: unified.FunctionCall{Name: name, Arguments: string(argsJSON)},
					})
				}
			} else if fr, ok := part["functionResponse"].(map[string]interface{}); ok {
				name, _ := fr["name"].(string)
				respData, _ := json.Marshal(fr["response"])
				msgs = append(msgs, unified.Message{
					Role:       "tool",
					ToolCallID: fmt.Sprintf("call_%s", name),
					Content:    respData,
				})
			}
		}

		var rawMsg json.RawMessage
		if hasImage {
			rawMsg = unified.BlocksContent(blocks)
		} else {
			var textParts []string
			for _, b := range blocks {
				if b.Type == "text" {
					textParts = append(textParts, b.Text)
				}
			}
			rawMsg = unified.StringContent(strings.Join(textParts, "\n"))
		}

		um := unified.Message{Role: role, Content: rawMsg}
		if len(toolCalls) > 0 {
			um.ToolCalls = toolCalls
		}
		msgs = append(msgs, um)
	}

	req := &unified.Request{
		Model:          modelID,
		Messages:       msgs,
		SystemPrompt:   systemPrompt,
		Temperature:    raw.GenerationConfig.Temperature,
		TopP:           raw.GenerationConfig.TopP,
		Stop:           raw.GenerationConfig.StopSequences,
		Stream:         false,
		SourceProtocol: "gemini",
	}
	if raw.GenerationConfig.MaxOutputTokens != nil {
		req.MaxTokens = *raw.GenerationConfig.MaxOutputTokens
	}
	if len(raw.Tools) > 0 {
		var geminiTools []struct {
			FunctionDeclarations []struct {
				Name        string          `json:"name"`
				Description string          `json:"description,omitempty"`
				Parameters  json.RawMessage `json:"parameters,omitempty"`
			} `json:"functionDeclarations"`
		}
		if json.Unmarshal(raw.Tools, &geminiTools) == nil {
			var unifiedTools []unified.Tool
			for _, gt := range geminiTools {
				for _, fd := range gt.FunctionDeclarations {
					unifiedTools = append(unifiedTools, unified.Tool{
						Type: "function",
						Function: unified.FunctionDef{
							Name: fd.Name, Description: fd.Description, Parameters: fd.Parameters,
						},
					})
				}
			}
			if b, err := json.Marshal(unifiedTools); err == nil {
				req.Tools = b
			}
		}
	}
	if len(raw.ToolConfig) > 0 {
		req.ToolChoice = raw.ToolConfig
	}
	return req, nil
}
