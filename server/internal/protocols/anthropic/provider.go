package anthropic

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
)

type AnthropicProvider struct {
	cfg *registry.Config
}

func NewAnthropicProvider(cfg *registry.Config) *AnthropicProvider {
	return &AnthropicProvider{cfg: cfg}
}

// =============================================================================
// SyncModels
// =============================================================================

func (p *AnthropicProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
	// 尝试从 Anthropic Models API 获取模型规格，失败则回退硬编码
	apiSpecs, err := p.fetchModelSpecs()
	if err != nil {
		log.Printf("[Anthropic] Models API unavailable (%v), using hardcoded specs as fallback", err)
		return p.knownModels(providerID), nil
	}
	log.Printf("[Anthropic] Models API returned %d models, merging with hardcoded IDs", len(apiSpecs))
	return p.buildModels(providerID, apiSpecs), nil
}

// anthropicModelSpec represents a model's specs from the Anthropic Models API.
type anthropicModelSpec struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	MaxInput     int    `json:"max_input_tokens"`
	MaxOutput    int    `json:"max_tokens"`
	Capabilities struct {
		Vision struct {
			Supported bool `json:"supported"`
		} `json:"image_input"`
	} `json:"capabilities"`
}

func (p *AnthropicProvider) fetchModelSpecs() (map[string]*anthropicModelSpec, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", p.cfg.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Anthropic Models API error: %s", string(body))
	}

	var result struct {
		Data []anthropicModelSpec `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	specs := make(map[string]*anthropicModelSpec, len(result.Data))
	for i := range result.Data {
		specs[result.Data[i].ID] = &result.Data[i]
	}
	return specs, nil
}

// knownModelIDs lists all Claude model IDs (正版无日期格式).
var knownModelIDs = []struct {
	id      string
	display string
}{
	{"claude-opus-4-8", "Claude Opus 4.8"},
	{"claude-sonnet-4-6", "Claude Sonnet 4.6"},
	{"claude-haiku-4-5", "Claude Haiku 4.5"},
	{"claude-opus-4-7", "Claude Opus 4.7"},
	{"claude-opus-4-6", "Claude Opus 4.6"},
	{"claude-sonnet-4-5", "Claude Sonnet 4.5"},
	{"claude-opus-4-5", "Claude Opus 4.5"},
}

// hardcodedFallbacks: only for the 3 old 4.5-gen models that the API may not return.
var hardcodedFallbacks = map[string]struct {
	ctx int
	out int
	vis bool
}{
	"claude-haiku-4-5":  {200000, 64000, true},
	"claude-sonnet-4-5": {200000, 64000, true},
	"claude-opus-4-5":   {200000, 64000, true},
}

// buildModels merges known IDs with API specs. Newer models (4.6+) come from API only;
// the 3 old 4.5-gen models fall back to official hardcoded values if not in API.
func (p *AnthropicProvider) buildModels(providerID uint, specs map[string]*anthropicModelSpec) []registry.ProviderModel {
	result := make([]registry.ProviderModel, 0, len(knownModelIDs))
	for _, m := range knownModelIDs {
		var ctx, out int
		var vis bool
		name := m.display

		if s, ok := specs[m.id]; ok {
			ctx, out, vis = s.MaxInput, s.MaxOutput, s.Capabilities.Vision.Supported
			if s.DisplayName != "" {
				name = s.DisplayName
			}
		} else if fb, ok := hardcodedFallbacks[m.id]; ok {
			ctx, out, vis = fb.ctx, fb.out, fb.vis
		} else {
			continue
		}

		result = append(result, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.id,
			DisplayName:    name,
			OwnedBy:        "anthropic",
			ContextWindow:  ctx,
			MaxOutput:      out,
			SupportsVision: vis,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return result
}

// knownModels is the offline fallback when the API is unreachable.
// It includes ALL models with their best-known specs from official docs.
func (p *AnthropicProvider) knownModels(providerID uint) []registry.ProviderModel {
	models := []struct {
		id   string
		name string
		ctx  int
		out  int
		vis  bool
	}{
		{"claude-opus-4-8", "Claude Opus 4.8", 1000000, 128000, true},
		{"claude-sonnet-4-6", "Claude Sonnet 4.6", 1000000, 128000, true},
		{"claude-haiku-4-5", "Claude Haiku 4.5", 200000, 64000, true},
		{"claude-opus-4-7", "Claude Opus 4.7", 1000000, 128000, true},
		{"claude-opus-4-6", "Claude Opus 4.6", 1000000, 128000, true},
		{"claude-sonnet-4-5", "Claude Sonnet 4.5", 200000, 64000, true},
		{"claude-opus-4-5", "Claude Opus 4.5", 200000, 64000, true},
	}
	result := make([]registry.ProviderModel, 0, len(models))
	for _, m := range models {
		result = append(result, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.id,
			DisplayName:    m.name,
			OwnedBy:        "anthropic",
			ContextWindow:  m.ctx,
			MaxOutput:      m.out,
			SupportsVision: m.vis,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return result
}

// =============================================================================
// ToUnified — Anthropic 请求 → UnifiedRequest
// =============================================================================

func (p *AnthropicProvider) ToUnified(body []byte, modelID string) (*unified.Request, error) {
	var raw struct {
		Model       string            `json:"model"`
		MaxTokens   int               `json:"max_tokens"`
		System      json.RawMessage   `json:"system,omitempty"`
		Messages    []json.RawMessage `json:"messages"`
		Tools       json.RawMessage   `json:"tools,omitempty"`
		Stream      bool              `json:"stream,omitempty"`
		Temperature *float64          `json:"temperature,omitempty"`
		TopP        *float64          `json:"top_p,omitempty"`
		Stop        []string          `json:"stop_sequences,omitempty"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse anthropic body: %w", err)
	}

	// 解析 system（可能是 string 或 content blocks）
	systemPrompt := ""
	if len(raw.System) > 0 {
		var s string
		if json.Unmarshal(raw.System, &s) == nil {
			systemPrompt = s
		} else {
			var blocks []unified.ContentBlock
			if json.Unmarshal(raw.System, &blocks) == nil {
				for _, b := range blocks {
					if b.Type == "text" {
						systemPrompt += b.Text
					}
				}
			}
		}
	}

	// 转换 messages
	msgs := make([]unified.Message, 0, len(raw.Messages))
	for _, rawMsg := range raw.Messages {
		var m struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(rawMsg, &m); err != nil {
			return nil, fmt.Errorf("parse anthropic message: %w", err)
		}
		um := unified.Message{Role: m.Role}

		// Anthropic content 可能是 string 或 content blocks
		// 统一转为 OpenAI 风格：assistant 的 tool_use → tool_calls，user 的 tool_result → tool role
		var s string
		if json.Unmarshal(m.Content, &s) == nil {
			um.Content = unified.StringContent(s)
			msgs = append(msgs, um)
			continue
		}

		var blocks []unified.ContentBlock
		if json.Unmarshal(m.Content, &blocks) != nil {
			um.Content = m.Content
			msgs = append(msgs, um)
			continue
		}

		// 分离 tool_use / tool_result / text / image
		textParts := make([]string, 0)
		var toolCalls []unified.ToolCall
		var toolResults []unified.Message
		for _, b := range blocks {
			switch b.Type {
			case "text":
				textParts = append(textParts, b.Text)
			case "image":
				// 保留为 image block
			case "tool_use":
				args, _ := json.Marshal(b.Input)
				toolCalls = append(toolCalls, unified.ToolCall{
					ID:   b.ID,
					Type: "function",
					Function: unified.FunctionCall{
						Name:      b.Name,
						Arguments: string(args),
					},
				})
			case "tool_result":
				// tool_result 转为独立的 tool role 消息
				toolResults = append(toolResults, unified.Message{
					Role:       "tool",
					ToolCallID: b.ToolUseID,
					Content:    b.Content,
				})
			}
		}

		if len(textParts) > 0 {
			um.Content = unified.StringContent(strings.Join(textParts, "\n"))
		}
		if len(toolCalls) > 0 {
			um.ToolCalls = toolCalls
		}
		msgs = append(msgs, um)
		msgs = append(msgs, toolResults...)
	}

	req := &unified.Request{
		Model:          modelID,
		Messages:       msgs,
		SystemPrompt:   systemPrompt,
		MaxTokens:      raw.MaxTokens,
		Temperature:    raw.Temperature,
		TopP:           raw.TopP,
		Stream:         raw.Stream,
		Stop:           raw.Stop,
		SourceProtocol: "anthropic",
	}
	if len(raw.Tools) > 0 {
		req.Tools = raw.Tools
	}
	return req, nil
}

