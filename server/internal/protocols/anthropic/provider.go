package anthropic

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

type AnthropicProvider struct {
	cfg *registry.Config
}

func NewAnthropicProvider(cfg *registry.Config) *AnthropicProvider {
	return &AnthropicProvider{cfg: cfg}
}

// =============================================================================
// SyncModels
// =============================================================================

type anthropicModelEntry struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

func (p *AnthropicProvider) SyncModels(providerID uint) ([]registry.ProviderModel, error) {
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
		return nil, fmt.Errorf("Anthropic API error: %s", string(body))
	}

	var result struct {
		Data []anthropicModelEntry `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]registry.ProviderModel, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID == "" {
			continue
		}
		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.ID
		}
		models = append(models, registry.ProviderModel{
			ProviderID:     providerID,
			ModelID:        m.ID,
			DisplayName:    displayName,
			OwnedBy:        "anthropic",
			SupportsVision: false,
			SupportsTools:  true,
			SupportsStream: true,
			IsAvailable:    true,
			Source:         "sync",
		})
	}
	return models, nil
}

// =============================================================================
// HandleNative — Anthropic 直通（只替换模型名）
// =============================================================================

func (p *AnthropicProvider) HandleNative(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return fmt.Errorf("parse body: %w", err)
	}
	bodyMap["model"] = modelID
	body, _ = json.Marshal(bodyMap)

	req, err := http.NewRequestWithContext(ctx.Request.Context(), "POST",
		p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
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
		return fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if p.isStreaming(resp) {
		ctx.Status(http.StatusOK)
		ctx.Header("Content-Type", "text/event-stream")
		ctx.Header("Cache-Control", "no-cache")
		ctx.Header("Connection", "keep-alive")
		return p.copyAnthropicStream(ctx.Request.Context(), ctx.Writer, resp.Body, usage)
	}

	ctx.Status(http.StatusOK)
	ctx.Header("Content-Type", "application/json")
	return p.copyAnthropicResponse(ctx.Writer, resp.Body, usage)
}

// =============================================================================
// FromOpenAI — OpenAI 请求 → Anthropic 格式，发上游，响应转回 OpenAI
// =============================================================================

type openaiChatRequest struct {
	Model     string                   `json:"model"`
	Messages  []map[string]interface{} `json:"messages"`
	MaxTokens int                      `json:"max_tokens"`
	Tools     json.RawMessage          `json:"tools,omitempty"`
	Stream    bool                     `json:"stream"`
	StreamOptions *struct{}            `json:"stream_options,omitempty"`
}

type anthropicRequest struct {
	Model     string                   `json:"model"`
	MaxTokens int                      `json:"max_tokens"`
	System    interface{}              `json:"system,omitempty"`
	Messages  []map[string]interface{} `json:"messages"`
	Stream    bool                     `json:"stream,omitempty"`
}

func (p *AnthropicProvider) FromOpenAI(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	var openAIReq openaiChatRequest
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		return fmt.Errorf("parse body: %w", err)
	}

	// 转换：OpenAI messages → Anthropic messages
	anthropicReq := p.convertOpenAIToAnthropic(openAIReq, modelID)

	anthropicBody, _ := json.Marshal(anthropicReq)

	req, err := http.NewRequestWithContext(ctx.Request.Context(), "POST",
		p.cfg.BaseURL+"/v1/messages", bytes.NewReader(anthropicBody))
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
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
		return fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if openAIReq.Stream {
		ctx.Status(http.StatusOK)
		ctx.Header("Content-Type", "text/event-stream")
		ctx.Header("Cache-Control", "no-cache")
		ctx.Header("Connection", "keep-alive")
		return p.streamAnthropicToOpenAI(ctx.Request.Context(), resp.Body, ctx.Writer, modelID, usage)
	}

	body, _ = io.ReadAll(resp.Body)
	openAIResp, err := p.convertAnthropicResponseToOpenAI(body, modelID, usage)
	if err != nil {
		return err
	}
	ctx.Status(http.StatusOK)
	ctx.Header("Content-Type", "application/json")
	ctx.Writer.Write(openAIResp)
	return nil
}

// =============================================================================
// 转换逻辑：OpenAI → Anthropic
// =============================================================================

func (p *AnthropicProvider) convertOpenAIToAnthropic(req openaiChatRequest, modelID string) anthropicRequest {
	anthropicMsgs := make([]map[string]interface{}, 0)
	var systemContent interface{}

	for _, msg := range req.Messages {
		role, _ := msg["role"].(string)
		content := msg["content"]

		if role == "system" {
			systemContent = content
			continue
		}

		// 转换 assistant 消息中的 tool_calls → Anthropic 格式
		anthropicRole := role

		newMsg := map[string]interface{}{"role": anthropicRole}

		switch v := content.(type) {
		case string:
			newMsg["content"] = v
		case []interface{}:
			// 已经是 content blocks 格式
			newMsg["content"] = v
		default:
			newMsg["content"] = fmt.Sprintf("%v", v)
		}

		// 处理 tool_calls
		if role == "assistant" {
			if toolCalls, ok := msg["tool_calls"]; ok {
				blocks := make([]map[string]interface{}, 0)
				if existingBlocks, ok := content.([]interface{}); ok {
					for _, b := range existingBlocks {
						if block, ok := b.(map[string]interface{}); ok {
							blocks = append(blocks, block)
						}
					}
				}
				if tcArr, ok := toolCalls.([]interface{}); ok {
					for _, tc := range tcArr {
						if tcMap, ok := tc.(map[string]interface{}); ok {
							tcType := "function"
							if t, _ := tcMap["type"].(string); t != "" {
								tcType = t
							}
							block := map[string]interface{}{"type": "tool_use"}
							if fn, ok := tcMap["function"]; ok {
								block["name"] = fn.(map[string]interface{})["name"]
								if args, ok := fn.(map[string]interface{})["arguments"]; ok {
									var parsed interface{}
									if err := json.Unmarshal([]byte(args.(string)), &parsed); err == nil {
										block["input"] = parsed
									} else {
										block["input"] = args
									}
								}
							}
							if id, ok := tcMap["id"]; ok {
								block["id"] = id
							}
							// 类型字段
							if tcType == "function" {
								block["type"] = "tool_use"
							} else {
								block["type"] = tcType
							}
							blocks = append(blocks, block)
						}
					}
				}
				newMsg["content"] = blocks
			}
		}

		// 处理 tool 角色
		if role == "tool" {
			if toolCallID, ok := msg["tool_call_id"]; ok {
				newMsg["content"] = []map[string]interface{}{
					{
						"type":      "tool_result",
						"tool_use_id": toolCallID,
						"content":   content,
					},
				}
			}
		}

		anthropicMsgs = append(anthropicMsgs, newMsg)
	}

	result := anthropicRequest{
		Model:     modelID,
		MaxTokens: req.MaxTokens,
		System:    systemContent,
		Messages:  anthropicMsgs,
		Stream:    req.Stream,
	}
	if result.MaxTokens == 0 {
		result.MaxTokens = 4096
	}
	return result
}

// =============================================================================
// 响应转换：Anthropic → OpenAI（非流式）
// =============================================================================

func (p *AnthropicProvider) convertAnthropicResponseToOpenAI(body []byte, modelID string, usage *registry.Usage) ([]byte, error) {
	var anthroResp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &anthroResp); err != nil {
		return nil, err
	}

	usage.InputTokens = anthroResp.Usage.InputTokens
	usage.OutputTokens = anthroResp.Usage.OutputTokens

	combined := ""
	for _, c := range anthroResp.Content {
		if c.Type == "text" {
			combined += c.Text
		}
	}

	finishReason := "stop"
	if anthroResp.StopReason == "max_tokens" {
		finishReason = "length"
	} else if anthroResp.StopReason == "tool_use" {
		finishReason = "tool_calls"
	}

	resp := map[string]interface{}{
		"id":      anthroResp.ID,
		"object":  "chat.completion",
		"model":   modelID,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]interface{}{"role": "assistant", "content": combined},
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":      anthroResp.Usage.InputTokens,
			"completion_tokens":  anthroResp.Usage.OutputTokens,
			"total_tokens":       anthroResp.Usage.InputTokens + anthroResp.Usage.OutputTokens,
		},
	}
	return json.Marshal(resp)
}

// =============================================================================
// SSE 流式处理
// =============================================================================

type anthropicUsageRaw struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (u anthropicUsageRaw) toUsage(usage *registry.Usage) {
	usage.InputTokens = u.InputTokens
	usage.OutputTokens = u.OutputTokens
}

func (p *AnthropicProvider) isStreaming(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return len(resp.Header["Transfer-Encoding"]) > 0 ||
		(len(contentType) >= 17 && contentType[:17] == "text/event-stream")
}

func (p *AnthropicProvider) copyAnthropicStream(ctx context.Context, dst io.Writer, src io.Reader, usage *registry.Usage) error {
	reader := bufio.NewReader(src)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if _, err := fmt.Fprint(dst, line); err != nil {
			return err
		}
		if flusher, ok := dst.(http.Flusher); ok {
			flusher.Flush()
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimSpace(data)

		var event struct {
			Type  string             `json:"type"`
			Usage anthropicUsageRaw  `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &event); err == nil {
			if event.Type == "message_delta" {
				event.Usage.toUsage(usage)
			}
		}
	}
	return nil
}

