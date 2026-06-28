package capabilities

// openAI 返回 OpenAI 协议的能力定义
func (r *Registry) openAI() *ProtocolCaps {
	return &ProtocolCaps{
		Protocol:    "openai",
		Label:       "OpenAI",
		Description: "OpenAI API 协议，业界最广泛使用的 AI API 标准格式。大量第三方模型（vLLM、Ollama、LiteLLM 等）兼容此格式。",
		Capabilities: []Capability{
			// ── 核心能力 ──
			{Key: "streaming", Label: "流式输出 (SSE)", Description: "支持 Server-Sent Events 流式返回，实时逐 token 输出", Category: "core"},
			{Key: "chat_completion", Label: "Chat Completion", Description: "/v1/chat/completions 端点，标准对话接口", Category: "core"},
			{Key: "embeddings", Label: "嵌入向量 (Embeddings)", Description: "/v1/embeddings 端点，生成文本嵌入向量", Category: "core"},
			{Key: "system_prompt", Label: "系统提示 (System Prompt)", Description: "通过 messages 中 role=system 设定 AI 行为规则", Category: "core"},
			{Key: "multi_turn", Label: "多轮对话", Description: "完整的 user/assistant/tool 多角色消息数组", Category: "core"},
			{Key: "token_usage", Label: "Token 用量统计", Description: "每次响应返回 prompt_tokens / completion_tokens / total_tokens 使用量", Category: "core"},

			// ── 输出控制 ──
			{Key: "function_calling", Label: "函数调用 (Function Calling)", Description: "原生 tools/functions 定义，支持 tool_choice 精确控制", Category: "output"},
			{Key: "json_mode", Label: "JSON 模式", Description: "response_format: json_object 确保输出有效 JSON", Category: "output"},
			{Key: "structured_output", Label: "结构化输出", Description: "response_format: json_schema 定义严格的 JSON Schema 输出", Category: "output"},
			{Key: "logprobs", Label: "对数概率 (Logprobs)", Description: "返回每个 token 的对数概率，用于置信度分析", Category: "output"},
			{Key: "n_choices", Label: "多候选回复", Description: "n 参数支持单次请求返回多个候选回复", Category: "output"},

			// ── 输入控制 ──
			{Key: "temperature_control", Label: "Temperature 控制", Description: "精确控制输出随机性 (0-2)", Category: "input"},
			{Key: "top_p", Label: "Top-P (核采样)", Description: "nucleus sampling 概率阈值控制", Category: "input"},
			{Key: "frequency_penalty", Label: "频率惩罚 (Frequency Penalty)", Description: "抑制重复 token，范围 -2.0 到 2.0", Category: "input"},
			{Key: "presence_penalty", Label: "存在惩罚 (Presence Penalty)", Description: "鼓励/抑制提及新话题，范围 -2.0 到 2.0", Category: "input"},
			{Key: "seed", Label: "随机种子 (Seed)", Description: "设置随机种子实现确定性输出（可复现）", Category: "input"},
			{Key: "stop_sequences", Label: "停止序列", Description: "自定义最多 4 组停止词，遇到即终止生成", Category: "input"},
			{Key: "max_tokens", Label: "最大 Token 限制", Description: "max_completion_tokens 精确控制输出长度", Category: "input"},

			// ── 多模态 ──
			{Key: "vision", Label: "视觉识别 (Vision)", Description: "支持图片输入（URL 或 Base64），理解图像内容", Category: "advanced"},
			{Key: "audio_input", Label: "音频输入", Description: "支持音频文件输入（部分模型如 GPT-4o-audio）", Category: "advanced"},

			// ── 高级功能 ──
			{Key: "stream_options", Label: "流式用量统计", Description: "stream_options.include_usage 在流式结束时返回 token 用量", Category: "advanced"},
			{Key: "parallel_tool_calls", Label: "并行工具调用", Description: "支持单次响应并行调用多个工具", Category: "advanced"},
			{Key: "thinking", Label: "思考/推理链 (o1)", Description: "o1 系列模型支持 reasoning_effort 参数控制思考深度", Category: "advanced"},
			{Key: "predicted_outputs", Label: "预测输出加速", Description: "prediction 参数可加速已知输出的生成", Category: "advanced"},
		},
	}
}
