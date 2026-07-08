package anthropic

// AnthropicCapabilities Anthropic 协议能力声明
var AnthropicCapabilities = map[string]struct{}{
	"text_chat": {}, "temperature": {}, "max_tokens": {}, "stop": {}, "stream": {}, "image_input": {},
	"top_p": {}, "top_k": {},
	"function_calling": {}, "tool_choice": {}, "parallel_tool_calls": {}, "computer_use": {},
	"thinking":       {},
	"prompt_caching": {}, "cache_control": {},
	"web_search":  {},
	"pdf_support": {},
	"citations":   {}, "rich_stream_events": {},
}
