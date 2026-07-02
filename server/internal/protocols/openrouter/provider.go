package openrouter

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

// OpenRouterProvider 实现 OpenRouter 协议（OpenAI 兼容 API）。
// 由于 OpenRouter 模型数量巨大，禁止自动同步模型列表。
// 用户需手动添加模型 ID，可通过 LookupModel 查询单个模型的详细信息。
type OpenRouterProvider struct {
	cfg *registry.Config
}

func NewOpenRouterProvider(cfg *registry.Config) *OpenRouterProvider {
	return &OpenRouterProvider{cfg: cfg}
}

// =============================================================================
// SyncModels — OpenRouter 禁止自动同步（模型太多）
// =============================================================================

func (p *OpenRouterProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	return nil, fmt.Errorf("OpenRouter does not support auto-sync: too many models. Please add models manually and use the model lookup endpoint to fetch individual model capabilities")
}

// =============================================================================
// LookupModel — 从 OpenRouter API 查询单个模型的详细信息
// =============================================================================

// OpenRouterModelInfo OpenRouter 模型信息 API 返回的数据结构
type OpenRouterModelInfo struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	ContextLength       int    `json:"context_length"`
	MaxCompletionTokens int    `json:"max_completion_tokens"`
	Pricing             struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	Architecture struct {
		Modality string `json:"modality"`
	} `json:"architecture"`
}

// LookupModel 查询指定模型的详细信息。
// 从 OpenRouter /models 列表接口获取数据并过滤指定模型。
func (p *OpenRouterProvider) LookupModel(modelID string) (*registry.ProviderModel, error) {
	allModels, err := p.listModels()
	if err != nil {
		return nil, err
	}

	for _, info := range allModels {
		if info.ID == modelID {
			return info.toProviderModel(), nil
		}
	}
	return nil, fmt.Errorf("model not found: %s", modelID)
}

