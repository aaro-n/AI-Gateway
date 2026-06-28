package capabilities

// gemini 返回 Google Gemini 协议的能力定义
func (r *Registry) gemini() *ProtocolCaps {
	return &ProtocolCaps{
		Protocol:    "gemini",
		Label:       "Google Gemini",
		Description: "Google Gemini API，Google 的多模态大模型原生协议。以原生多模态和超长上下文（最高 200 万 token）为亮点。",
		Capabilities: []Capability{
			// ── 核心能力 ──
			{Key: "streaming", Label: "流式输出 (SSE)", Description: "支持 streamGenerateContent，通过 alt=sse 实现 SSE 流式返回", Category: "core"},
			{Key: "chat_completion", Label: "Generate Content API", Description: "generateContent / streamGenerateContent 端点，Gemini 的标准对话接口", Category: "core"},
			{Key: "system_prompt", Label: "系统指令 (System Instruction)", Description: "顶层 system_instruction 字段，与 contents 分离设计", Category: "core"},
			{Key: "multi_turn", Label: "多轮对话", Description: "contents 数组支持 user/model 角色交替的多轮对话", Category: "core"},
			{Key: "token_usage", Label: "Token 用量统计", Description: "响应返回 usageMetadata: promptTokenCount / candidatesTokenCount / totalTokenCount", Category: "core"},

			// ── 输出控制 ──
			{Key: "function_calling", Label: "函数调用 (Function Calling)", Description: "原生 functionDeclarations 定义，支持自动/手动/强制三种模式", Category: "output"},
			{Key: "json_mode", Label: "JSON 模式", Description: "response_mime_type: application/json + response_schema 确保输出有效 JSON", Category: "output"},
			{Key: "structured_output", Label: "结构化输出", Description: "response_schema 定义严格的输出 Schema，支持 controlled generation", Category: "output"},
			{Key: "code_execution", Label: "代码执行", Description: "支持内置代码执行引擎，可运行 Python 代码并获取结果", Category: "output"},
			{Key: "google_search", Label: "Google 搜索接地", Description: "支持 Google Search grounding，自动搜索最新信息增强回答", Category: "output"},

			// ── 输入控制 ──
			{Key: "temperature_control", Label: "Temperature 控制", Description: "控制输出随机性 (0-2)", Category: "input"},
			{Key: "top_p", Label: "Top-P (核采样)", Description: "nucleus sampling 概率阈值控制", Category: "input"},
			{Key: "top_k", Label: "Top-K 采样", Description: "仅从概率最高的 K 个 token 中采样（默认 40）", Category: "input"},
			{Key: "stop_sequences", Label: "停止序列", Description: "自定义最多 5 组停止序列", Category: "input"},
			{Key: "max_tokens", Label: "最大 Token 限制", Description: "maxOutputTokens 控制最大输出长度", Category: "input"},
			{Key: "seed", Label: "随机种子 (Seed)", Description: "设置随机种子实现确定性输出", Category: "input"},

			// ── 多模态 ──
			{Key: "vision", Label: "视觉识别 (Vision)", Description: "原生多模态，支持图片、视频输入，原生高效处理", Category: "advanced"},
			{Key: "audio_input", Label: "音频输入", Description: "支持音频文件直接输入，原生音频理解", Category: "advanced"},
			{Key: "video_input", Label: "视频输入", Description: "支持视频文件输入，逐帧分析理解视频内容", Category: "advanced"},

			// ── 高级功能 ──
			{Key: "caching", Label: "上下文缓存 (Context Caching)", Description: "支持缓存大型上下文（如视频、文档），降低重复请求成本", Category: "advanced"},
			{Key: "safety_settings", Label: "安全过滤设置", Description: "细粒度的安全过滤器（harassment/hate/dangerous/sexual），可调整阈值", Category: "advanced"},
			{Key: "candidate_count", Label: "多候选回复", Description: "candidateCount 参数支持单次请求返回多个候选回复", Category: "advanced"},
			{Key: "thought_signatures", Label: "思考签名验证", Description: "流式响应中包含 thought 内容，可验证思考过程完整性", Category: "advanced"},
		},
	}
}
