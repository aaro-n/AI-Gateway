package openai

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

type OpenAIProvider struct {
	cfg *registry.Config
}

func NewOpenAIProvider(cfg *registry.Config) *OpenAIProvider {
	return &OpenAIProvider{cfg: cfg}
}

// =============================================================================
// SyncModels — 从 OpenAI API 同步模型列表
// =============================================================================

type openAIModelEntry struct {
	ID                  string `json:"id"`
	OwnedBy             string `json:"owned_by"`
	DisplayName         string `json:"display_name"`          // 某些兼容 API 返回
	ContextLength       int    `json:"context_length"`        // 上下文窗口
	MaxInputTokens      int    `json:"max_input_tokens"`      // 别名
	MaxTokens           int    `json:"max_tokens"`            // 最大输出（别名）
	MaxCompletionTokens int    `json:"max_completion_tokens"` // 最大输出（OpenAI 标准）
	Pricing             struct {
		Completion float64 `json:"completion"`
		Prompt     float64 `json:"prompt"`
	} `json:"pricing"`
	Capabilities *struct {
		Vision    bool `json:"vision"`
		Streaming bool `json:"streaming"`
	} `json:"capabilities,omitempty"`
}

// openAIModelDefaults 为 OpenAI 标准 API 不返回 context_window 的情况提供默认值。
// 数据来源：https://platform.openai.com/docs/models (Core Generative Models)
// 不在本表中的模型同步时会被过滤舍弃（非核心对话模型如音频/图像/embedding 等）。
var openAIModelDefaults = map[string]struct {
	contextWindow int
	maxOutput     int
	vision        bool
}{
	// GPT-5.2 系列
	"gpt-5.2-pro": {400000, 128000, true},
	"gpt-5.2":     {400000, 128000, true},
	// GPT-5.1 系列
	"gpt-5.1":           {400000, 128000, true},
	"gpt-5.1-codex-max": {400000, 128000, true},
	"gpt-5.1-codex":     {400000, 128000, true},
	// GPT-5 系列
	"gpt-5":       {400000, 128000, true},
	"gpt-5-codex": {400000, 128000, true},
	"gpt-5-mini":  {400000, 128000, true},
	"gpt-5-nano":  {400000, 128000, true},
	// GPT-4.1 系列
	"gpt-4.1":      {1047576, 32768, true},
	"gpt-4.1-mini": {1047576, 32768, true},
	"gpt-4.1-nano": {1047576, 32768, true},
	// GPT-4o 系列
	"gpt-4o":      {128000, 16384, true},
	"gpt-4o-mini": {128000, 16384, true},
	// GPT-4 系列
	"gpt-4":       {8192, 8192, false},
	"gpt-4-turbo": {128000, 4096, true},
	// GPT-3.5
	"gpt-3.5-turbo": {16385, 4096, false},
	// o 系列推理模型
	"o1":         {200000, 100000, true},
	"o1-mini":    {128000, 65536, false},
	"o1-preview": {128000, 32768, false},
	"o1-pro":     {200000, 100000, true},
	"o3":         {200000, 100000, true},
	"o3-mini":    {200000, 100000, false},
	"o4-mini":    {200000, 100000, true},
}