// =============================================================================
// FromUnified — UnifiedRequest → Anthropic 请求，发上游，返回统一响应
// =============================================================================

func (p *AnthropicProvider) FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error) {
	anthropicReq := p.unifiedToAnthropic(req)
	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, nil, err
	}

	httpReq, err := http.NewRequest("POST", p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("x-api-key", p.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
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
		events := p.streamAnthropicToUnified(resp.Body)
		return nil, events, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	uresp, err := p.parseAnthropicResponse(respBody)
	return uresp, nil, err
}

// unifiedToAnthropic 将 UnifiedRequest 转为 Anthropic 请求体
func (p *AnthropicProvider) unifiedToAnthropic(req *unified.Request) map[string]interface{} {
	anthropicMsgs := make([]map[string]interface{}, 0, len(req.Messages))

	for _, m := range req.Messages {
		if m.Role == "system" {
			continue // system 走顶层字段
		}

		if m.Role == "tool" {
			// OpenAI tool role → Anthropic user message with tool_result block
			anthropicMsgs = append(anthropicMsgs, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":        "tool_result",
						"tool_use_id": m.ToolCallID,
						"content":     unified.ContentString(m.Content),
					},
				},
			})
			continue
		}

		// user / assistant
		blocks := p.unifiedContentToAnthropicBlocks(m)
		msg := map[string]interface{}{
			"role":    m.Role,
			"content": blocks,
		}
		// assistant 的 tool_calls → tool_use blocks
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var input interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
					input = tc.Function.Arguments
				}
				blocks = append(blocks, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			msg["content"] = blocks
		}
		anthropicMsgs = append(anthropicMsgs, msg)
	}

	result := map[string]interface{}{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   anthropicMsgs,
		"stream":     req.Stream,
	}
	if req.MaxTokens == 0 {
		result["max_tokens"] = 4096
	}
	if req.SystemPrompt != "" {
		result["system"] = req.SystemPrompt
	}
	if req.Temperature != nil {
		result["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		result["top_p"] = *req.TopP
	}
	if len(req.Stop) > 0 {
		result["stop_sequences"] = req.Stop
	}
	if len(req.Tools) > 0 {
		var unifiedTools []unified.Tool
		if err := json.Unmarshal(req.Tools, &unifiedTools); err == nil {
			tools := make([]map[string]interface{}, 0, len(unifiedTools))
			for _, t := range unifiedTools {
				if t.Function.Name != "" {
					tools = append(tools, map[string]interface{}{
						"name":         t.Function.Name,
						"description":  t.Function.Description,
						"input_schema": t.Function.Parameters,
					})
				}
			}
			if len(tools) > 0 {
				result["tools"] = tools
			}
		}
	}
	return result
}