// listModels 获取 OpenRouter 完整模型列表（内部方法）
func (p *OpenRouterProvider) listModels() ([]OpenRouterModelInfo, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("GET", p.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenRouter API error (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data []OpenRouterModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse models list: %w", err)
	}
	return result.Data, nil
}

// toProviderModel 将 OpenRouter 模型信息转换为统一的 ProviderModel
func (info *OpenRouterModelInfo) toProviderModel() *registry.ProviderModel {
	// 解析价格（OpenRouter 价格是字符串格式，如 "0.000005"）
	inputPrice := 0.0
	outputPrice := 0.0
	if info.Pricing.Prompt != "" {
		fmt.Sscanf(info.Pricing.Prompt, "%f", &inputPrice)
	}
	if info.Pricing.Completion != "" {
		fmt.Sscanf(info.Pricing.Completion, "%f", &outputPrice)
	}

	displayName := info.Name
	if displayName == "" {
		displayName = info.ID
	}

	contextWindow := info.ContextLength
	if contextWindow == 0 {
		contextWindow = 8192
	}

	maxOutput := info.MaxCompletionTokens
	if maxOutput == 0 {
		maxOutput = 4096
	}

	// 从 architecture.modality 判断能力
	// OpenRouter 的 modality 格式如 "text+image->text", "multimodal", "text"
	modality := strings.ToLower(info.Architecture.Modality)
	supportsVision := strings.Contains(modality, "image") || strings.Contains(modality, "multimodal")
	// 几乎所有 OpenRouter 上的对话模型都支持工具调用，无法从 API 精确判断时默认 true
	supportsTools := true

	return &registry.ProviderModel{
		ModelID:        info.ID,
		DisplayName:    displayName,
		OwnedBy:        extractOwnedBy(info.ID),
		ContextWindow:  contextWindow,
		MaxOutput:      maxOutput,
		InputPrice:     inputPrice,
		OutputPrice:    outputPrice,
		SupportsVision: supportsVision,
		SupportsTools:  supportsTools,
		SupportsStream: true,
		IsAvailable:    true,
		Source:         "manual",
	}
}

// extractOwnedBy 从 OpenRouter model ID 提取提供商名称
// 例如 "openai/gpt-4o" → "openai", "anthropic/claude-3" → "anthropic"
func extractOwnedBy(modelID string) string {
	if idx := strings.Index(modelID, "/"); idx > 0 {
		return modelID[:idx]
	}
	return "openrouter"
}

// LookupModelStatic 静态查找：不依赖 Provider 实例，直接调用 OpenRouter API
func LookupModelStatic(baseURL, apiKey, modelID string) (*registry.ProviderModel, error) {
	prov := NewOpenRouterProvider(&registry.Config{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
	})
	return prov.LookupModel(modelID)
}

// LookupModelsBatch 批量查找：一次获取模型列表，过滤多个模型 ID
func LookupModelsBatch(baseURL, apiKey string, modelIDs []string) ([]registry.ProviderModel, []map[string]string) {
	prov := NewOpenRouterProvider(&registry.Config{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
	})

	allModels, err := prov.listModels()
	if err != nil {
		// 全部失败
		errs := make([]map[string]string, len(modelIDs))
		for i, id := range modelIDs {
			errs[i] = map[string]string{"model_id": id, "error": err.Error()}
		}
		return nil, errs
	}

	// 建立索引
	modelMap := make(map[string]*OpenRouterModelInfo, len(allModels))
	for i := range allModels {
		modelMap[allModels[i].ID] = &allModels[i]
	}

	results := make([]registry.ProviderModel, 0, len(modelIDs))
	errors := make([]map[string]string, 0)

	for _, modelID := range modelIDs {
		info, ok := modelMap[modelID]
		if !ok {
			errors = append(errors, map[string]string{
				"model_id": modelID,
				"error":    fmt.Sprintf("model not found in OpenRouter catalog: %s", modelID),
			})
			continue
		}
		results = append(results, *info.toProviderModel())
	}

	return results, errors
}

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

// =============================================================================
// FromUnified — UnifiedRequest → OpenRouter 请求，发上游，返回统一响应
// =============================================================================

func (p *OpenRouterProvider) FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error) {
	openRouterReq := p.unifiedToOpenRouter(req)
	body, err := json.Marshal(openRouterReq)
	if err != nil {
		return nil, nil, err
	}

	httpReq, err := http.NewRequest("POST", p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HTTP-Referer", "https://ai-gateway.local")
	httpReq.Header.Set("X-Title", "AI Gateway")

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
		events := p.streamOpenRouterToUnified(resp.Body)
		return nil, events, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	uresp, err := p.parseOpenRouterResponse(respBody)
	return uresp, nil, err
}

func (p *OpenRouterProvider) unifiedToOpenRouter(req *unified.Request) map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(req.Messages)+1)
	if req.SystemPrompt != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": req.SystemPrompt,
		})
	}
	for _, m := range req.Messages {
		msg := map[string]interface{}{"role": m.Role}
		if len(m.Content) > 0 {
			msg["content"] = json.RawMessage(m.Content)
		}
		if m.ReasoningContent != "" {
			msg["reasoning_content"] = m.ReasoningContent
		}
		if m.Prefix {
			msg["prefix"] = true
		}
		if len(m.ToolCalls) > 0 {
			msg["tool_calls"] = m.ToolCalls
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		messages = append(messages, msg)
	}

	result := map[string]interface{}{
		"model":    req.Model,
		"messages": messages,
		"stream":   req.Stream,
	}
	if req.MaxTokens > 0 {
		result["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		result["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		result["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		result["top_k"] = *req.TopK
	}
	if req.Seed != nil {
		result["seed"] = *req.Seed
	}
	if req.FrequencyPenalty != nil {
		result["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		result["presence_penalty"] = *req.PresencePenalty
	}
	if req.ReasoningBudget != nil {
		result["reasoning_budget"] = *req.ReasoningBudget
	}
	if len(req.Modalities) > 0 {
		result["modalities"] = req.Modalities
	}
	if len(req.Tools) > 0 {
		result["tools"] = unified.RawJSON(req.Tools)
	}
	if len(req.ToolChoice) > 0 {
		result["tool_choice"] = unified.RawJSON(req.ToolChoice)
	}
	if len(req.ResponseFormat) > 0 {
		result["response_format"] = unified.RawJSON(req.ResponseFormat)
	}
	if len(req.Stop) > 0 {
		result["stop"] = req.Stop
	}
	if req.ReasoningEffort != "" {
		result["reasoning_effort"] = req.ReasoningEffort
	}
	if req.Stream {
		result["stream_options"] = map[string]bool{"include_usage": true}
	}
	return result
}

// =============================================================================
// FormatUnified — Unified 响应/流 → OpenRouter/OpenAI 客户端格式
// =============================================================================

func (p *OpenRouterProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		usage.InputTokens = resp.Usage.InputTokens
		usage.OutputTokens = resp.Usage.OutputTokens
		usage.CachedTokens = resp.Usage.CachedTokens

		openAIResp := map[string]interface{}{
			"id":     resp.ID,
			"object": "chat.completion",
			"model":  resp.Model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       p.buildOpenRouterMessage(resp),
					"finish_reason": resp.FinishReason,
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     resp.Usage.InputTokens,
				"completion_tokens": resp.Usage.OutputTokens,
				"total_tokens":      resp.Usage.TotalTokens(),
			},
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		body, _ := json.Marshal(openAIResp)
		_, err := c.Writer.Write(body)
		return err
	}

	// 流式 SSE
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var id string
	for ev := range events {
		switch ev.Type {
		case unified.EventChunk:
			if ev.Delta != nil {
				if id == "" {
					id = "chatcmpl-openrouter"
				}
				deltaMap := map[string]interface{}{}
				if ev.Delta.Role != "" {
					deltaMap["role"] = ev.Delta.Role
				}
				if ev.Delta.Content != "" {
					deltaMap["content"] = ev.Delta.Content
				}
				if ev.Delta.ReasoningContent != "" {
					deltaMap["reasoning_content"] = ev.Delta.ReasoningContent
				}
				if len(ev.Delta.ToolCalls) > 0 {
					deltaMap["tool_calls"] = ev.Delta.ToolCalls
				}
				chunk := map[string]interface{}{
					"id":     id,
					"object": "chat.completion.chunk",
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"delta":         deltaMap,
							"finish_reason": nil,
						},
					},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
			}
		case unified.EventUsage:
			if ev.Usage != nil {
				usage.InputTokens = ev.Usage.InputTokens
				usage.OutputTokens = ev.Usage.OutputTokens
				usage.CachedTokens = ev.Usage.CachedTokens
			}
		case unified.EventDone:
			chunk := map[string]interface{}{
				"id":     id,
				"object": "chat.completion.chunk",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]interface{}{},
						"finish_reason": ev.FinishReason,
					},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
			c.Writer.Flush()
		}
	}
	return nil
}

