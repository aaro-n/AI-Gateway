package deepseek

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
)

// thinkTagPattern matches `<think>...</think>` blocks in content.
// Used to strip/wrap reasoning_content when proxying to standard OpenAI clients.
var thinkTagPattern = regexp.MustCompile(`(?s)<think>(.*?)</think>`)

// stripThinkTag extracts reasoning text from `<think>...</think>` tags
// and returns the cleaned content without the tag.
func stripThinkTag(content string) (reasoning string, cleanContent string) {
	matches := thinkTagPattern.FindStringSubmatch(content)
	if matches == nil {
		return "", content
	}
	reasoning = strings.TrimSpace(matches[1])
	cleanContent = strings.TrimSpace(thinkTagPattern.ReplaceAllString(content, ""))
	return reasoning, cleanContent
}

// wrapThinkTag wraps reasoning content in `<think>...</think>` and prepends to content.
func wrapThinkTag(reasoning, content string) string {
	if reasoning == "" {
		return content
	}
	return "<think>" + reasoning + "</think>" + "\n" + content
}

type DeepSeekProvider struct {
	cfg *registry.Config
}

func NewDeepSeekProvider(cfg *registry.Config) *DeepSeekProvider {
	return &DeepSeekProvider{cfg: cfg}
}

// =============================================================================
// SyncModels — 从 DeepSeek API 同步模型列表
// =============================================================================

type deepseekModelEntry struct {
	ID                  string `json:"id"`
	OwnedBy             string `json:"owned_by"`
	DisplayName         string `json:"display_name"`
	ContextLength       int    `json:"context_length"`
	MaxInputTokens      int    `json:"max_input_tokens"`
	MaxTokens           int    `json:"max_tokens"`
	MaxCompletionTokens int    `json:"max_completion_tokens"`
}

func (p *DeepSeekProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
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
		return nil, fmt.Errorf("DeepSeek API error: %s", string(body))
	}

	var result struct {
		Data []deepseekModelEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]registry.ProviderModel, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID == "" {
			continue
		}
		// 跳过非 chat 模型
		if strings.Contains(m.ID, "embedding") || strings.Contains(m.ID, "moderation") ||
			strings.Contains(m.ID, "whisper") || strings.Contains(m.ID, "tts") {
			continue
		}

		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.ID
		}

		contextWindow := m.ContextLength
		if contextWindow == 0 {
			contextWindow = m.MaxInputTokens
		}

		maxOutput := m.MaxCompletionTokens
		if maxOutput == 0 {
			maxOutput = m.MaxTokens
		}

		// 兜底：API 未返回时使用已知模型默认值
		if contextWindow == 0 || maxOutput == 0 {
			if def, ok := deepseekModelDefaults[m.ID]; ok {
				if contextWindow == 0 {
					contextWindow = def.contextWindow
				}
				if maxOutput == 0 {
					maxOutput = def.maxOutput
				}
			}
		}

		models = append(models, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.ID,
			DisplayName:    displayName,
			OwnedBy:        m.OwnedBy,
			ContextWindow:  contextWindow,
			MaxOutput:      maxOutput,
			SupportsVision: detectDeepSeekVision(m.ID),
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return models, nil
}

