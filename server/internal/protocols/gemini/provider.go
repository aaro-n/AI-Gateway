package gemini

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/reasonmap"
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/schemaclean"
	"ai-gateway/internal/core/unified"
)

type GeminiProvider struct {
	cfg *registry.Config
}

func NewGeminiProvider(cfg *registry.Config) *GeminiProvider {
	return &GeminiProvider{cfg: cfg}
}

// =============================================================================
// SyncModels
// =============================================================================

type geminiRawModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

func (p *GeminiProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	url := fmt.Sprintf("%s/models?key=%s", p.cfg.BaseURL, p.cfg.APIKey)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gemini API error: %s", string(body))
	}

	var result struct {
		Models []geminiRawModel `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]registry.ProviderModel, 0, len(result.Models))
	for _, m := range result.Models {
		supportsGenerate := false
		for _, method := range m.SupportedGenerationMethods {
			if method == "generateContent" {
				supportsGenerate = true
				break
			}
		}
		if !supportsGenerate || m.Name == "" {
			continue
		}
		modelID := strings.TrimPrefix(m.Name, "models/")
		models = append(models, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        modelID,
			DisplayName:    m.DisplayName,
			OwnedBy:        "google",
			ContextWindow:  m.InputTokenLimit,
			MaxOutput:      m.OutputTokenLimit,
			SupportsVision: true,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return models, nil
}

// =============================================================================
// ToUnified — Gemini 请求 → UnifiedRequest
// =============================================================================

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

	// systemInstruction
	systemPrompt := ""
	if len(raw.SystemInstruction) > 0 {
		var si struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		}
		if json.Unmarshal(raw.SystemInstruction, &si) == nil {
			for _, part := range si.Parts {
				systemPrompt += part.Text
			}
		}
	}

	// contents → messages
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

		// 合并 parts 为 content blocks — 支持 text / inlineData / functionCall / functionResponse
		var blocks []unified.ContentBlock
		var hasImage bool
		var toolCalls []unified.ToolCall
		for _, partRaw := range c.Parts {
			var part map[string]interface{}
			if json.Unmarshal(partRaw, &part) != nil {
				continue
			}
			if text, ok := part["text"].(string); ok {
				blocks = append(blocks, unified.ContentBlock{
					Type: "text",
					Text: text,
				})
			} else if inlineData, ok := part["inlineData"].(map[string]interface{}); ok {
				mimeType, _ := inlineData["mimeType"].(string)
				data, _ := inlineData["data"].(string)
				if mimeType != "" && data != "" {
					hasImage = true
					dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, data)
					blocks = append(blocks, unified.ContentBlock{
						Type: "image_url",
						ImageURL: &unified.ImageURL{
							URL: dataURL,
						},
					})
				}
			} else if fc, ok := part["functionCall"].(map[string]interface{}); ok {
				// Gemini functionCall → OpenAI tool_calls
				name, _ := fc["name"].(string)
				if args, ok := fc["args"]; ok {
					argsJSON, _ := json.Marshal(args)
					toolCalls = append(toolCalls, unified.ToolCall{
						ID:   fmt.Sprintf("call_%s", name),
						Type: "function",
						Function: unified.FunctionCall{
							Name:      name,
							Arguments: string(argsJSON),
						},
					})
				}
			} else if fr, ok := part["functionResponse"].(map[string]interface{}); ok {
				// Gemini functionResponse → OpenAI tool role message
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

		um := unified.Message{
			Role:    role,
			Content: rawMsg,
		}
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
		Stream:         false, // Gemini 通过 URL 区分流式；调用方应从 URL streamGenerateContent 检测并补设
		SourceProtocol: "gemini",
	}
	if raw.GenerationConfig.MaxOutputTokens != nil {
		req.MaxTokens = *raw.GenerationConfig.MaxOutputTokens
	}
	// 转换 Gemini tools → unified Tools (透传 json.RawMessage，保留所有字段)
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
							Name:        fd.Name,
							Description: fd.Description,
							Parameters:  fd.Parameters,
						},
					})
				}
			}
			if b, err := json.Marshal(unifiedTools); err == nil {
				req.Tools = b
			}
		}
	}
	// 转换 Gemini toolConfig → unified ToolChoice (raw JSON pass-through)
	if len(raw.ToolConfig) > 0 {
		req.ToolChoice = raw.ToolConfig
	}
	return req, nil
}

// =============================================================================
// FromUnified — UnifiedRequest → Gemini 请求，发上游，返回统一响应
// =============================================================================

func (p *GeminiProvider) FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error) {
	geminiReq := p.unifiedToGemini(req)
	body, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, nil, err
	}

	method := "generateContent"
	if req.Stream {
		method = "streamGenerateContent"
	}
	url := fmt.Sprintf("%s/models/%s:%s?key=%s", p.cfg.BaseURL, req.Model, method, p.cfg.APIKey)
	if req.Stream {
		url += "&alt=sse"
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, nil, &registry.HTTPError{StatusCode: resp.StatusCode, Body: respBody}
	}

	if req.Stream {
		events := p.streamGeminiToUnified(resp.Body)
		return nil, events, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	uresp, err := p.parseGeminiResponse(respBody)
	return uresp, nil, err
}

// unifiedToGemini 将 UnifiedRequest 转为 Gemini 请求体
func (p *GeminiProvider) unifiedToGemini(req *unified.Request) map[string]interface{} {
	contents := make([]map[string]interface{}, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			continue
		}
		geminiRole := "user"
		if m.Role == "assistant" {
			geminiRole = "model"
		}
		parts := p.unifiedContentToGeminiParts(m)
		// assistant tool_calls → functionCall parts
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var args interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = tc.Function.Arguments
				}
				parts = append(parts, map[string]interface{}{
					"functionCall": map[string]interface{}{
						"name": tc.Function.Name,
						"args": args,
					},
				})
			}
		}
		// tool role → functionResponse part
		if m.Role == "tool" {
			var resp interface{}
			if err := json.Unmarshal(m.Content, &resp); err != nil {
				resp = unified.ContentString(m.Content)
			}
			parts = append(parts, map[string]interface{}{
				"functionResponse": map[string]interface{}{
					"name":     m.Name,
					"response": resp,
				},
			})
			geminiRole = "user"
		}
		contents = append(contents, map[string]interface{}{
			"role":  geminiRole,
			"parts": parts,
		})
	}

	result := map[string]interface{}{
		"contents": contents,
	}
	if req.SystemPrompt != "" {
		result["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{{"text": req.SystemPrompt}},
		}
	}

	genConfig := map[string]interface{}{}
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		genConfig["topP"] = *req.TopP
	}
	if req.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = req.MaxTokens
	}
	if len(req.Stop) > 0 {
		genConfig["stopSequences"] = req.Stop
	}
	result["generationConfig"] = genConfig

	if len(req.Tools) > 0 {
		var unifiedTools []unified.Tool
		if err := json.Unmarshal(req.Tools, &unifiedTools); err == nil {
			functionDeclarations := make([]map[string]interface{}, 0, len(unifiedTools))
			for _, t := range unifiedTools {
				if t.Function.Name != "" {
					// 清理 JSON Schema 以兼容 Gemini（移除 $ref / oneOf / const 等）
					cleanParams := json.RawMessage(schemaclean.ForGemini(t.Function.Parameters))

					fd := map[string]interface{}{
						"name":        t.Function.Name,
						"description": t.Function.Description,
					}
					if len(cleanParams) > 0 && string(cleanParams) != "null" {
						var paramsObj interface{}
						if json.Unmarshal(cleanParams, &paramsObj) == nil {
							fd["parameters"] = paramsObj
						}
					}
					functionDeclarations = append(functionDeclarations, fd)
				}
			}
			if len(functionDeclarations) > 0 {
				result["tools"] = []map[string]interface{}{
					{"functionDeclarations": functionDeclarations},
				}
			}
		}
	}
	// 转换 ToolChoice → Gemini toolConfig
	if len(req.ToolChoice) > 0 {
		tc := string(req.ToolChoice)
		// 尝试识别常见值
		var mode string
		switch tc {
		case `"none"`, "none":
			mode = "NONE"
		case `"auto"`, "auto":
			mode = "AUTO"
		case `"required"`, "required":
			mode = "ANY"
		default:
			// 可能是 Gemini 原生的 toolConfig JSON，直接透传
			var tcMap map[string]interface{}
			if json.Unmarshal(req.ToolChoice, &tcMap) == nil {
				result["toolConfig"] = tcMap
			}
		}
		if mode != "" {
			result["toolConfig"] = map[string]interface{}{
				"functionCallingConfig": map[string]interface{}{"mode": mode},
			}
		}
	}
	return result
}

func (p *GeminiProvider) unifiedContentToGeminiParts(m unified.Message) []map[string]interface{} {
	parts := make([]map[string]interface{}, 0)

	if s := unified.ContentString(m.Content); s != "" {
		parts = append(parts, map[string]interface{}{"text": s})
		return parts
	}

	for _, b := range unified.ContentBlocks(m.Content) {
		switch b.Type {
		case "text":
			parts = append(parts, map[string]interface{}{"text": b.Text})
		case "image_url":
			if b.ImageURL != nil {
				url := b.ImageURL.URL
				if strings.HasPrefix(url, "data:") {
					mediaType, data := parseDataURL(url)
					parts = append(parts, map[string]interface{}{
						"inlineData": map[string]interface{}{
							"mimeType": mediaType,
							"data":     data,
						},
					})
				}
			}
		}
	}

	if len(parts) == 0 {
		parts = append(parts, map[string]interface{}{"text": ""})
	}
	return parts
}

func parseDataURL(url string) (mediaType, data string) {
	if idx := strings.Index(url, ";"); idx > 0 {
		mediaType = strings.TrimPrefix(url[:idx], "data:")
		if comma := strings.Index(url, ","); comma > 0 {
			data = url[comma+1:]
		}
	}
	return
}

// =============================================================================
// 解析 Gemini 响应 → UnifiedResponse
// =============================================================================

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
			// text
			if text, ok := part["text"].(string); ok {
				textContent += text
			}
			// thought (Gemini think part)
			if thought, ok := part["thought"].(string); ok && thought != "" {
				reasoningContent += thought
			}
			// thoughtSignature (Gemini 推理签名)
			if sig, ok := part["thoughtSignature"].(string); ok && sig != "" {
				reasoningSig = &sig
			}
			// functionCall → tool_calls
			if fc, ok := part["functionCall"].(map[string]interface{}); ok {
				name, _ := fc["name"].(string)
				argsJSON, _ := json.Marshal(fc["args"])
				toolCalls = append(toolCalls, unified.ToolCall{
					ID:   fmt.Sprintf("call_%s", name),
					Type: "function",
					Function: unified.FunctionCall{
						Name:      name,
						Arguments: string(argsJSON),
					},
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

// =============================================================================
// 流式：Gemini SSE → unified.StreamEvent chan
// =============================================================================

func (p *GeminiProvider) streamGeminiToUnified(body io.ReadCloser) <-chan unified.StreamEvent {
	ch := make(chan unified.StreamEvent, 32)
	go func() {
		defer body.Close()
		defer close(ch)
		reader := bufio.NewReader(body)
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

			var chunk struct {
				Candidates []struct {
					Content struct {
						Parts []json.RawMessage `json:"parts"`
					} `json:"content"`
					FinishReason string `json:"finishReason"`
				} `json:"candidates"`
				UsageMetadata *struct {
					PromptTokenCount     int `json:"promptTokenCount"`
					CandidatesTokenCount int `json:"candidatesTokenCount"`
				} `json:"usageMetadata"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			if chunk.UsageMetadata != nil {
				ch <- unified.StreamEvent{
					Type: unified.EventUsage,
					Usage: &unified.Usage{
						InputTokens:  chunk.UsageMetadata.PromptTokenCount,
						OutputTokens: chunk.UsageMetadata.CandidatesTokenCount,
					},
				}
			}

			if len(chunk.Candidates) > 0 {
				var hasText bool
				var trailingSig string
				for _, partRaw := range chunk.Candidates[0].Content.Parts {
					var part map[string]interface{}
					if json.Unmarshal(partRaw, &part) != nil {
						continue
					}
					// text / thought / thoughtSignature
					if text, ok := part["text"].(string); ok {
						if text != "" {
							ch <- unified.StreamEvent{
								Type:  unified.EventChunk,
								Delta: &unified.Delta{Content: text},
							}
							hasText = true
						}
					}
					if thought, ok := part["thought"].(string); ok && thought != "" {
						ch <- unified.StreamEvent{
							Type:  unified.EventChunk,
							Delta: &unified.Delta{ReasoningContent: thought},
						}
					}
					// thoughtSignature 单独出现（不含 text 的 part）
					if sig, ok := part["thoughtSignature"].(string); ok && sig != "" {
						if hasText {
							// text part 携带签名 → trailing signature（Sub2api 模式）
							trailingSig = sig
						} else {
							ch <- unified.StreamEvent{
								Type:  unified.EventChunk,
								Delta: &unified.Delta{ReasoningSignature: &sig},
							}
						}
					}
					// functionCall
					if fc, ok := part["functionCall"].(map[string]interface{}); ok {
						name, _ := fc["name"].(string)
						argsJSON, _ := json.Marshal(fc["args"])
						ch <- unified.StreamEvent{
							Type: unified.EventChunk,
							Delta: &unified.Delta{
								ToolCalls: []unified.ToolCall{{
									ID:   fmt.Sprintf("call_%s", name),
									Type: "function",
									Function: unified.FunctionCall{
										Name:      name,
										Arguments: string(argsJSON),
									},
								}},
							},
						}
					}
				}
				// trailing signature: text part 携带签名，text content 已发送后补发签名
				if trailingSig != "" {
					ch <- unified.StreamEvent{
						Type:  unified.EventChunk,
						Delta: &unified.Delta{ReasoningSignature: &trailingSig},
					}
				}
				if chunk.Candidates[0].FinishReason != "" {
					ch <- unified.StreamEvent{
						Type:         unified.EventDone,
						FinishReason: reasonmap.GeminiToUnified(chunk.Candidates[0].FinishReason),
					}
				}
			}
		}
	}()
	return ch
}