func (p *OpenAIProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", p.cfg.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API error: %s", string(body))
	}

	var result struct {
		Data []openAIModelEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]registry.ProviderModel, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID == "" {
			continue
		}
		// 跳过非 chat 模型（embedding、moderation、tts 等）
		if strings.Contains(m.ID, "embedding") || strings.Contains(m.ID, "moderation") ||
			strings.Contains(m.ID, "whisper") || strings.Contains(m.ID, "tts") ||
			strings.Contains(m.ID, "dall-e") || strings.Contains(m.ID, "davinci") {
			continue
		}

		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.ID
		}

		// 上下文窗口：多种字段名兼容
		contextWindow := m.ContextLength
		if contextWindow == 0 {
			contextWindow = m.MaxInputTokens
		}

		// 最大输出：优先 max_completion_tokens，其次 max_tokens
		maxOutput := m.MaxCompletionTokens
		if maxOutput == 0 {
			maxOutput = m.MaxTokens
		}

		// 视觉支持：从 capabilities 读取
		supportsVision := false
		if m.Capabilities != nil {
			supportsVision = m.Capabilities.Vision
		}

		// 核心对话模型白名单：用官方默认值填充 API 未返回的字段，
		// 不在白名单中的模型（音频/图像/embedding/旧版快照等）直接舍弃
		def, inDefaults := openAIModelDefaults[m.ID]
		if !inDefaults {
			continue
		}

		if contextWindow == 0 {
			contextWindow = def.contextWindow
		}
		if maxOutput == 0 {
			maxOutput = def.maxOutput
		}
		if !supportsVision {
			supportsVision = def.vision
		}

		models = append(models, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.ID,
			DisplayName:    displayName,
			OwnedBy:        m.OwnedBy,
			ContextWindow:  contextWindow,
			MaxOutput:      maxOutput,
			InputPrice:     m.Pricing.Prompt,
			OutputPrice:    m.Pricing.Completion,
			SupportsVision: supportsVision,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return models, nil
}

// =============================================================================
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
		// 先解析为 map 保留 unknown fields（reasoning_content 等）
		var rawMap map[string]json.RawMessage
		if err := json.Unmarshal(rawMsg, &rawMap); err != nil {
			return nil, fmt.Errorf("parse message: %w", err)
		}
		var m unified.Message
		if err := json.Unmarshal(rawMsg, &m); err != nil {
			return nil, fmt.Errorf("parse message: %w", err)
		}
		// 提取 reasoning_content（o1/o3 思维链，多轮对话须原样传回）
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
// FromUnified — UnifiedRequest → OpenAI 请求，发上游，返回统一响应
// =============================================================================

func (p *OpenAIProvider) FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error) {
	openAIReq := p.unifiedToOpenAI(req)
	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, nil, err
	}

	httpReq, err := http.NewRequest("POST", p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
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
		events := p.streamOpenAIToUnified(resp.Body)
		return nil, events, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	uresp, err := p.parseOpenAIResponse(respBody)
	return uresp, nil, err
}

// unifiedToOpenAI 将 UnifiedRequest 转回 OpenAI 请求体
func (p *OpenAIProvider) unifiedToOpenAI(req *unified.Request) map[string]interface{} {
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
		// 还原 reasoning_content（DeepSeek/o1 思维链，多轮对话必须传回）
		if m.ReasoningContent != "" {
			msg["reasoning_content"] = m.ReasoningContent
		}
		// 还原 prefix（DeepSeek Chat Prefix Completion）
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
		result["max_completion_tokens"] = req.MaxTokens
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
// FormatUnified — Unified 响应/流 → OpenAI 客户端格式
// =============================================================================

func (p *OpenAIProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		// 非流式
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
					"message":       p.buildOpenAIMessage(resp),
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

	// 流式
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
					id = "chatcmpl-unified"
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

func (p *OpenAIProvider) buildOpenAIMessage(resp *unified.Response) map[string]interface{} {
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
// 内部：解析 OpenAI 响应 → UnifiedResponse
// =============================================================================

type openAIUsageRaw struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

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

func (p *OpenAIProvider) streamOpenAIToUnified(body io.ReadCloser) <-chan unified.StreamEvent {
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
				ch <- unified.StreamEvent{Type: unified.EventDone}
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
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
				Usage *openAIUsageRaw `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
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
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" || len(delta.ToolCalls) > 0 || delta.Role != "" || delta.ReasoningContent != "" {
					ch <- unified.StreamEvent{
						Type: unified.EventChunk,
						Delta: &unified.Delta{
							Role:             delta.Role,
							Content:          delta.Content,
							ReasoningContent: delta.ReasoningContent,
							ToolCalls:        delta.ToolCalls,
						},
					}
				}
				if chunk.Choices[0].FinishReason != "" {
					ch <- unified.StreamEvent{
						Type:         unified.EventDone,
						FinishReason: chunk.Choices[0].FinishReason,
					}
				}
			}
		}
	}()
	return ch
}
