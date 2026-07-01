// Package reasonmap 提供跨协议 finish_reason / stop_reason 双向映射表。
// 参考: New-API relay/reasonmap + AxonHub llm/model.go FinishReason 常量
package reasonmap

// =============================================================================
// Anthropic stop_reason ↔ Unified finish_reason
// =============================================================================

// AnthropicToUnified 将 Anthropic stop_reason 转为 unified finish_reason。
func AnthropicToUnified(stopReason string) string {
	switch stopReason {
	case "end_turn":
		return "stop"
	case "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	case "refusal":
		return "content_filter"
	default:
		return stopReason
	}
}

// UnifiedToAnthropic 将 unified finish_reason 转为 Anthropic stop_reason。
func UnifiedToAnthropic(finishReason string) string {
	switch finishReason {
	case "stop":
		return "end_turn"
	case "stop_sequence":
		return "stop_sequence"
	case "length", "max_tokens":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "refusal"
	default:
		return finishReason
	}
}

// =============================================================================
// Gemini finishReason ↔ Unified finish_reason
// =============================================================================

// GeminiToUnified 将 Gemini finishReason 转为 unified finish_reason。
func GeminiToUnified(finishReason string) string {
	switch finishReason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY":
		return "content_filter"
	case "RECITATION":
		return "content_filter"
	case "MALFORMED_FUNCTION_CALL":
		return "tool_calls"
	default:
		return finishReason
	}
}

// UnifiedToGemini 将 unified finish_reason 转为 Gemini finishReason。
func UnifiedToGemini(finishReason string) string {
	switch finishReason {
	case "stop":
		return "STOP"
	case "length", "max_tokens":
		return "MAX_TOKENS"
	case "tool_calls":
		return "MALFORMED_FUNCTION_CALL"
	case "content_filter":
		return "SAFETY"
	default:
		return finishReason
	}
}

// =============================================================================
// Unified ↔ OpenAI (OpenAI 是基准格式，但 content_filter 需要处理)
// =============================================================================

// OpenAIToUnified 将 OpenAI finish_reason 标准化。
func OpenAIToUnified(finishReason string) string {
	if finishReason == "" || finishReason == "null" {
		return ""
	}
	return finishReason
}

// UnifiedToOpenAI 将 unified 还原为 OpenAI finish_reason。
func UnifiedToOpenAI(finishReason string) string {
	return finishReason
}