// unifiedContentToAnthropicBlocks 将 Unified Message content 转为 Anthropic content blocks
func (p *AnthropicProvider) unifiedContentToAnthropicBlocks(m unified.Message) []map[string]interface{} {
	blocks := make([]map[string]interface{}, 0)

	// string content
	if s := unified.ContentString(m.Content); s != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "text",
			"text": s,
		})
		return blocks
	}

	// content blocks
	for _, b := range unified.ContentBlocks(m.Content) {
		switch b.Type {
		case "text":
			blocks = append(blocks, map[string]interface{}{
				"type": "text",
				"text": b.Text,
			})
		case "image_url":
			if b.ImageURL != nil {
				url := b.ImageURL.URL
				if strings.HasPrefix(url, "data:") {
					mediaType, data := parseDataURL(url)
					blocks = append(blocks, map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type":       "base64",
							"media_type": mediaType,
							"data":       data,
						},
					})
				} else {
					blocks = append(blocks, map[string]interface{}{
						"type": "image",
						"source": map[string]interface{}{
							"type": "url",
							"url":  url,
						},
					})
				}
			}
		}
	}

	if len(blocks) == 0 {
		blocks = append(blocks, map[string]interface{}{"type": "text", "text": ""})
	}
	return blocks
}

func parseDataURL(url string) (mediaType, data string) {
	// data:image/jpeg;base64,xxxx
	if idx := strings.Index(url, ";"); idx > 0 {
		mediaType = strings.TrimPrefix(url[:idx], "data:")
		if comma := strings.Index(url, ","); comma > 0 {
			data = url[comma+1:]
		}
	}
	return
}

