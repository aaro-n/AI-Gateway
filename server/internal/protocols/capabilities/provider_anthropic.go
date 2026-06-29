package capabilities

// anthropic 返回 Anthropic 协议的能力定义
func (r *Registry) anthropic() *ProtocolCaps {
	return &ProtocolCaps{
		Protocol:    "anthropic",
		Label:       "Anthropic",
		Description: "Anthropic Messages API，Claude 系列模型的原生协议。以安全性、长上下文和 extended thinking 见长。",
		Capabilities: []Capability{
			// ── 核心能力 ──
			{Key: "streaming", Label: "流式输出 (SSE)", Description: "支持 Server-Sent Events 流式返回，包含多种事件类型（message_start, content_block_delta, message_stop 等）", Category: "core"},
			{Key: "chat_completion", Label: "Messages API", Description: "/v1/messages 端点，Anthropic 的标准对话接口", Category: "core"},
			{Key: "system_prompt", Label: "系统提示 (System Prompt)", Description: "顶层 system 字段，支持字符串或数组格式，与 messages 分离设计", Category: "core"},
			{Key: "multi_turn", Label: "多轮对话", Description: "user/assistant 交替的消息数组，每轮可包含多个 content block", Category: "core"},
			{Key: "token_usage", Label: "Token 用量统计", Description: "响应返回 input_tokens / output_tokens，含 cache_creation_input_tokens / cache_read_input_tokens", Category: "core"},

			// ── 输出控制 ──
			{Key: "function_calling", Label: "工具调用 (Tool Use)", Description: "原生 tool_use content block，支持 tool_choice 精确控制（auto/any/tool）", Category: "output"},
			{Key: "thinking", Label: "扩展思考 (Extended Thinking)", Description: "thinking budget_tokens 参数，Claude 在回答前进行深度思考，支持签名验证和思考过程可见", Category: "output"},
			{Key: "computer_use", Label: "计算机操作 (Computer Use)", Description: "支持屏幕截图理解和鼠标/键盘操作指令（Beta）", Category: "output"}, {Key: "web_search", Label: "网页搜索 (Web Search)", Description: "通过 tool_use type=web_search_20250305 激活服务端网页搜索", Category: "output"},
			// ── 输入控制 ──
			{Key: "temperature_control", Label: "Temperature 控制", Description: "控制输出随机性 (0-1)，支持接近 0 的确定性输出", Category: "input"},
			{Key: "top_p", Label: "Top-P (核采样)", Description: "nucleus sampling 概率阈值控制", Category: "input"},
			{Key: "top_k", Label: "Top-K 采样", Description: "仅从概率最高的 K 个 token 中采样", Category: "input"},
			{Key: "stop_sequences", Label: "停止序列", Description: "自定义停止序列，遇到即终止生成", Category: "input"},
			{Key: "max_tokens", Label: "最大 Token 限制", Description: "max_tokens 精确控制输出长度（必填参数）", Category: "input"},

			// ── 多模态 ──
			{Key: "vision", Label: "视觉识别 (Vision)", Description: "原生支持图片输入（Base64），支持多种图片格式，可精确指定图片在文本中的位置", Category: "advanced"},
			{Key: "pdf_support", Label: "PDF 文档理解", Description: "支持 PDF 文档作为输入，Claude 可直接理解文档内容", Category: "advanced"},

			// ── 高级功能 ──
			{Key: "prompt_caching", Label: "Prompt 缓存", Description: "支持标记可缓存内容，减少重复上下文的 token 消耗和延迟（cache_control）", Category: "advanced"},
			{Key: "citations", Label: "文本引用 (Citations)", Description: "自动为回答中的事实性陈述添加源文档引用", Category: "advanced"},
			{Key: "stream_events", Label: "流式事件类型", Description: "丰富的 SSE 事件类型：message_start/delta/stop, content_block_start/delta/stop, ping", Category: "advanced"},
			{Key: "disable_parallel_tool_use", Label: "禁用并行工具调用", Description: "disable_parallel_tool_use 参数可强制串行执行工具，避免竞态", Category: "advanced"},
		},
	}
}
