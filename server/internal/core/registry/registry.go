package registry

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/unified"
	"ai-gateway/internal/protocols/capabilities"
)

// =============================================================================
// Provider 接口 — 每个协议必须实现（轴辐式：只与 Unified 中间表示互转）
// =============================================================================

// Config Provider 通用配置
type Config struct {
	BaseURL string
	APIKey  string
}

// Usage Token 用量统计（保留以兼容现有代码，等价于 unified.Usage）
type Usage struct {
	CachedTokens    int
	CacheHitTokens  int
	CacheMissTokens int
	InputTokens     int
	OutputTokens    int
}

func (u Usage) TotalTokens() int {
	return u.CachedTokens + u.InputTokens + u.OutputTokens
}

// ToUnifiedUsage 转换为 unified.Usage
func (u Usage) ToUnified() unified.Usage {
	return unified.Usage{
		CachedTokens:    u.CachedTokens,
		CacheHitTokens:  u.CacheHitTokens,
		CacheMissTokens: u.CacheMissTokens,
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
	}
}

// FromUnifiedUsage 从 unified.Usage 转换
func FromUnifiedUsage(u unified.Usage) Usage {
	return Usage{
		CachedTokens:    u.CachedTokens,
		CacheHitTokens:  u.CacheHitTokens,
		CacheMissTokens: u.CacheMissTokens,
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
	}
}

// ProviderModel 上游模型信息（用于模型同步）
type ProviderModel struct {
	ProviderID     uint    `json:"provider_id"`
	ModelID        string  `json:"model_id"`
	DisplayName    string  `json:"display_name"`
	OwnedBy        string  `json:"owned_by"`
	ContextWindow  int     `json:"context_window"`
	MaxOutput      int     `json:"max_output"`
	InputPrice     float64 `json:"input_price"`
	OutputPrice    float64 `json:"output_price"`
	SupportsVision bool    `json:"supports_vision"`
	SupportsTools  bool    `json:"supports_tools"`
	SupportsStream bool    `json:"supports_stream"`
	IsAvailable    bool    `json:"is_available"`
	Source         string  `json:"source"`
}

// Provider 协议 Provider 接口 — 每个协议插件必须实现
//
// 轴辐式架构：每个协议只与 unified 中间表示互转，不直接与其他协议对话。
// 通用中间件 (UnifiedGatewayHandler) 负责：
//
//	入口协议.ToUnified(body) → *unified.Request
//	路由选择上游协议
//	上游协议.FromUnified(req) → 发请求 → 返回 unified.Response 或 <-chan unified.StreamEvent
//	入口协议.FormatUnified(resp/stream) → 客户端格式
type Provider interface {
	// SyncModels 从上游同步模型列表
	SyncModels(providerID uint) ([]ProviderModel, error)

	// ToUnified 将本协议的客户端请求体解析为统一中间表示。
	// body 是原始 HTTP 请求体；modelID 是路由后要替换的目标上游模型名。
	ToUnified(body []byte, modelID string) (*unified.Request, error)

	// FromUnified 将统一中间表示转换为本协议格式，发送到上游，返回统一响应。
	// 非流式：返回 *unified.Response；流式：返回 <-chan unified.StreamEvent（chan 关闭表示结束）。
	// 二者通过 req.Stream 字段判断。
	FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error)

	// FormatUnified 将统一响应/流式事件格式化回本协议的客户端格式并写入 dst。
	// 非流式：传入 resp 和 nil chan；流式：传入 nil resp 和 events chan。
	FormatUnified(resp *unified.Response, events <-chan unified.StreamEvent, dst *gin.Context, usage *Usage) error
}

// =============================================================================
// Inbound / Outbound — 可选的职责分离接口 (AxonHub 风格)
//
// 如果协议实现了 Inbound 和 Outbound，中间件优先使用分离接口；
// 否则回退到 Provider 的 ToUnified/FromUnified/FormatUnified 方法。
// =============================================================================

// Inbound 处理客户端请求解析和响应格式化（面向客户端）。
type Inbound interface {
	// ParseRequest 将客户端原生请求体解析为 unified.Request。
	// modelID 是路由后的目标上游模型名。
	ParseRequest(body []byte, modelID string) (*unified.Request, error)

	// FormatResponse 将 unified 响应/流格式化为客户端原生格式。
	FormatResponse(resp *unified.Response, events <-chan unified.StreamEvent, dst *gin.Context, usage *Usage) error
}

