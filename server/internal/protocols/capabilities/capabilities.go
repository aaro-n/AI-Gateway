// Package capabilities 定义了每个 AI 提供商（OpenAI、Anthropic、Gemini）的协议能力特性。
// 用于协议对比（Protocol Comparison）功能 —— 当客户端使用协议 A 但后端模型使用协议 B 时，
// 用户可以清楚看到哪些功能会丢失或受限。
package capabilities

import (
	"fmt"
	"strings"
)

// Capability 代表一项具体的能力/功能
type Capability struct {
	Key         string `json:"key"`         // 唯一标识，如 "streaming", "function_calling"
	Label       string `json:"label"`       // 显示名称
	Description string `json:"description"` // 详细描述
	Category    string `json:"category"`    // 分类：core, input, output, advanced, other
}

// ProtocolCaps 描述一个协议的完整能力集
type ProtocolCaps struct {
	Protocol     string       `json:"protocol"`     // "openai", "anthropic", "gemini"
	Label        string       `json:"label"`        // "OpenAI", "Anthropic", "Google Gemini"
	Description  string       `json:"description"`  // 协议简介
	Capabilities []Capability `json:"capabilities"` // 该协议支持的所有能力
}

// ConversionLoss 描述从协议 A 转换到协议 B 时会丢失什么
type ConversionLoss struct {
	CapabilityKey string `json:"capability_key"`
	LossLevel     string `json:"loss_level"` // "full" (完全丢失), "partial" (部分丢失), "degraded" (降级)
	Note          string `json:"note"`       // 丢失说明
}

// ComparisonResult 协议对比结果
type ComparisonResult struct {
	FromProtocol string           `json:"from_protocol"` // 输入协议
	FromLabel    string           `json:"from_label"`
	ToProtocol   string           `json:"to_protocol"` // 后端协议
	ToLabel      string           `json:"to_label"`
	Losses       []ConversionLoss `json:"losses"`  // 丢失/受限的能力
	Summary      string           `json:"summary"` // 一句话总结
}

// Registry 协议能力注册表
type Registry struct {
	protocols map[string]*ProtocolCaps
}

var globalRegistry = NewRegistry()

func NewRegistry() *Registry {
	r := &Registry{protocols: make(map[string]*ProtocolCaps)}
	r.registerAll()
	return r
}

func (r *Registry) registerAll() {
	r.register(r.openAI())
	r.register(r.anthropic())
	r.register(r.gemini())
	r.register(r.deepseek())
}

func (r *Registry) register(p *ProtocolCaps) {
	r.protocols[p.Protocol] = p
}

// GetAll 返回所有协议的能力定义
func (r *Registry) GetAll() []*ProtocolCaps {
	result := make([]*ProtocolCaps, 0, len(r.protocols))
	// 固定顺序
	for _, k := range []string{"openai", "anthropic", "gemini", "deepseek"} {
		if p, ok := r.protocols[k]; ok {
			result = append(result, p)
		}
	}
	return result
}

// Get 获取单个协议的能力定义
func (r *Registry) Get(protocol string) *ProtocolCaps {
	return r.protocols[protocol]
}

// Global 访问函数
func GetAll() []*ProtocolCaps       { return globalRegistry.GetAll() }
func Get(name string) *ProtocolCaps { return globalRegistry.Get(name) }

// Compare 返回两个协议之间的能力对比
func Compare(from, to string) ComparisonResult {
	fromCaps, toCaps := globalRegistry.Get(from), globalRegistry.Get(to)
	if fromCaps == nil || toCaps == nil {
		label := func(p string) string {
			if p == "openai" {
				return "OpenAI"
			} else if p == "anthropic" {
				return "Anthropic"
			} else if p == "gemini" {
				return "Google Gemini"
			} else if p == "deepseek" {
				return "DeepSeek"
			}
			return p
		}
		return ComparisonResult{
			FromProtocol: from,
			FromLabel:    label(from),
			ToProtocol:   to,
			ToLabel:      label(to),
			Summary:      "未知协议，无法对比",
		}
	}
	result := ComparisonResult{
		FromProtocol: from,
		FromLabel:    fromCaps.Label,
		ToProtocol:   to,
		ToLabel:      toCaps.Label,
	}
	if from == to {
		result.Summary = "相同协议，无功能损失"
		return result
	}
	// 构建目标协议能力集合
	toSet := make(map[string]bool)
	for _, c := range toCaps.Capabilities {
		toSet[c.Key] = true
	}
	// 找出源协议有但目标协议没有的能力
	for _, c := range fromCaps.Capabilities {
		if !toSet[c.Key] {
			loss := ConversionLoss{
				CapabilityKey: c.Key,
				LossLevel:     "full",
				Note:          c.Label + " — " + c.Description,
			}
			// 检查是否有部分支持
			if partialNote := checkPartialSupport(c.Key, to); partialNote != "" {
				loss.LossLevel = "partial"
				loss.Note = partialNote
			}
			result.Losses = append(result.Losses, loss)
		}
	}
	if len(result.Losses) == 0 {
		result.Summary = "功能完全兼容，无损失"
	} else {
		result.Summary = buildSummary(result.Losses)
	}
	return result
}

