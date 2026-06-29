package capabilities

// deepseek 返回 DeepSeek 协议的能力定义
func (r *Registry) deepseek() *ProtocolCaps {
	return &ProtocolCaps{
		Protocol:    "deepseek",
		Label:       "DeepSeek",
		Description: "DeepSeek API 协议，兼容 OpenAI Chat Completions 格式。支持 deepseek-chat 和 deepseek-reasoner 等模型。",
		Capabilities: []Capability{
			// ── 核心能力 ──
			{Key: "streaming", Label: "流式输出 (SSE)", Description: "支持 Server-Sent Events 流式返回，实时逐 token 输出", Category: "core"},
			{Key: "chat_completion", Label: "Chat Completion", Description: "/chat/completions 端点，标准对话接口", Category: "core"},
			{Key: "system_prompt", Label: "系统提示 (System Prompt)", Description: "通过 messages 中 role=system 设定 AI 行为规则", Category: "core"},
			{Key: "multi_turn", Label: "多轮对话", Description: "完整的 user/assistant/tool 多角色消息数组", Category: "core"},
			{Key: "token_usage", Label: "Token 用量统计", Description: "每次响应返回 prompt_tokens / completion_tokens / total_tokens 使用量", Category: "core"},

			// ── 输出控制 ──
			{Key: "function_calling", Label: "函数调用 (Function Calling)", Description: "原生 tools/functions 定义，支持 tool_choice 精确控制", Category: "output"},
			{Key: "json_mode", Label: "JSON 模式", Description: "response_format: json_object 确保输出有效 JSON", Category: "output"},
			{Key: "structured_output", Label: "结构化输出", Description: "response_format: json_schema 定义严格的 JSON Schema 输出", Category: "output"},

			// ── 输入控制 ──
			{Key: "temperature_control", Label: "Temperature 控制", Description: "精确控制输出随机性 (0-2)", Category: "input"},
			{Key: "top_p", Label: "Top-P (核采样)", Description: "nucleus sampling 概率阈值控制", Category: "input"},
			{Key: "frequency_penalty", Label: "频率惩罚 (Frequency Penalty)", Description: "抑制重复 token，范围 -2.0 到 2.0", Category: "input"},
			{Key: "presence_penalty", Label: "存在惩罚 (Presence Penalty)", Description: "鼓励/抑制提及新话题，范围 -2.0 到 2.0", Category: "input"},
			{Key: "stop_sequences", Label: "停止序列", Description: "自定义停止词，遇到即终止生成", Category: "input"},
			{Key: "max_tokens", Label: "最大 Token 限制", Description: "max_tokens / max_completion_tokens 精确控制输出长度", Category: "input"},

			// ── 多模态 ──
			{Key: "vision", Label: "视觉识别 (Vision)", Description: "支持图片输入（URL 或 Base64），理解图像内容", Category: "advanced"},

			// ── 高级功能 ──
			{Key: "stream_options", Label: "流式用量统计", Description: "stream_options.include_usage 在流式结束时返回 token 用量", Category: "advanced"},
			{Key: "thinking", Label: "思考/推理链 (Reasoner)", Description: "deepseek-reasoner 模型支持 depth reasoning，返回 reasoning_content", Category: "advanced"},
			{Key: "reasoning_effort", Label: "推理深度控制", Description: "reasoning_effort 参数 (high/max) 精确控制推理计算资源", Category: "advanced"},
			{Key: "prefix_completion", Label: "前缀补全 (Chat Prefix)", Description: "消息 prefix: true 标识强制模型沿给定前缀续写", Category: "advanced"},
			{Key: "context_caching", Label: "上下文缓存", Description: "服务端前缀缓存，返回 prompt_cache_hit/miss_tokens 统计", Category: "advanced"},
			{Key: "web_search", Label: "网页搜索 (Web Search)", Description: "通过内置 tools 激活服务端网页搜索能力", Category: "advanced"},
			{Key: "parallel_tool_calls", Label: "并行工具调用", Description: "支持单次响应并行调用多个工具", Category: "advanced"},
		},
	}
}
