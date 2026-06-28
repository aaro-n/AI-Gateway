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

import "encoding/json"

// =============================================================================
// 请求侧
// =============================================================================

// Request 统一请求中间表示。
// 字段以 OpenAI Chat Completions 为基础，扩展容纳 Anthropic/Gemini 的差异。
type Request struct {
	Model           string          `json:"model"`
	Messages        []Message       `json:"messages"`
	SystemPrompt    string          `json:"system_prompt,omitempty"` // Anthropic 顶层 system / Gemini systemInstruction
	MaxTokens       int             `json:"max_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	Tools           []Tool          `json:"tools,omitempty"`
	ToolChoice      json.RawMessage `json:"tool_choice,omitempty"` // 各协议差异大，保留原始 JSON
	Stop            []string        `json:"stop,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"` // o1/claude thinking 等推理模型

	// 源协议标记（响应反向转换时需要知道返回什么格式给客户端）
	SourceProtocol string `json:"-"`
}

// Message 统一消息（OpenAI 风格，支持 content blocks）
type Message struct {
	Role       string          `json:"role"`    // system/user/assistant/tool
	Content    json.RawMessage `json:"content"` // string 或 []ContentBlock
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
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
	ID               string     `json:"id"`
	Model            string     `json:"model"`
	Content          string     `json:"content"`                     // 文本内容合并
	ReasoningContent string     `json:"reasoning_content,omitempty"` // o1/Claude 思考链
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	FinishReason     string     `json:"finish_reason"` // stop/length/tool_calls
	Usage            Usage      `json:"usage"`
}

// Usage 统一用量
type Usage struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
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
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"` // o1/Claude 思考链增量
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	InputJSON        string     `json:"input_json,omitempty"` // Anthropic input_json_delta 工具参数增量
	Role             string     `json:"role,omitempty"`
}
