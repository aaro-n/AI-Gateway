// Package unified 定义协议无关的统一中间表示 (Unified Intermediate Representation)。
//
// 设计目标：实现真正的轴辐式 (Hub-and-Spoke) 协议转换。
// 每个协议插件只需实现两个方向的转换：
//   - ToUnified:   本协议请求  → UnifiedRequest
//   - FromUnified: UnifiedRequest → 本协议请求，发上游，返回 UnifiedResponse/Stream
//
// 通用中间件 (UnifiedGatewayHandler) 永远只操作 Unified 类型，
// 新增协议时无需修改中间件，也无需修改其他协议插件。
//
// 中间表示选择 OpenAI Chat Completions 格式的"超集"，
// 因为它字段最全、生态最广，且现有转换代码已大量基于此格式。
package unified

import (
	"context"
	"encoding/json"

	"ai-gateway/internal/core/unified/thinking"
)

// =============================================================================
// 请求侧
// =============================================================================

// Request 统一请求中间表示。
// 字段以 OpenAI Chat Completions 为基础，扩展容纳 Anthropic/Gemini/DeepSeek 的差异。
type Request struct {
	Model            string          `json:"model"`
	Messages         []Message       `json:"messages"`
	SystemPrompt     string          `json:"system_prompt,omitempty"` // Anthropic 顶层 system / Gemini systemInstruction
	MaxTokens        int             `json:"max_tokens,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	TopK             *int            `json:"top_k,omitempty"`             // Anthropic/Gemini top_k 采样
	Seed             *int            `json:"seed,omitempty"`              // 确定性种子（OpenAI/DeepSeek）
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"` // 频率惩罚（-2.0 到 2.0）
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`  // 存在惩罚（-2.0 到 2.0）
	Stream           bool            `json:"stream,omitempty"`
	Tools            json.RawMessage `json:"tools,omitempty"`           // 原始 tools JSON（透传，兼容 web_search 等非 function 工具）
	ToolChoice       json.RawMessage `json:"tool_choice,omitempty"`     // 各协议差异大，保留原始 JSON
	ResponseFormat   json.RawMessage `json:"response_format,omitempty"` // JSON 模式 / 结构化输出
	Stop             []string        `json:"stop,omitempty"`
	ReasoningEffort  string          `json:"reasoning_effort,omitempty"` // o1/Claude/DeepSeek 推理深度
	ReasoningBudget  *int            `json:"reasoning_budget,omitempty"` // Claude thinking budget_tokens
	Modalities       []string        `json:"modalities,omitempty"`       // 输出模态 ["text","audio","image"] — OpenAI

	// ThkConfig 思考管道配置（由网关在路由后注入，FromUnified 消费）。
	// 优先于 ReasoningEffort / ReasoningBudget，表示已通过校验和转换的最终配置。
	ThkConfig *thinking.ThinkingConfig `json:"-"`

	// 供应商特有字段透传 (AxonHub 风格 TransformerMetadata)。
	// 包含: cache_control 断点、thinking type/display 等无法映射到 unified 标准字段的供应商专有数据。
	// 注意: 仅用于同协议穿透或相近协议的精确还原，跨大协议转换时大概率丢失。
	TransformerMetadata map[string]any `json:"-"`

	// 源协议标记（响应反向转换时需要知道返回什么格式给客户端）
	SourceProtocol string `json:"-"`

	// 请求上下文（用于流式取消、超时控制）
	Ctx context.Context `json:"-"`
}

// Message 统一消息（OpenAI 风格，支持 content blocks）
type Message struct {
	Role               string          `json:"role"`                          // system/user/assistant/tool
	Content            json.RawMessage `json:"content"`                       // string 或 []ContentBlock
	ReasoningContent   string          `json:"reasoning_content,omitempty"`   // DeepSeek/o1 思考链（多轮对话须原样传回）
	ReasoningSignature *string         `json:"reasoning_signature,omitempty"` // Anthropic thinking signature — 完整性验证
	ToolCalls          []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID         string          `json:"tool_call_id,omitempty"`
	Name               string          `json:"name,omitempty"`
	Prefix             bool            `json:"prefix,omitempty"` // DeepSeek Chat Prefix Completion

	// 供应商特有字段透传。Anthropic 用: "cache_control"
	TransformerMetadata map[string]any `json:"-"`
}

// ToolCall 工具调用（OpenAI 风格）
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

// Tool 工具定义（OpenAI 风格）
type Tool struct {
	Type     string      `json:"type"` // "function"
	Function FunctionDef `json:"function"`
}

// FunctionDef 函数定义
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema
}

// ContentBlock 通用内容块（覆盖 text/image/tool_use/tool_result）
type ContentBlock struct {
	Type      string          `json:"type"` // text/image_url/image/tool_use/tool_result
	Text      string          `json:"text,omitempty"`
	ImageURL  *ImageURL       `json:"image_url,omitempty"`
	Image     *ImageSource    `json:"image,omitempty"` // Anthropic 风格
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"` // tool_result 的内容

	// 供应商特有字段透传。Anthropic 用: "cache_control" — Prompt Caching 断点标记
	TransformerMetadata map[string]any `json:"-"`
}

