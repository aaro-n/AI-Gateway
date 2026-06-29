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

	"ai-gateway/internal/core/registry"
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

		// 合并 parts 为 content blocks
		var blocks []unified.ContentBlock
		var hasImage bool
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

		msgs = append(msgs, unified.Message{
			Role:    role,
			Content: rawMsg,
		})
	}

	req := &unified.Request{
		Model:          modelID,
		Messages:       msgs,
		SystemPrompt:   systemPrompt,
		Temperature:    raw.GenerationConfig.Temperature,
		TopP:           raw.GenerationConfig.TopP,
		Stop:           raw.GenerationConfig.StopSequences,
		Stream:         false, // Gemini 通过 URL 区分流式，ToUnified 无法感知，默认 false
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
					functionDeclarations = append(functionDeclarations, map[string]interface{}{
						"name":        t.Function.Name,
						"description": t.Function.Description,
						"parameters":  t.Function.Parameters,
					})
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
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
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
		var text string
		for _, part := range raw.Candidates[0].Content.Parts {
			text += part.Text
		}
		uresp.Content = text

		switch raw.Candidates[0].FinishReason {
		case "MAX_TOKENS":
			uresp.FinishReason = "length"
		case "STOP":
			uresp.FinishReason = "stop"
		default:
			uresp.FinishReason = "stop"
		}
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
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
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
				for _, part := range chunk.Candidates[0].Content.Parts {
					if part.Text != "" {
						ch <- unified.StreamEvent{
							Type:  unified.EventChunk,
							Delta: &unified.Delta{Content: part.Text},
						}
					}
				}
				if chunk.Candidates[0].FinishReason != "" {
					finishReason := "stop"
					if chunk.Candidates[0].FinishReason == "MAX_TOKENS" {
						finishReason = "length"
					}
					ch <- unified.StreamEvent{
						Type:         unified.EventDone,
						FinishReason: finishReason,
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

		finishReason := "STOP"
		if resp.FinishReason == "length" {
			finishReason = "MAX_TOKENS"
		}

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
			finishReason := "STOP"
			if ev.FinishReason == "length" {
				finishReason = "MAX_TOKENS"
			}
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
