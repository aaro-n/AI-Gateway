package capabilities

// openRouter 返回 OpenRouter 协议的能力定义
func (r *Registry) openRouter() *ProtocolCaps {
	return &ProtocolCaps{
		Protocol:    "openrouter",
		Label:       "OpenRouter",
		Description: "OpenRouter 聚合 API，兼容 OpenAI Chat Completions 格式。统一接入多种模型提供商，支持 200+ 模型。",
		Capabilities: []Capability{
			// ── 核心能力 ──
			{Key: "streaming", Label: "流式输出 (SSE)", Description: "支持 Server-Sent Events 流式返回，实时逐 token 输出", Category: "core"},
			{Key: "chat_completion", Label: "Chat Completion", Description: "/chat/completions 端点，标准对话接口", Category: "core"},
			{Key: "system_prompt", Label: "系统提示 (System Prompt)", Description: "通过 messages 中 role=system 设定 AI 行为规则", Category: "core"},
			{Key: "multi_turn", Label: "多轮对话", Description: "完整的 user/assistant/tool 多角色消息数组", Category: "core"},
			{Key: "token_usage", Label: "Token 用量统计", Description: "每次响应返回 prompt_tokens / completion_tokens / total_tokens 使用量", Category: "core"},
			{Key: "multi_provider", Label: "多提供商聚合", Description: "统一 API 接入 Anthropic、Google、Meta、DeepSeek 等多家模型提供商", Category: "core"},

			// ── 输出控制 ──
			{Key: "function_calling", Label: "函数调用 (Function Calling)", Description: "原生 tools/functions 定义，支持 tool_choice 精确控制", Category: "output"},
			{Key: "json_mode", Label: "JSON 模式", Description: "response_format: json_object 确保输出有效 JSON", Category: "output"},
			{Key: "structured_output", Label: "结构化输出", Description: "response_format: json_schema 定义严格的 JSON Schema 输出", Category: "output"},

			// ── 输入控制 ──
			{Key: "temperature_control", Label: "Temperature 控制", Description: "精确控制输出随机性 (0-2)", Category: "input"},
			{Key: "top_p", Label: "Top-P (核采样)", Description: "nucleus sampling 概率阈值控制", Category: "input"},
			{Key: "top_k", Label: "Top-K 采样", Description: "限制候选 token 数量，仅考虑概率最高的 K 个 token", Category: "input"},
			{Key: "frequency_penalty", Label: "频率惩罚 (Frequency Penalty)", Description: "抑制重复 token，范围 -2.0 到 2.0", Category: "input"},
			{Key: "presence_penalty", Label: "存在惩罚 (Presence Penalty)", Description: "鼓励/抑制提及新话题，范围 -2.0 到 2.0", Category: "input"},
			{Key: "seed", Label: "随机种子 (Seed)", Description: "设置确定性输出的随机种子，确保重复请求结果一致", Category: "input"},
			{Key: "stop_sequences", Label: "停止序列", Description: "自定义停止词，遇到即终止生成", Category: "input"},
			{Key: "max_tokens", Label: "最大 Token 限制", Description: "max_tokens / max_completion_tokens 精确控制输出长度", Category: "input"},

			// ── 多模态 ──
			{Key: "vision", Label: "视觉理解", Description: "支持图片输入的视觉模型（如 GPT-4o、Claude 3.5、Gemini Pro Vision 等）", Category: "multimodal"},

			// ── 高级功能 ──
			{Key: "stream_options", Label: "流式用量统计", Description: "stream_options.include_usage 在流式结束时返回 token 用量", Category: "advanced"},
			{Key: "reasoning", Label: "推理/思维链", Description: "支持 reasoning_content 思维链输出的推理模型", Category: "advanced"},
			{Key: "reasoning_effort", Label: "推理深度控制", Description: "reasoning_effort 参数精确控制推理计算资源", Category: "advanced"},
		},
	}
}