// =============================================================================
// FormatUnified — Unified 响应/流 → Gemini 客户端格式
// =============================================================================

func (p *GeminiProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		// 非流式
		usage.InputTokens = resp.Usage.InputTokens
		usage.OutputTokens = resp.Usage.OutputTokens

		parts := make([]map[string]interface{}, 0)
		if resp.Content != "" {
			parts = append(parts, map[string]interface{}{"text": resp.Content})
		}
		// tool_calls → functionCall parts
		for _, tc := range resp.ToolCalls {
			var args interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				args = tc.Function.Arguments
			}
			parts = append(parts, map[string]interface{}{
				"functionCall": map[string]interface{}{
					"name": tc.Function.Name,
					"args": args,
				},
			})
		}

		finishReason := reasonmap.UnifiedToGemini(resp.FinishReason)

		geminiResp := map[string]interface{}{
			"candidates": []map[string]interface{}{
				{
					"content": map[string]interface{}{
						"role":  "model",
						"parts": parts,
					},
					"finishReason": finishReason,
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":     resp.Usage.InputTokens,
				"candidatesTokenCount": resp.Usage.OutputTokens,
				"totalTokenCount":      resp.Usage.TotalTokens(),
			},
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		body, _ := json.Marshal(geminiResp)
		_, err := c.Writer.Write(body)
		return err
	}

	// 流式：Unified events → Gemini SSE
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var inputTokens, outputTokens int
	for ev := range events {
		switch ev.Type {
		case unified.EventChunk:
			if ev.Delta != nil && ev.Delta.Content != "" {
				chunk := map[string]interface{}{
					"candidates": []map[string]interface{}{
						{
							"content": map[string]interface{}{
								"role":  "model",
								"parts": []map[string]interface{}{{"text": ev.Delta.Content}},
							},
						},
					},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
			}
		case unified.EventUsage:
			if ev.Usage != nil {
				inputTokens = ev.Usage.InputTokens
				outputTokens = ev.Usage.OutputTokens
			}
		case unified.EventDone:
			finishReason := reasonmap.UnifiedToGemini(ev.FinishReason)
			chunk := map[string]interface{}{
				"candidates": []map[string]interface{}{
					{
						"content":      map[string]interface{}{"role": "model", "parts": []interface{}{}},
						"finishReason": finishReason,
					},
				},
				"usageMetadata": map[string]interface{}{
					"promptTokenCount":     inputTokens,
					"candidatesTokenCount": outputTokens,
					"totalTokenCount":      inputTokens + outputTokens,
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		}
	}
	usage.InputTokens = inputTokens
	usage.OutputTokens = outputTokens
	return nil
}
