package unified

// PromptTokensDetails 输入 token 子项详情。
type PromptTokensDetails struct {
	AudioTokens       int64 `json:"audio_tokens,omitempty"`
	CachedTokens      int64 `json:"cached_tokens,omitempty"`
	WriteCachedTokens int64 `json:"write_cached_tokens,omitempty"`
}

// CompletionTokensDetails 输出 token 子项详情。
type CompletionTokensDetails struct {
	AudioTokens              int64 `json:"audio_tokens,omitempty"`
	ReasoningTokens          int64 `json:"reasoning_tokens,omitempty"`
	AcceptedPredictionTokens int64 `json:"accepted_prediction_tokens,omitempty"`
	RejectedPredictionTokens int64 `json:"rejected_prediction_tokens,omitempty"`
}

// DetailedUsage 含子项详情的统一用量，兼容 OpenAI / Anthropic / Gemini 等厂商。
// 从上游原始 usage 填入此结构后，各 FormatUnified 可按需转回厂商格式。
type DetailedUsage struct {
	PromptTokens            int64                    `json:"prompt_tokens"`
	CompletionTokens        int64                    `json:"completion_tokens"`
	TotalTokens             int64                    `json:"total_tokens"`
	PromptTokensDetails     *PromptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitempty"`
}

// ToDetailedUsage 将现有 Usage（简单 int 字段）升级为 DetailedUsage。
// 适用于已有代码只需 InputTokens / OutputTokens，不丢数据。
func (u Usage) ToDetailedUsage() *DetailedUsage {
	return &DetailedUsage{
		PromptTokens:     int64(u.InputTokens),
		CompletionTokens: int64(u.OutputTokens),
		TotalTokens:      int64(u.TotalTokens()),
	}
}

// ToUsage 将 DetailedUsage 降级为简单 Usage。
func (d *DetailedUsage) ToUsage() Usage {
	if d == nil {
		return Usage{}
	}
	return Usage{
		InputTokens:  int(d.PromptTokens),
		OutputTokens: int(d.CompletionTokens),
	}
}

// OpenAIToDetailed 将 OpenAI 格式的 usage JSON 转换为 DetailedUsage。
func OpenAIToDetailed(promptTokens, completionTokens, totalTokens int64, cachedTokens int64) *DetailedUsage {
	return &DetailedUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		PromptTokensDetails: &PromptTokensDetails{
			CachedTokens: cachedTokens,
		},
	}
}

// AnthropicToDetailed 将 Anthropic 格式的 usage 转换为 DetailedUsage。
func AnthropicToDetailed(inputTokens, outputTokens, cacheRead, cacheCreation int64) *DetailedUsage {
	return &DetailedUsage{
		PromptTokens:     inputTokens,
		CompletionTokens: outputTokens,
		TotalTokens:      inputTokens + outputTokens,
		PromptTokensDetails: &PromptTokensDetails{
			CachedTokens:      cacheRead,
			WriteCachedTokens: cacheCreation,
		},
	}
}

// GeminiToDetailed 将 Gemini 格式的 usage 转换为 DetailedUsage。
func GeminiToDetailed(promptTokens, candidatesTokens, totalTokens int64) *DetailedUsage {
	return &DetailedUsage{
		PromptTokens:     promptTokens,
		CompletionTokens: candidatesTokens,
		TotalTokens:      totalTokens,
	}
}
