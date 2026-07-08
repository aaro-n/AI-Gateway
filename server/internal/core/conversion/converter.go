// Package conversion 提供跨协议转换损失检测。
//
// 核心函数 Compare 采用声明式能力差集：将入口协议和上游协议的 CapabilitySet
// 做纯集合差集，自动得出损失的功能列表和对应的 LossBit 位掩码。
// 不依赖请求体的运行时检查，新增协议只需在 capabilities 包中声明能力即可。
package conversion

import (
	"ai-gateway/internal/protocols/capabilities"
)

// ConversionResult 描述一次跨协议转换的损失分析结果。
type ConversionResult struct {
	IsConversion bool     `json:"is_conversion"` // 是否发生了跨协议转换
	FromProtocol string   `json:"from_protocol"` // 客户端协议
	ToProtocol   string   `json:"to_protocol"`   // 上游协议
	Status       uint64   `json:"status"`        // 损失位掩码 (功能丢失标志位的 OR)
	Warnings     []string `json:"warnings"`      // 人类可读的警告列表
	LostFields   []string `json:"lost_fields"`   // 被丢弃的具体字段名
}

// Compare 对比入口协议与上游协议的能力集，返回损失分析（纯声明式差集）。
//
// 参数：
//   - entry: 客户端入口协议的能力定义
//   - upstream: 上游供应商协议的能力定义
//
// 如果 entry == upstream，返回 IsConversion=false, Status=0（同协议无损失）。
func Compare(entry, upstream *capabilities.ProtocolCaps) *ConversionResult {
	result := &ConversionResult{
		FromProtocol: entry.Protocol,
		ToProtocol:   upstream.Protocol,
		IsConversion: entry.Protocol != upstream.Protocol,
	}

	// 同协议无损失
	if entry.Protocol == upstream.Protocol {
		return result
	}

	// 构建上游能力 Key 集合（快速查找）
	upKeys := make(map[string]bool, len(upstream.Capabilities))
	for _, c := range upstream.Capabilities {
		upKeys[c.Key] = true
	}

	var warnings []string
	var lostFields []string
	var lossFlags uint64

	for _, c := range entry.Capabilities {
		if upKeys[c.Key] {
			continue // 上游支持，无损失
		}

		lostFields = append(lostFields, c.Key)

		// 尝试从全局映射获取 LossBit
		bit := LookupLossBit(c.Key)
		if bit != 0 {
			lossFlags |= bit
		}

		// 生成警告
		hint := c.LossHint
		if hint == "" {
			hint = c.Label + " 在上游协议（" + upstream.Label + "）中不受支持"
		}
		warnings = append(warnings, hint)
	}

	result.Status = lossFlags
	result.Warnings = warnings
	result.LostFields = lostFields
	return result
}
