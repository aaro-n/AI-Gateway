package deepseek

import (
	"ai-gateway/internal/core/unified"
	"encoding/json"
)

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