func (p *AnthropicProvider) copyAnthropicResponse(dst io.Writer, src io.Reader, usage *registry.Usage) error {
	body, err := io.ReadAll(src)
	if err != nil {
		return err
	}
	dst.Write(body)

	var resp struct {
		Usage anthropicUsageRaw `json:"usage"`
	}
	json.Unmarshal(body, &resp)
	resp.Usage.toUsage(usage)
	return nil
}

// =============================================================================
// 流式响应转换：Anthropic SSE → OpenAI SSE
// =============================================================================

func (p *AnthropicProvider) streamAnthropicToOpenAI(ctx context.Context, src io.Reader, dst io.Writer, modelID string, usage *registry.Usage) error {
	reader := bufio.NewReader(src)
	var textBuilder strings.Builder
	var deltaID string

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

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		eventType, _ := event["type"].(string)

		switch eventType {
		case "message_start":
			if msg, ok := event["message"].(map[string]interface{}); ok {
				if id, ok := msg["id"].(string); ok {
					deltaID = id
				}
			}

		case "content_block_delta":
			if delta, ok := event["delta"].(map[string]interface{}); ok {
				if deltaType, _ := delta["type"].(string); deltaType == "text_delta" {
					if text, ok := delta["text"].(string); ok {
						textBuilder.WriteString(text)
						chunk := map[string]interface{}{
							"id":      deltaID,
							"object":  "chat.completion.chunk",
							"model":   modelID,
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

		case "message_delta":
			if usageRaw, ok := event["usage"].(map[string]interface{}); ok {
				if outputTokens, ok := usageRaw["output_tokens"].(float64); ok {
					usage.OutputTokens = int(outputTokens)
				}
				if inputTokens, ok := usageRaw["input_tokens"].(float64); ok {
					usage.InputTokens = int(inputTokens)
				}
			}

		case "message_stop":
			fmt.Fprintf(dst, "data: [DONE]\n\n")
			if flusher, ok := dst.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
	return nil
}