func (p *OpenRouterProvider) buildOpenRouterMessage(resp *unified.Response) map[string]interface{} {
	msg := map[string]interface{}{"role": "assistant"}
	if resp.Content != "" {
		msg["content"] = resp.Content
	}
	if resp.ReasoningContent != "" {
		msg["reasoning_content"] = resp.ReasoningContent
	}
	if len(resp.ToolCalls) > 0 {
		msg["tool_calls"] = resp.ToolCalls
	}
	return msg
}

// =============================================================================
// 内部：解析 OpenRouter 响应 → UnifiedResponse
// =============================================================================

type openRouterUsageRaw struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (p *OpenRouterProvider) parseOpenRouterResponse(body []byte) (*unified.Response, error) {
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
		Usage openRouterUsageRaw `json:"usage"`
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
// 流式：OpenRouter SSE → unified.StreamEvent chan
// =============================================================================

func (p *OpenRouterProvider) streamOpenRouterToUnified(body io.ReadCloser) <-chan unified.StreamEvent {
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
			if data == "[DONE]" {
				ch <- unified.StreamEvent{Type: unified.EventDone, FinishReason: "stop"}
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Role             string             `json:"role"`
						Content          string             `json:"content"`
						ReasoningContent string             `json:"reasoning_content"`
						ToolCalls        []unified.ToolCall `json:"tool_calls"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
				Usage *openRouterUsageRaw `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			// 发送 usage 事件
			if chunk.Usage != nil {
				ch <- unified.StreamEvent{
					Type: unified.EventUsage,
					Usage: &unified.Usage{
						CachedTokens: chunk.Usage.PromptTokensDetails.CachedTokens,
						InputTokens:  chunk.Usage.PromptTokens - chunk.Usage.PromptTokensDetails.CachedTokens,
						OutputTokens: chunk.Usage.CompletionTokens,
					},
				}
			}

			// 发送 delta 事件
			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				delta := &unified.Delta{
					Role:             choice.Delta.Role,
					Content:          choice.Delta.Content,
					ReasoningContent: choice.Delta.ReasoningContent,
					ToolCalls:        choice.Delta.ToolCalls,
				}
				ev := unified.StreamEvent{Type: unified.EventChunk, Delta: delta}
				if choice.FinishReason != nil {
					ev.FinishReason = *choice.FinishReason
				}
				ch <- ev
			}
		}
	}()
	return ch
}
