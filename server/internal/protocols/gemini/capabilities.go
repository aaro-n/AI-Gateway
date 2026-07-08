package gemini

// GeminiCapabilities 声明 Gemini 协议支持的所有能力
var GeminiCapabilities = map[string]struct{}{
	// 通用 (所有协议支持)
	"text_chat":   {},
	"temperature": {},
	"max_tokens":  {},
	"stop":        {},
	"stream":      {},
	"image_input": {},

	// 采样
	"top_p": {},
	"top_k": {},

	// 工具
	"function_calling":   {},
	"tool_choice":        {},
	"code_execution":     {},
	"thought_signatures": {},

	// 缓存
	"context_caching": {},

	// 输出
	"logprobs":  {},
	"n_choices": {},

	// 安全
	"safety_settings": {},

	// 搜索
	"google_search": {},

	// 媒体
	"video_input": {},
	"audio_input": {},
}