// Outbound 处理 unified → 上游原生格式的转换和调用（面向上游）。
type Outbound interface {
	// BuildRequest 将 unified.Request 转换为上游原生 HTTP 请求并调用。
	// 非流式：返回 *unified.Response；流式：返回 <-chan unified.StreamEvent。
	// 二者通过 req.Stream 字段判断。
	BuildRequest(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error)
}

// =============================================================================
// StreamHintProvider — 可选接口，让无法从 body 判断流式的协议提供流式提示
//
// 大多数协议（OpenAI/Anthropic/DeepSeek）在请求体中的 "stream" 字段指示流式模式。
// Gemini 通过 URL 路径（"streamGenerateContent" vs "generateContent"）区分流式，
// body 中无此信息。实现此接口可让 UnifiedGatewayHandler 在 ToUnified 之后
// 正确设置 unifiedReq.Stream，避免协议特定的硬编码分支。
// =============================================================================

// StreamHintProvider 可选的流式提示接口
type StreamHintProvider interface {
	// IsStreamRequest 从 HTTP 请求判断是否为流式请求
	IsStreamRequest(c *gin.Context) bool
}

// =============================================================================
// 测试接口 — 每个协议必须提供
// =============================================================================

// TestExecutor 测试执行器
type TestExecutor interface {
	// BuildTestRequest 构建测试请求体
	BuildTestRequest(modelID string) ([]byte, error)
	// ExecuteTest 执行测试请求
	ExecuteTest(ctx *gin.Context, modelID string, usage *Usage) error
	// ExtractContent 从响应中提取文本内容
	ExtractContent(body []byte) string
}

// =============================================================================
// Form Schema — 前端动态表单
// =============================================================================

// FormField 表单字段定义
type FormField struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Type        string   `json:"type"` // url, text, number, password, select, switch
	Placeholder string   `json:"placeholder,omitempty"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"` // select 类型的选项
}

// =============================================================================
// ProtocolDescriptor — 协议注册项
// =============================================================================

// ProtocolDescriptor 协议描述符
type ProtocolDescriptor struct {
	// ── 身份标识 ──
	Name  string `json:"name"`  // 唯一标识 "openai", "anthropic", "gemini"
	Label string `json:"label"` // 显示名称 "OpenAI", "Anthropic", "Google Gemini"

	// ── 能力声明（声明式转换损失检测） ──
	// 注册时通过 capabilities.Get(desc.Name) 自动填充。
	// 若为 nil，跨协议损失检测退化为仅输出警告列表。
	Capabilities *capabilities.ProtocolCaps `json:"-"`

	// ── Key 格式 ──
	KeyPrefix  string              `json:"key_prefix"` // "sk-", "sk-ant-", "AIza"
	KeyLength  int                 `json:"key_length"` // 24, 24, 26
	KeyEncoder func([]byte) string `json:"-"`          // hex / base64

	// ── 认证 ──
	AuthExtractor func(c *gin.Context) string `json:"-"`

	// ── Provider 工厂 ──
	NewProvider func(cfg *Config) Provider `json:"-"`

	// ── 测试 ──
	TestExecutor TestExecutor `json:"-"`

	// ── 路由注册（可选，默认用通用路由） ──
	RegisterRoutes func(engine *gin.Engine, auth gin.HandlerFunc) `json:"-"`

	// ── DB 列名 ──
	DBColumn string `json:"-"` // 旧扁平列名 "gemini_base_url"；待 JSON Endpoints 迁移后废弃

	// ── 默认值 ──
	DefaultBaseURL string `json:"default_base_url"`

	// ── 前端表单 Schema ──
	FormSchema []FormField `json:"form_schema"`
}

// =============================================================================
// 注册表
// =============================================================================

var protocols = make(map[string]ProtocolDescriptor)

// Register 注册协议
func Register(desc ProtocolDescriptor) {
	protocols[desc.Name] = desc
}

// Get 获取协议
func Get(name string) (ProtocolDescriptor, bool) {
	desc, ok := protocols[name]
	return desc, ok
}

// All 获取所有协议
func All() map[string]ProtocolDescriptor {
	return protocols
}

// Names 获取所有协议名称
func Names() []string {
	names := make([]string, 0, len(protocols))
	for name := range protocols {
		names = append(names, name)
	}
	return names
}

// =============================================================================
// 错误类型
// =============================================================================

// HTTPError 上游 HTTP 错误（非 200 响应）
type HTTPError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("upstream HTTP %d: %s", e.StatusCode, string(e.Body))
}

// IsRateLimit 判断是否为限流错误 (429)
func (e *HTTPError) IsRateLimit() bool {
	return e.StatusCode == 429
}
