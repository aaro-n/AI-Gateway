// Package conversion 提供跨协议转换损失检测与状态编码。
//
// 当客户端请求使用协议 A 但上游实际使用协议 B 时，本包检测哪些字段/能力会丢失，
// 并将损失模式编码为 uint64 位掩码 (conv_status)，用于日志统计、告警和 X-Gateway-Warnings 响应头。
//
// conv_status 编码格式（64-bit）：
//
//	┌──────────┬──────────┬──────┬───────┬──────────────────────────────┐
//	│ 63 ... 56 │ 55 ... 48 │ 47  │ 46-32 │ 31 ... 0                      │
//	│ 入口协议  │ 上游协议  │ 直通 │ 保留   │ 功能丢失标志位                  │
//	│  (8bit)   │  (8bit)   │ 1bit │ 15bit  │ (32bit bitmap)                │
//	└──────────┴──────────┴──────┴───────┴──────────────────────────────┘
package conversion

import "strings"

// =============================================================================
// 协议 ID 编码（8 bits，位于 conv_status 高位）
// =============================================================================

const (
	ProtoOpenAI     uint64 = 0x01
	ProtoAnthropic  uint64 = 0x02
	ProtoGemini     uint64 = 0x03
	ProtoDeepSeek   uint64 = 0x04
	ProtoOpenRouter uint64 = 0x05
)

// =============================================================================
// 功能丢失标志位（32 bits，bit=1 表示丢失/降级）
// =============================================================================

const (
	LossFrequencyPenalty   uint64 = 1 << 31 // 频率惩罚被丢弃
	LossPresencePenalty    uint64 = 1 << 30 // 存在惩罚被丢弃
	LossSeed               uint64 = 1 << 29 // 确定性种子被丢弃
	LossReasoningEffort    uint64 = 1 << 28 // 推理深度被丢弃
	LossModalities         uint64 = 1 << 27 // 输出模态被丢弃
	LossTopK               uint64 = 1 << 26 // Top-K 采样被丢弃
	LossThinking           uint64 = 1 << 25 // 扩展思考被丢弃
	LossSafetySettings     uint64 = 1 << 24 // 安全设置被丢弃
	LossCodeExecution      uint64 = 1 << 23 // 代码执行被丢弃
	LossGoogleSearch       uint64 = 1 << 22 // Google 搜索被丢弃
	LossContextCaching     uint64 = 1 << 21 // 上下文缓存被丢弃
	LossComputerUse        uint64 = 1 << 20 // 计算机操作用被丢弃
	LossWebSearch          uint64 = 1 << 19 // 网络搜索被丢弃
	LossPromptCaching      uint64 = 1 << 18 // Prompt 缓存被丢弃
	LossRichStreamEvents   uint64 = 1 << 17 // 富流式事件降级
	LossSystemDegraded     uint64 = 1 << 16 // System 提示词被合并/裁剪
	LossStopReasonMapped   uint64 = 1 << 15 // 停止原因被映射（语义有损）
	LossPDFSupport         uint64 = 1 << 14 // PDF 支持被丢弃
	LossVideoInput         uint64 = 1 << 13 // 视频输入被丢弃
	LossAudioInput         uint64 = 1 << 12 // 音频输入被丢弃
	LossJSONSchemaDegraded uint64 = 1 << 11 // JSON Schema 被裁剪
	LossLogprobs           uint64 = 1 << 10 // Logprobs 被丢弃
	LossPrefixCompletion   uint64 = 1 << 9  // 前缀补全被丢弃
	LossToolChoiceDegraded uint64 = 1 << 8  // Tool Choice 格式降级
)

// StatusOK 表示零损失（同协议直通）。
const StatusOK uint64 = 0

// =============================================================================
// 协议名 ↔ ID 互转
// =============================================================================

// ProtocolName 将协议 ID 映射为名称。
func ProtocolName(id uint64) string {
	switch id {
	case ProtoOpenAI:
		return "openai"
	case ProtoAnthropic:
		return "anthropic"
	case ProtoGemini:
		return "gemini"
	case ProtoDeepSeek:
		return "deepseek"
	case ProtoOpenRouter:
		return "openrouter"
	default:
		return "unknown"
	}
}

// ProtocolID 将协议名称映射为 ID。
func ProtocolID(name string) uint64 {
	switch name {
	case "openai":
		return ProtoOpenAI
	case "anthropic":
		return ProtoAnthropic
	case "gemini":
		return ProtoGemini
	case "deepseek":
		return ProtoDeepSeek
	case "openrouter":
		return ProtoOpenRouter
	default:
		return 0
	}
}

// =============================================================================
// 编码 / 解码
// =============================================================================