// =============================================================================
// 解析 Anthropic 响应 → UnifiedResponse
// =============================================================================

func (p *AnthropicProvider) parseAnthropicResponse(body []byte) (*unified.Response, error) {
	var raw struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	uresp := &unified.Response{
		ID:    raw.ID,
		Model: raw.Model,
		Usage: unified.Usage{
			InputTokens:  raw.Usage.InputTokens,
			OutputTokens: raw.Usage.OutputTokens,
		},
	}

	var textContent string
	var reasoningContent string
	var toolCalls []unified.ToolCall
	for _, c := range raw.Content {
		switch c.Type {
		case "text":
			textContent += c.Text
		case "thinking":
			if c.Text != "" {
				reasoningContent += c.Text
			} else if c.Name == "thinking" && len(c.Input) > 0 {
				reasoningContent += string(c.Input)
			}
		case "tool_use":
			args, _ := json.Marshal(c.Input)
			toolCalls = append(toolCalls, unified.ToolCall{
				ID:   c.ID,
				Type: "function",
				Function: unified.FunctionCall{
					Name:      c.Name,
					Arguments: string(args),
				},
			})
		}
	}
	uresp.Content = textContent
	uresp.ReasoningContent = reasoningContent
	uresp.ToolCalls = toolCalls

	switch raw.StopReason {
	case "max_tokens":
		uresp.FinishReason = "length"
	case "tool_use":
		uresp.FinishReason = "tool_calls"
	default:
		uresp.FinishReason = "stop"
	}
	return uresp, nil
}

// =============================================================================
// 流式：Anthropic SSE → unified.StreamEvent chan
// =============================================================================

