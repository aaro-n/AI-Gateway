package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/registry"
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
// HandleNative — Gemini 原生直通（只替换模型名，不改请求体格式）
// =============================================================================

func (p *GeminiProvider) HandleNative(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	// 判断是 generateContent 还是 streamGenerateContent
	method := "generateContent"
	if strings.Contains(ctx.Request.URL.Path, "streamGenerateContent") {
		method = "streamGenerateContent"
	}

	url := fmt.Sprintf("%s/models/%s:%s", p.cfg.BaseURL, modelID, method)
	if ctx.Request.URL.RawQuery != "" {
		url = url + "?" + ctx.Request.URL.RawQuery
		if !strings.Contains(url, "key=") {
			url = url + "&key=" + p.cfg.APIKey
		}
	} else {
		url = url + "?key=" + p.cfg.APIKey
	}

	req, err := http.NewRequestWithContext(ctx.Request.Context(), "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		ctx.Status(resp.StatusCode)
		ctx.Writer.Write(respBody)
		return fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// 透传响应
	ctx.Status(resp.StatusCode)
	for k, vv := range resp.Header {
		for _, v := range vv {
			ctx.Header(k, v)
		}
	}
	if method == "streamGenerateContent" {
		return p.copyGeminiStream(ctx.Request.Context(), ctx.Writer, resp.Body, usage)
	}
	return p.copyGeminiResponse(ctx.Writer, resp.Body, usage)
}

// =============================================================================
// FromOpenAI — OpenAI 请求 → Gemini 格式，发送，响应转回 OpenAI
// =============================================================================

func (p *GeminiProvider) FromOpenAI(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	var openAIReq struct {
		Model       string                   `json:"model"`
		Messages    []map[string]interface{} `json:"messages"`
		Stream      bool                     `json:"stream"`
		Temperature *float64                 `json:"temperature,omitempty"`
		MaxTokens   *int                     `json:"max_tokens,omitempty"`
	}
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		return fmt.Errorf("parse body: %w", err)
	}

	geminiReq := p.convertOpenAIToGemini(openAIReq.Messages, openAIReq.Temperature, openAIReq.MaxTokens)

	geminiBody, _ := json.Marshal(geminiReq)

	method := "generateContent"
	if openAIReq.Stream {
		method = "streamGenerateContent"
	}

	url := fmt.Sprintf("%s/models/%s:%s?key=%s", p.cfg.BaseURL, modelID, method, p.cfg.APIKey)
	if openAIReq.Stream {
		url = url + "&alt=sse"
	}

	req, err := http.NewRequestWithContext(ctx.Request.Context(), "POST", url, bytes.NewReader(geminiBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		ctx.Status(resp.StatusCode)
		ctx.Writer.Write(respBody)
		return fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if openAIReq.Stream {
		ctx.Status(http.StatusOK)
		ctx.Header("Content-Type", "text/event-stream")
		ctx.Header("Cache-Control", "no-cache")
		ctx.Header("Connection", "keep-alive")
		return p.streamGeminiToOpenAI(ctx.Request.Context(), resp.Body, ctx.Writer, modelID, usage)
	}

	body, _ = io.ReadAll(resp.Body)
	openAIResp, err := p.convertGeminiResponseToOpenAI(body, modelID, usage)
	if err != nil {
		return err
	}
	ctx.Status(http.StatusOK)
	ctx.Header("Content-Type", "application/json")
	ctx.Writer.Write(openAIResp)
	return nil
}

// =============================================================================
// 转换：OpenAI → Gemini
// =============================================================================

func (p *GeminiProvider) convertOpenAIToGemini(messages []map[string]interface{}, temperature *float64, maxTokens *int) map[string]interface{} {
	systemInstruction := ""
	contents := make([]map[string]interface{}, 0)

	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content := msg["content"]

		if role == "system" {
			if s, ok := content.(string); ok {
				systemInstruction += s + "\n"
			}
			continue
		}

		geminiRole := "user"
		if role == "assistant" || role == "model" {
			geminiRole = "model"
		}

		parts := p.convertContentToParts(content)
		contents = append(contents, map[string]interface{}{
			"role":  geminiRole,
			"parts": parts,
		})
	}

	req := map[string]interface{}{
		"contents": contents,
	}
	if systemInstruction != "" {
		req["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{{"text": strings.TrimSpace(systemInstruction)}},
		}
	}
	generationConfig := map[string]interface{}{}
	if temperature != nil {
		generationConfig["temperature"] = *temperature
	}
	if maxTokens != nil {
		generationConfig["maxOutputTokens"] = *maxTokens
	} else {
		generationConfig["maxOutputTokens"] = 4096
	}
	req["generationConfig"] = generationConfig
	return req
}

func (p *GeminiProvider) convertContentToParts(content interface{}) []map[string]interface{} {
	parts := make([]map[string]interface{}, 0)

	switch v := content.(type) {
	case string:
		parts = append(parts, map[string]interface{}{"text": v})
	case []interface{}:
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				blockType, _ := block["type"].(string)
				switch blockType {
				case "text":
					if text, ok := block["text"].(string); ok {
						parts = append(parts, map[string]interface{}{"text": text})
					}
				case "image_url":
					if imgURL, ok := block["image_url"].(map[string]interface{}); ok {
						if url, ok := imgURL["url"].(string); ok {
							parts = append(parts, map[string]interface{}{
								"inlineData": map[string]interface{}{
									"mimeType": "image/jpeg",
									"data":     strings.TrimPrefix(url, "data:image/jpeg;base64,"),
								},
							})
						}
					}
				}
			}
		}
	}

	if len(parts) == 0 {
		parts = append(parts, map[string]interface{}{"text": fmt.Sprintf("%v", content)})
	}
	return parts
}