// CompareAll 返回所有协议间的两两对比
func CompareAll() []ComparisonResult {
	protocols := globalRegistry.GetAll()
	var results []ComparisonResult
	for _, from := range protocols {
		for _, to := range protocols {
			if from.Protocol == to.Protocol {
				continue
			}
			results = append(results, Compare(from.Protocol, to.Protocol))
		}
	}
	return results
}

func checkPartialSupport(key, toProtocol string) string {
	partials := map[string]map[string]string{
		"streaming": {
			// All three support streaming
		},
		"function_calling": {
			"anthropic": "Anthropic 通过 tool_use 支持工具调用，但与 OpenAI function calling 语法不同，转换时会做格式适配",
			"gemini":    "Gemini 通过 functionDeclarations 支持函数调用，转换时会做格式适配",
		},
		"vision": {
			"gemini": "Gemini 原生支持多模态视觉输入，但 OpenAI 格式转换时图片格式和尺寸可能受限",
		},
		"system_prompt": {
			"anthropic": "Anthropic 使用 system 字段而非 messages[0].role=system，转换时会做适配",
			"gemini":    "Gemini 通过 system_instruction 支持系统提示，放置在请求顶层而非消息数组",
		},
		"thinking": {
			"openai": "OpenAI o1 系列支持 reasoning_effort，但与 Anthropic extended thinking 机制不同，可能丢失思考深度控制",
		},
		"json_mode": {
			"anthropic": "Anthropic 最新协议已原生支持 JSON 模式或通过强制 tool 工具调用的方式输出结构化 JSON，旧版客户端转换时会降级",
		},
		"temperature_control": {
			// Both OpenAI, Anthropic, Gemini, and DeepSeek support temperature
		},
		"top_p": {
			// Both OpenAI, Anthropic, Gemini, and DeepSeek support top_p
		},
		"logprobs": {
			"anthropic": "Anthropic 不支持 logprobs（对数概率）输出",
			"gemini":    "Gemini 不支持 logprobs（对数概率）输出",
		},
		"seed": {
			"anthropic": "Anthropic 不支持 seed 参数（确定性输出）",
			"deepseek":  "DeepSeek 不支持 seed 参数（确定性输出）",
		},
		"stop_sequences": {
			// Custom stop sequences - Anthropic/Gemini support differently
		},
		"frequency_penalty": {
			"anthropic": "Anthropic 不支持 frequency_penalty",
			"gemini":    "Gemini 不支持 frequency_penalty",
		},
		"presence_penalty": {
			"anthropic": "Anthropic 不支持 presence_penalty",
			"gemini":    "Gemini 不支持 presence_penalty",
		},
		"n_choices": {
			"anthropic": "Anthropic 不支持多候选回复（n 参数），每次调用仅能产生一个候选回答",
			"deepseek":  "DeepSeek 不支持多候选回复（n 参数），每次调用仅能产生一个候选回答",
		},
		"audio_input": {
			"anthropic": "Anthropic 暂不支持音频输入，转换时音频输入会被忽略或报错",
			"deepseek":  "DeepSeek 暂不支持音频输入，转换时音频输入会被忽略或报错",
		},
		"video_input": {
			"openai":    "OpenAI 目前不直接支持原生视频文件流输入，只能通过按帧提取图片转换输入",
			"anthropic": "Anthropic 目前不直接支持原生视频文件流输入，只能通过按帧提取图片转换输入",
			"deepseek":  "DeepSeek 暂不支持视频输入，转换时视频会被忽略或报错",
		},
		"predicted_outputs": {
			"anthropic": "Anthropic 不支持预测输出加速 (prediction 参数)",
			"gemini":    "Gemini 不支持预测输出加速 (prediction 参数)",
			"deepseek":  "DeepSeek 不支持预测输出加速 (prediction 参数)",
		},
	}
	if m, ok := partials[key]; ok {
		if note, ok2 := m[toProtocol]; ok2 {
			return note
		}
	}
	return ""
}

func buildSummary(losses []ConversionLoss) string {
	fullCount := 0
	partialCount := 0
	for _, l := range losses {
		switch l.LossLevel {
		case "full":
			fullCount++
		case "partial":
			partialCount++
		}
	}
	parts := make([]string, 0, 2)
	if fullCount > 0 {
		parts = append(parts, fmt.Sprintf("%d 项功能完全丢失", fullCount))
	}
	if partialCount > 0 {
		parts = append(parts, fmt.Sprintf("%d 项功能部分受限", partialCount))
	}
	return strings.Join(parts, "，")
}