func (p *AnthropicProvider) streamAnthropicToUnified(body io.ReadCloser) <-chan unified.StreamEvent {
	ch := make(chan unified.StreamEvent, 32)
	go func() {
		defer body.Close()
		defer close(ch)
		reader := bufio.NewReader(body)
		var inputTokens int
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
			var event struct {
				Type    string          `json:"type"`
				Message json.RawMessage `json:"message"`
				Delta   json.RawMessage `json:"delta"`
				Usage   json.RawMessage `json:"usage"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "message_start":
				if len(event.Message) > 0 {
					var msg struct {
						Usage struct {
							InputTokens int `json:"input_tokens"`
						} `json:"usage"`
					}
					if json.Unmarshal(event.Message, &msg) == nil {
						inputTokens = msg.Usage.InputTokens
					}
				}
			case "content_block_delta":
				if len(event.Delta) > 0 {
					var delta struct {
						Type        string `json:"type"`
						Text        string `json:"text"`
						Thinking    string `json:"thinking"`
						PartialJSON string `json:"partial_json"`
					}
					if json.Unmarshal(event.Delta, &delta) == nil {
						switch delta.Type {
						case "text_delta":
							ch <- unified.StreamEvent{
								Type: unified.EventChunk,
								Delta: &unified.Delta{
									Content: delta.Text,
								},
							}
						case "thinking_delta":
							ch <- unified.StreamEvent{
								Type: unified.EventChunk,
								Delta: &unified.Delta{
									ReasoningContent: delta.Thinking,
								},
							}
						case "input_json_delta":
							ch <- unified.StreamEvent{
								Type: unified.EventChunk,
								Delta: &unified.Delta{
									InputJSON: delta.PartialJSON,
								},
							}
						}
					}
				}
			case "message_delta":
				if len(event.Usage) > 0 {
					var u struct {
						OutputTokens int `json:"output_tokens"`
					}
					if json.Unmarshal(event.Usage, &u) == nil {
						ch <- unified.StreamEvent{
							Type: unified.EventUsage,
							Usage: &unified.Usage{
								InputTokens:  inputTokens,
								OutputTokens: u.OutputTokens,
							},
						}
					}
				}
			case "message_stop":
				ch <- unified.StreamEvent{
					Type:         unified.EventDone,
					FinishReason: "stop",
				}
				return
			}
		}
	}()
	return ch
}

// =============================================================================
// FormatUnified — Unified 响应/流 → Anthropic 客户端格式
// =============================================================================

func (p *AnthropicProvider) FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, c *gin.Context, usage *registry.Usage) error {
	if resp != nil {
		// 非流式
		usage.InputTokens = resp.Usage.InputTokens
		usage.OutputTokens = resp.Usage.OutputTokens

		contentBlocks := make([]map[string]interface{}, 0)
		if resp.Content != "" {
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type": "text",
				"text": resp.Content,
			})
		}
		for _, tc := range resp.ToolCalls {
			var input interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				input = tc.Function.Arguments
			}
			contentBlocks = append(contentBlocks, map[string]interface{}{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": input,
			})
		}

		stopReason := "end_turn"
		switch resp.FinishReason {
		case "length":
			stopReason = "max_tokens"
		case "tool_calls":
			stopReason = "tool_use"
		}

		anthropicResp := map[string]interface{}{
			"id":          resp.ID,
			"type":        "message",
			"role":        "assistant",
			"model":       resp.Model,
			"content":     contentBlocks,
			"stop_reason": stopReason,
			"usage": map[string]interface{}{
				"input_tokens":  resp.Usage.InputTokens,
				"output_tokens": resp.Usage.OutputTokens,
			},
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		body, _ := json.Marshal(anthropicResp)
		_, err := c.Writer.Write(body)
		return err
	}

	// 流式：Unified events → Anthropic SSE
	c.Status(http.StatusOK)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	var inputTokens, outputTokens int
	var blockIndex int
	var blockActive bool
	var currentBlockType string // "text" or "thinking"

	ensureBlockStart := func(blockType string) {
		if blockActive && currentBlockType == blockType {
			return
		}
		if blockActive {
			// 关闭当前 block
			p.writeSSE(c, map[string]interface{}{
				"type":  "content_block_stop",
				"index": blockIndex,
			})
			blockIndex++
		}
		p.writeSSE(c, map[string]interface{}{
			"type":          "content_block_start",
			"index":         blockIndex,
			"content_block": map[string]interface{}{"type": blockType, blockType: ""},
		})
		blockActive = true
		currentBlockType = blockType
	}

	for ev := range events {
		switch ev.Type {
		case unified.EventChunk:
			if ev.Delta == nil {
				continue
			}
			if ev.Delta.ReasoningContent != "" {
				ensureBlockStart("thinking")
				p.writeSSE(c, map[string]interface{}{
					"type":  "content_block_delta",
					"index": blockIndex,
					"delta": map[string]interface{}{
						"type":     "thinking_delta",
						"thinking": ev.Delta.ReasoningContent,
					},
				})
			}
			if ev.Delta.Content != "" {
				ensureBlockStart("text")
				p.writeSSE(c, map[string]interface{}{
					"type":  "content_block_delta",
					"index": blockIndex,
					"delta": map[string]interface{}{
						"type": "text_delta",
						"text": ev.Delta.Content,
					},
				})
			}
		case unified.EventUsage:
			if ev.Usage != nil {
				inputTokens = ev.Usage.InputTokens
				outputTokens = ev.Usage.OutputTokens
			}
		case unified.EventDone:
			// 关闭当前 block
			if blockActive {
				p.writeSSE(c, map[string]interface{}{
					"type":  "content_block_stop",
					"index": blockIndex,
				})
				blockIndex++
				blockActive = false
			}
			// message_delta with stop_reason + usage
			stopReason := "end_turn"
			switch ev.FinishReason {
			case "length":
				stopReason = "max_tokens"
			case "tool_calls":
				stopReason = "tool_use"
			case "stop_sequence":
				stopReason = "stop_sequence"
			}
			p.writeSSE(c, map[string]interface{}{
				"type":  "message_delta",
				"delta": map[string]interface{}{"stop_reason": stopReason},
				"usage": map[string]interface{}{
					"input_tokens":  inputTokens,
					"output_tokens": outputTokens,
				},
			})
			// message_stop
			p.writeSSE(c, map[string]interface{}{"type": "message_stop"})
		}
	}
	usage.InputTokens = inputTokens
	usage.OutputTokens = outputTokens
	return nil
}

func (p *AnthropicProvider) writeSSE(c *gin.Context, event map[string]interface{}) {
	data, _ := json.Marshal(event)
	fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event["type"], data)
	c.Writer.Flush()
}