// CacheControl Anthropic Prompt Caching 控制，附着于 ContentBlock 或 SystemPrompt。
// 参见 AxonHub: llm/model.go CacheControl / new-api: dto/claude.go CacheControl
type CacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

// ThinkingConfig Anthropic extended thinking 配置。
// Claude API: thinking: {type: "enabled"|"disabled"|"adaptive", budget_tokens: N}
// Gemini API: thinkingConfig: {thinkingBudget: N, includeThoughts: bool}
type ThinkingConfig struct {
	Type            string `json:"type,omitempty"`             // "enabled", "disabled", "adaptive"
	BudgetTokens    *int   `json:"budget_tokens,omitempty"`    // Claude 专用
	IncludeThoughts *bool  `json:"include_thoughts,omitempty"` // Gemini 专用 — 是否在响应中包含思考内容
}

// ImageURL OpenAI 风格图片
type ImageURL struct {
	URL string `json:"url"`
}

// ImageSource Anthropic 风格图片源
type ImageSource struct {
	Type      string `json:"type"` // base64 / url
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

// =============================================================================
// 响应侧（非流式）
// =============================================================================

// Response 统一响应中间表示（非流式）
type Response struct {
	ID                 string     `json:"id"`
	Model              string     `json:"model"`
	Content            string     `json:"content"`                       // 文本内容合并
	ReasoningContent   string     `json:"reasoning_content,omitempty"`   // o1/Claude 思考链
	ReasoningSignature *string    `json:"reasoning_signature,omitempty"` // Anthropic thinking signature
	ToolCalls          []ToolCall `json:"tool_calls,omitempty"`
	FinishReason       string     `json:"finish_reason"` // stop/length/tool_calls/content_filter
	Usage              Usage      `json:"usage"`

	// 供应商特有字段透传。Anthropic 用: "stop_sequence" 原始值
	TransformerMetadata map[string]any `json:"-"`
}

// Finish reason 常量 — 参考 New-API reasonmap
const (
	FinishReasonStop          = "stop"
	FinishReasonLength        = "length"
	FinishReasonToolCalls     = "tool_calls"
	FinishReasonFunctionCall  = "function_call"
	FinishReasonContentFilter = "content_filter"
)

// Usage 统一用量
type Usage struct {
	CachedTokens    int `json:"cached_tokens,omitempty"`
	InputTokens     int `json:"input_tokens"`
	OutputTokens    int `json:"output_tokens"`
	CacheHitTokens  int `json:"cache_hit_tokens,omitempty"`  // DeepSeek prompt_cache_hit_tokens
	CacheMissTokens int `json:"cache_miss_tokens,omitempty"` // DeepSeek prompt_cache_miss_tokens
}

func (u Usage) TotalTokens() int {
	return u.CachedTokens + u.InputTokens + u.OutputTokens
}

// =============================================================================
// 响应侧（流式）
// =============================================================================

// StreamEvent 统一流式事件
type StreamEvent struct {
	Type         StreamEventType `json:"type"`
	Delta        *Delta          `json:"delta,omitempty"`
	Usage        *Usage          `json:"usage,omitempty"`
	FinishReason string          `json:"finish_reason,omitempty"`
	MessageID    string          `json:"-"` // Anthropic message_start 中的 msg ID（用于 FormatUnified 还原 message_start）
	Model        string          `json:"-"` // Anthropic message_start 中的 model（用于 FormatUnified）
}

// StreamEventType 事件类型
type StreamEventType string

const (
	EventChunk StreamEventType = "chunk" // 内容增量
	EventUsage StreamEventType = "usage" // 用量更新
	EventDone  StreamEventType = "done"  // 流结束
	EventError StreamEventType = "error" // 错误
)

// Delta 增量内容
type Delta struct {
	Content            string     `json:"content,omitempty"`
	ReasoningContent   string     `json:"reasoning_content,omitempty"`   // o1/Claude 思考链增量
	ReasoningSignature *string    `json:"reasoning_signature,omitempty"` // Anthropic signature_delta
	ToolCalls          []ToolCall `json:"tool_calls,omitempty"`
	InputJSON          string     `json:"input_json,omitempty"` // Anthropic input_json_delta 工具参数增量
	Role               string     `json:"role,omitempty"`

	// 供应商特有字段透传
	TransformerMetadata map[string]any `json:"-"`
}