// =============================================================================
// 响应转换：Gemini → OpenAI
// =============================================================================

func (p *GeminiProvider) convertGeminiResponseToOpenAI(body []byte, modelID string, usage *registry.Usage) ([]byte, error) {
	var geminiResp struct {
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
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, err
	}

	usage.InputTokens = geminiResp.UsageMetadata.PromptTokenCount
	usage.OutputTokens = geminiResp.UsageMetadata.CandidatesTokenCount

	combined := ""
	if len(geminiResp.Candidates) > 0 {
		for _, part := range geminiResp.Candidates[0].Content.Parts {
			if part.Text != "" {
				combined += part.Text
			}
		}
	}

	finishReason := "stop"
	if len(geminiResp.Candidates) > 0 {
		if geminiResp.Candidates[0].FinishReason == "MAX_TOKENS" {
			finishReason = "length"
		}
	}

	resp := map[string]interface{}{
		"object": "chat.completion",
		"model":  modelID,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]interface{}{"role": "assistant", "content": combined},
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     geminiResp.UsageMetadata.PromptTokenCount,
			"completion_tokens": geminiResp.UsageMetadata.CandidatesTokenCount,
			"total_tokens":      geminiResp.UsageMetadata.TotalTokenCount,
		},
	}
	return json.Marshal(resp)
}

// =============================================================================
// 流式
// =============================================================================

func (p *GeminiProvider) copyGeminiStream(ctx context.Context, dst io.Writer, src io.Reader, usage *registry.Usage) error {
	reader := bufio.NewReader(src)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		fmt.Fprint(dst, line)
		if flusher, ok := dst.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	return nil
}

func (p *GeminiProvider) copyGeminiResponse(dst io.Writer, src io.Reader, usage *registry.Usage) error {
	body, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	dst.Write(body)
	return nil
}

func (p *GeminiProvider) streamGeminiToOpenAI(ctx context.Context, src io.Reader, dst io.Writer, modelID string, usage *registry.Usage) error {
	reader := bufio.NewReader(src)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "data:") {
			continue
		}
		data := strings.TrimPrefix(trimmed, "data:")
		data = strings.TrimSpace(data)

		var geminiChunk struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			UsageMetadata *struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		}

		if err := json.Unmarshal([]byte(data), &geminiChunk); err != nil {
			continue
		}

		if geminiChunk.UsageMetadata != nil {
			usage.InputTokens = geminiChunk.UsageMetadata.PromptTokenCount
			usage.OutputTokens = geminiChunk.UsageMetadata.CandidatesTokenCount
		}

		if len(geminiChunk.Candidates) > 0 && len(geminiChunk.Candidates[0].Content.Parts) > 0 {
			text := geminiChunk.Candidates[0].Content.Parts[0].Text
			if text != "" {
				chunk := map[string]interface{}{
					"object": "chat.completion.chunk",
					"model":  modelID,
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]interface{}{"content": text},
						},
					},
				}
				chunkJSON, _ := json.Marshal(chunk)
				fmt.Fprintf(dst, "data: %s\n\n", chunkJSON)
				if flusher, ok := dst.(http.Flusher); ok {
					flusher.Flush()
				}
			}
		}
	}
	return nil
}