// =============================================================================
// ToUnified — DeepSeek 请求 → UnifiedRequest
// 关键：保留 assistant 消息中的 reasoning_content（多轮对话必须原样传回，否则报 400）
func (p *DeepSeekProvider) ToUnified(body []byte, modelID string) (*unified.Request, error) {
	var raw struct {
		Model               string            `json:"model"`
		Messages            []json.RawMessage `json:"messages"`
		MaxTokens           int               `json:"max_tokens"`
		MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
		Temperature         *float64          `json:"temperature,omitempty"`
		TopP                *float64          `json:"top_p,omitempty"`
		FrequencyPenalty    *float64          `json:"frequency_penalty,omitempty"`
		PresencePenalty     *float64          `json:"presence_penalty,omitempty"`
		Stream              bool              `json:"stream,omitempty"`
		Tools               json.RawMessage   `json:"tools,omitempty"`
		ToolChoice          json.RawMessage   `json:"tool_choice,omitempty"`
		ResponseFormat      json.RawMessage   `json:"response_format,omitempty"`
		Stop                []string          `json:"stop,omitempty"`
		ReasoningEffort     string            `json:"reasoning_effort,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse deepseek body: %w", err)
	}

	msgs := make([]unified.Message, 0, len(raw.Messages))
	systemParts := make([]string, 0)
	for _, rawMsg := range raw.Messages {
		// 先解析为 map 以保留 unknown fields（如 reasoning_content / prefix）
		var rawMap map[string]json.RawMessage
		if err := json.Unmarshal(rawMsg, &rawMap); err != nil {
			return nil, fmt.Errorf("parse message: %w", err)
		}
		var m unified.Message
		if err := json.Unmarshal(rawMsg, &m); err != nil {
			return nil, fmt.Errorf("parse message: %w", err)
		}
		// 提取 reasoning_content（DeepSeek/o1 思维链，多轮对话须原样传回）
		if rc, ok := rawMap["reasoning_content"]; ok {
			var s string
			if json.Unmarshal(rc, &s) == nil {
				m.ReasoningContent = s
			}
		}
		// 如果 assistant 消息的 content 中包含 <think>...</think> 标签，
		// 剥离标签内容到 ReasoningContent（兼容标准 OpenAI 客户端回传的格式）
		if m.Role == "assistant" && m.ReasoningContent == "" {
			contentStr := unified.ContentString(m.Content)
			if reasoning, cleanContent := stripThinkTag(contentStr); reasoning != "" {
				m.ReasoningContent = reasoning
				m.Content = unified.StringContent(cleanContent)
			}
		}
		// 提取 prefix（DeepSeek Chat Prefix Completion）
		if pf, ok := rawMap["prefix"]; ok {
			var b bool
			if json.Unmarshal(pf, &b) == nil {
				m.Prefix = b
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
		FrequencyPenalty: raw.FrequencyPenalty,
		PresencePenalty:  raw.PresencePenalty,
		Stream:           raw.Stream,
		ToolChoice:       raw.ToolChoice,
		ResponseFormat:   raw.ResponseFormat,
		Stop:             raw.Stop,
		ReasoningEffort:  raw.ReasoningEffort,
		SourceProtocol:   "deepseek",
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
// FromUnified — UnifiedRequest → DeepSeek 请求，发上游，返回统一响应
// =============================================================================

func (p *DeepSeekProvider) FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error) {
	deepseekReq := p.unifiedToDeepSeek(req)
	body, err := json.Marshal(deepseekReq)
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
		events := p.streamDeepSeekToUnified(resp.Body)
		return nil, events, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	uresp, err := p.parseDeepSeekResponse(respBody)
	return uresp, nil, err
}

// unifiedToDeepSeek 将 UnifiedRequest 转回 DeepSeek 请求体
// 关键：还原 reasoning_content（多轮对话必须原样传回，否则 DeepSeek 报 400）
func (p *DeepSeekProvider) unifiedToDeepSeek(req *unified.Request) map[string]interface{} {
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
		// 还原 reasoning_content（DeepSeek 思维链，多轮对话必须传回）
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
		result["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		result["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		result["top_p"] = *req.TopP
	}
	if req.FrequencyPenalty != nil {
		result["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		result["presence_penalty"] = *req.PresencePenalty
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
// FormatUnified — Unified 响应/流 → DeepSeek 客户端格式（OpenAI 兼容）
// =============================================================================

func (p *DeepSeekProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		// 非流式
		usage.InputTokens = resp.Usage.InputTokens
		usage.OutputTokens = resp.Usage.OutputTokens
		usage.CachedTokens = resp.Usage.CachedTokens
		usage.CacheHitTokens = resp.Usage.CacheHitTokens
		usage.CacheMissTokens = resp.Usage.CacheMissTokens

		usageMap := map[string]interface{}{
			"prompt_tokens":     resp.Usage.InputTokens,
			"completion_tokens": resp.Usage.OutputTokens,
			"total_tokens":      resp.Usage.TotalTokens(),
		}
		if resp.Usage.CacheHitTokens > 0 || resp.Usage.CacheMissTokens > 0 {
			usageMap["prompt_cache_hit_tokens"] = resp.Usage.CacheHitTokens
			usageMap["prompt_cache_miss_tokens"] = resp.Usage.CacheMissTokens
		}
		if resp.Usage.CachedTokens > 0 {
			usageMap["prompt_tokens_details"] = map[string]interface{}{
				"cached_tokens": resp.Usage.CachedTokens,
			}
		}

		deepseekResp := map[string]interface{}{
			"id":     resp.ID,
			"object": "chat.completion",
			"model":  resp.Model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       p.buildDeepSeekMessage(resp),
					"finish_reason": resp.FinishReason,
				},
			},
			"usage": usageMap,
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		body, _ := json.Marshal(deepseekResp)
		_, err := c.Writer.Write(body)
		return err
	}

	// 流式：将 reasoning_content 包裹为 <think>...</think>
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var id string
	var thinkStarted bool // 是否已输出 <think> 开头
	var thinkEnded bool   // 是否已输出 </think> 结尾
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

				// 支持原生 reasoning_content
				if ev.Delta.ReasoningContent != "" {
					deltaMap["reasoning_content"] = ev.Delta.ReasoningContent
				}

				// 处理 reasoning_content 混合 content：包裹为 <think>...</think>
				var contentText string
				if ev.Delta.ReasoningContent != "" {
					thinkText := ev.Delta.ReasoningContent
					if !thinkStarted {
						thinkText = "<think>" + thinkText
						thinkStarted = true
					}
					contentText = thinkText
				}
				if ev.Delta.Content != "" {
					if thinkStarted && !thinkEnded {
						contentText += "</think>\n" + ev.Delta.Content
						thinkEnded = true
					} else {
						contentText += ev.Delta.Content
					}
				}
				if contentText != "" {
					deltaMap["content"] = contentText
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
				usage.CacheHitTokens = ev.Usage.CacheHitTokens
				usage.CacheMissTokens = ev.Usage.CacheMissTokens
			}
		case unified.EventDone:
			if thinkStarted && !thinkEnded {
				// 兜底输出闭合标签
				chunk := map[string]interface{}{
					"id":     id,
					"object": "chat.completion.chunk",
					"choices": []map[string]interface{}{
						{
							"index": 0,
							"delta": map[string]interface{}{
								"content": "</think>\n",
							},
							"finish_reason": nil,
						},
					},
				}
				data, _ := json.Marshal(chunk)
				fmt.Fprintf(c.Writer, "data: %s\n\n", data)
				c.Writer.Flush()
				thinkEnded = true
			}

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

func (p *DeepSeekProvider) buildDeepSeekMessage(resp *unified.Response) map[string]interface{} {
	msg := map[string]interface{}{"role": "assistant"}
	content := resp.Content
	// 将 reasoning_content 包裹为 <think>...</think> 前置到 content（兼容标准 OpenAI 客户端）
	if resp.ReasoningContent != "" {
		content = wrapThinkTag(resp.ReasoningContent, content)
	}
	if content != "" {
		msg["content"] = content
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
// 内部：解析 DeepSeek 响应 → UnifiedResponse
// =============================================================================

type deepseekUsageRaw struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens"`  // DeepSeek 专用：前缀缓存命中
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens"` // DeepSeek 专用：前缀缓存未命中
	PromptTokensDetails   struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (p *DeepSeekProvider) parseDeepSeekResponse(body []byte) (*unified.Response, error) {
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
		Usage deepseekUsageRaw `json:"usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	uresp := &unified.Response{
		ID:    raw.ID,
		Model: raw.Model,
		Usage: unified.Usage{
			CachedTokens:    raw.Usage.PromptTokensDetails.CachedTokens,
			CacheHitTokens:  raw.Usage.PromptCacheHitTokens,
			CacheMissTokens: raw.Usage.PromptCacheMissTokens,
			InputTokens:     raw.Usage.PromptTokens - raw.Usage.PromptTokensDetails.CachedTokens,
			OutputTokens:    raw.Usage.CompletionTokens,
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
// 流式：DeepSeek SSE → unified.StreamEvent chan
// =============================================================================

func (p *DeepSeekProvider) streamDeepSeekToUnified(body io.ReadCloser) <-chan unified.StreamEvent {
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
				Usage *deepseekUsageRaw `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if chunk.Usage != nil {
				ch <- unified.StreamEvent{
					Type: unified.EventUsage,
					Usage: &unified.Usage{
						CachedTokens:    chunk.Usage.PromptTokensDetails.CachedTokens,
						CacheHitTokens:  chunk.Usage.PromptCacheHitTokens,
						CacheMissTokens: chunk.Usage.PromptCacheMissTokens,
						InputTokens:     chunk.Usage.PromptTokens - chunk.Usage.PromptTokensDetails.CachedTokens,
						OutputTokens:    chunk.Usage.CompletionTokens,
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

// deepseekModelDefaults provides known specs for DeepSeek models when the API doesn't return them.
var deepseekModelDefaults = map[string]struct {
	contextWindow int
	maxOutput     int
	vision        bool
}{
	"deepseek-chat":     {131072, 8192, false},
	"deepseek-reasoner": {131072, 8192, false},
	"deepseek-v3":       {131072, 8192, false},
	"deepseek-v4-flash": {131072, 16384, true},
	"deepseek-v4-pro":   {262144, 32768, true},
}

// detectDeepSeekVision checks if a DeepSeek model supports vision based on its ID.
func detectDeepSeekVision(modelID string) bool {
	if def, ok := deepseekModelDefaults[modelID]; ok {
		return def.vision
	}
	return false
}
