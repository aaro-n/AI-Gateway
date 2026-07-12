package openrouter

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

	resp, err := p.httpPool.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, nil, &registry.HTTPError{StatusCode: resp.StatusCode, Body: respBody}
	}

	if req.Stream {
		ctx := req.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
		events := p.streamOpenRouterToUnified(ctx, resp.Body)
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
	// 思考管道 — ThkConfig 覆盖优先
	if cfg := req.ThkConfig; cfg != nil {
		switch cfg.Mode {
		case thinking.ModeBudget:
			result["reasoning_budget"] = cfg.Budget
		case thinking.ModeLevel:
			result["reasoning_effort"] = cfg.Level
		case thinking.ModeAuto:
			// auto — 不发送 effort，由上游推理模型自动决定
		case thinking.ModeNone:
			result["reasoning_effort"] = "none"
		}
	}
	if req.Stream {
		result["stream_options"] = map[string]bool{"include_usage": true}
	}
	return result
}