// BuildStatusCode 构建 64-bit 转换状态码。
//
// 参数：
//   - entryProto: 客户端入口协议 ID
//   - upProto: 上游供应商协议 ID
//   - lostFeatures: 功能丢失位掩码（Loss* 常量的 OR 组合）
//   - isDirect: 是否为同协议直通
func BuildStatusCode(entryProto, upProto uint64, lostFeatures uint64, isDirect bool) uint64 {
	code := (entryProto << 56) | (upProto << 48)
	if isDirect {
		code |= 1 << 47
	}
	code |= (lostFeatures & 0xFFFFFFFF)
	return code
}

// StatusInfo 解码后的状态信息。
type StatusInfo struct {
	EntryProtocol    string   `json:"entry_protocol"`
	UpstreamProtocol string   `json:"upstream_protocol"`
	IsDirect         bool     `json:"is_direct"`
	LostFeatures     []string `json:"lost_features"`
}

// Decode 解码 conv_status 中的功能丢失标志位，返回人类可读标签列表。
// 返回 nil 表示无损失。
func Decode(status uint64) []string {
	if status == StatusOK {
		return nil
	}
	return decodeFeatureFlags(status & 0xFFFFFFFF)
}

// DecodeFull 完整解码 conv_status，包含协议信息和功能丢失列表。
func DecodeFull(code uint64) StatusInfo {
	return StatusInfo{
		EntryProtocol:    ProtocolName(code >> 56 & 0xFF),
		UpstreamProtocol: ProtocolName(code >> 48 & 0xFF),
		IsDirect:         (code>>47)&1 == 1,
		LostFeatures:     decodeFeatureFlags(code & 0xFFFFFFFF),
	}
}

// DecodeSummary 将 conv_status 解码为逗号分隔的摘要字符串。
func DecodeSummary(status uint64) string {
	labels := Decode(status)
	if len(labels) == 0 {
		return "ok"
	}
	return strings.Join(labels, ",")
}

// =============================================================================
// 内部：功能标志位 → 标签映射 & 能力 Key → LossBit 映射
// =============================================================================

var featureLabels = map[uint64]string{
	LossFrequencyPenalty:   "frequency_penalty",
	LossPresencePenalty:    "presence_penalty",
	LossSeed:               "seed",
	LossReasoningEffort:    "reasoning_effort",
	LossModalities:         "modalities",
	LossTopK:               "top_k",
	LossThinking:           "thinking",
	LossSafetySettings:     "safety_settings",
	LossCodeExecution:      "code_execution",
	LossGoogleSearch:       "google_search",
	LossContextCaching:     "context_caching",
	LossComputerUse:        "computer_use",
	LossWebSearch:          "web_search",
	LossPromptCaching:      "prompt_caching",
	LossRichStreamEvents:   "rich_stream_events",
	LossSystemDegraded:     "system_degraded",
	LossStopReasonMapped:   "stop_reason_mapped",
	LossPDFSupport:         "pdf_support",
	LossVideoInput:         "video_input",
	LossAudioInput:         "audio_input",
	LossJSONSchemaDegraded: "json_schema_degraded",
	LossLogprobs:           "logprobs",
	LossPrefixCompletion:   "prefix_completion",
	LossToolChoiceDegraded: "tool_choice_degraded",
}

// keyToLossBit 将能力 Key 映射到对应的 Loss* 常量。
// 用于 converter.Compare 做声明式差集时自动计算 lossFlags。
var keyToLossBit = map[string]uint64{
	"frequency_penalty": LossFrequencyPenalty,
	"presence_penalty":  LossPresencePenalty,
	"seed":              LossSeed,
	"reasoning_effort":  LossReasoningEffort,
	"modalities":        LossModalities,
	"top_k":             LossTopK,
	"thinking":          LossThinking,
	"safety_settings":   LossSafetySettings,
	"code_execution":    LossCodeExecution,
	"google_search":     LossGoogleSearch,
	"context_caching":   LossContextCaching,
	"computer_use":      LossComputerUse,
	"web_search":        LossWebSearch,
	"prompt_caching":    LossPromptCaching,
	"stream_events":     LossRichStreamEvents,
	"pdf_support":       LossPDFSupport,
	"video_input":       LossVideoInput,
	"audio_input":       LossAudioInput,
	"json_mode":         LossJSONSchemaDegraded,
	"structured_output": LossJSONSchemaDegraded,
	"logprobs":          LossLogprobs,
	"prefix_completion": LossPrefixCompletion,
	"tool_choice":       LossToolChoiceDegraded,
}

// LookupLossBit 查询能力 Key 对应的 LossBit，如果未映射则返回 0。
func LookupLossBit(key string) uint64 {
	return keyToLossBit[key]
}

func decodeFeatureFlags(flags uint64) []string {
	if flags == 0 {
		return nil
	}
	labels := make([]string, 0)
	for bit, label := range featureLabels {
		if flags&bit != 0 {
			labels = append(labels, label)
		}
	}
	return labels
}
