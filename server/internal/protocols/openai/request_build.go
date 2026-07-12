package openai

import (
	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/unified"
	"ai-gateway/internal/core/unified/thinking"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// FromUnified — UnifiedRequest → OpenAI 请求，发上游，返回统一响应
// =============================================================================

func (p *OpenAIProvider) FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error) {
	openAIReq := p.unifiedToOpenAI(req)
	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, nil, err
	}

	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpPool.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		p.lastRawResponse = respBody
		return nil, nil, &registry.HTTPError{StatusCode: resp.StatusCode, Body: respBody}
	}

	if req.Stream {
		events := p.streamOpenAIToUnified(ctx, resp.Body)
		return nil, events, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	p.lastRawResponse = respBody
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
		result["tool_choice"] = normalizeToolChoiceForOpenAI(req.ToolChoice)
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
	// 思考管道 — ThkConfig 覆盖优先
	if cfg := req.ThkConfig; cfg != nil {
		switch cfg.Mode {
		case thinking.ModeBudget:
			result["reasoning_budget"] = cfg.Budget
		case thinking.ModeLevel:
			result["reasoning_effort"] = cfg.Level
		case thinking.ModeAuto:
			// auto → 不发送 effort，让推理模型自行决定
		case thinking.ModeNone:
			// 明确禁用（某些模型可能支持 reasoning_effort: "none"）
			result["reasoning_effort"] = "none"
		}
	}
	if req.Stream {
		result["stream_options"] = map[string]bool{"include_usage": true}
	}
	return result
}

// normalizeToolChoiceForOpenAI 将 Anthropic 风格的 tool_choice 转为 OpenAI 格式。
// Anthropic: {"type":"auto"} / {"type":"any"} / {"type":"tool","name":"xxx"}
// OpenAI:    "auto" / "none" / "required" / {"type":"function","function":{"name":"xxx"}}
// 直接透传已兼容 OpenAI 格式的值。
func normalizeToolChoiceForOpenAI(toolChoice json.RawMessage) interface{} {
	// 尝试解析为字符串（已是 OpenAI 兼容格式）
	var strVal string
	if json.Unmarshal(toolChoice, &strVal) == nil {
		return strVal
	}
	// 尝试解析为对象
	var obj map[string]interface{}
	if json.Unmarshal(toolChoice, &obj) != nil {
		return json.RawMessage(toolChoice) // 无法解析，透传
	}
	// Anthropic 格式转换
	if tcType, ok := obj["type"].(string); ok {
		switch tcType {
		case "auto":
			return "auto"
		case "any":
			return "required"
		case "tool":
			if name, ok := obj["name"].(string); ok && name != "" {
				return map[string]interface{}{
					"type": "function",
					"function": map[string]interface{}{
						"name": name,
					},
				}
			}
		}
	}
	// 无法识别，透传（可能是 OpenAI 自己的对象格式）
	return json.RawMessage(toolChoice)
}
