package registry

import (
	"github.com/gin-gonic/gin"
)

// =============================================================================
// Provider 接口 — 每个协议必须实现
// =============================================================================

// Config Provider 通用配置
type Config struct {
	BaseURL string
	APIKey  string
}

// Usage Token 用量统计
type Usage struct {
	CachedTokens int
	InputTokens  int
	OutputTokens int
}

func (u Usage) TotalTokens() int {
	return u.CachedTokens + u.InputTokens + u.OutputTokens
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
type Provider interface {
	// SyncModels 从上游同步模型列表
	SyncModels(providerID uint) ([]ProviderModel, error)

	// HandleNative 处理本协议原生请求（同协议直通，只替换模型名）
	HandleNative(ctx *gin.Context, modelID string, usage *Usage) error

	// FromOpenAI 处理来自 OpenAI 入口的请求（OpenAI → 本协议，转换后发上游）
	FromOpenAI(ctx *gin.Context, modelID string, usage *Usage) error
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

	// ── Key 格式 ──
	KeyPrefix  string                  `json:"key_prefix"`  // "sk-", "sk-ant-", "AIza"
	KeyLength  int                     `json:"key_length"`  // 24, 24, 26
	KeyEncoder func([]byte) string     `json:"-"`           // hex / base64

	// ── 认证 ──
	AuthExtractor func(c *gin.Context) string `json:"-"`

	// ── Provider 工厂 ──
	NewProvider func(cfg *Config) Provider `json:"-"`

	// ── 测试 ──
	TestExecutor TestExecutor `json:"-"`

	// ── 路由注册（可选，默认用通用路由） ──
	RegisterRoutes func(engine *gin.Engine, auth gin.HandlerFunc) `json:"-"`

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
