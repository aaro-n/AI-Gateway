// Package conversion 提供跨协议转换损失检测与状态编码。
//
// 当客户端请求使用协议 A 但上游实际使用协议 B 时，本包检测哪些字段/能力会丢失，
// 并将损失模式编码为 uint64 位掩码 (conv_status)，用于日志统计、告警和 X-Gateway-Warnings 响应头。
package conversion

import "strings"

// ConvStatus 位掩码 — 跨协议转换损失类型。
// 使用 uint64 编码，可通过 | 组合多种损失。
const (
	StatusOK                   uint64 = 0
	StatusFieldDropped         uint64 = 1 << iota // 部分请求字段被丢弃
	StatusFieldDegraded                           // 部分字段降级（精度/范围损失）
	StatusToolsLost                               // 工具调用/function calling 丢失
	StatusVisionLost                              // 多模态视觉输入丢失
	StatusThinkingLost                            // 思考/推理 (thinking/reasoning) 丢失
	StatusStreamLost                              // 流式模式丢失
	StatusCacheLost                               // Prompt Caching 丢失
	StatusSystemPromptDegraded                    // 系统提示降级
	StatusJSONModeLost                            // JSON 模式丢失
	StatusLogprobsLost                            // logprobs 丢失
	StatusPenaltyDegraded                         // 频率/存在惩罚降级
)

// statusLabels 状态码 → 人类可读标签
var statusLabels = map[uint64]string{
	StatusFieldDropped:         "field_dropped",
	StatusFieldDegraded:        "field_degraded",
	StatusToolsLost:            "tools_lost",
	StatusVisionLost:           "vision_lost",
	StatusThinkingLost:         "thinking_lost",
	StatusStreamLost:           "stream_lost",
	StatusCacheLost:            "cache_lost",
	StatusSystemPromptDegraded: "system_prompt_degraded",
	StatusJSONModeLost:         "json_mode_lost",
	StatusLogprobsLost:         "logprobs_lost",
	StatusPenaltyDegraded:      "penalty_degraded",
}

// BuildStatusCode 将 ConversionResult 编码为 uint64 位掩码。
func BuildStatusCode(result ConversionResult) uint64 {
	return result.Status
}

// Decode 将位掩码解码为人类可读的状态标签列表。
// 返回空切片表示无损失。
func Decode(status uint64) []string {
	if status == StatusOK {
		return nil
	}
	labels := make([]string, 0)
	for bit, label := range statusLabels {
		if status&bit != 0 {
			labels = append(labels, label)
		}
	}
	return labels
}

// DecodeSummary 将位掩码解码为逗号分隔的摘要字符串。
func DecodeSummary(status uint64) string {
	labels := Decode(status)
	if len(labels) == 0 {
		return "ok"
	}
	return strings.Join(labels, ",")
}
