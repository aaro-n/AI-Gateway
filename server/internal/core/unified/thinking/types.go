// Package thinking 提供统一的思考（thinking/reasoning）配置处理管道。
//
// 设计目标：
//   - 从 UnifiedRequest 的 ReasoningEffort / ReasoningBudget 字段构建统一的 ThinkingConfig
//   - 校验配置是否在模型能力范围内
//   - Level ↔ Budget 自动互转（如 "high" → 24576, 16384 → "high"）
//   - 为各协议 FromUnified 提供统一的思考参数注入点
//
// 借鉴 CLIProxyAPI 的 internal/thinking/ 设计，适配 AI-Gateway 的 Hub-and-Spoke 架构。
package thinking

// ThinkingMode 思考配置的类型
type ThinkingMode int

const (
	// ModeNone 禁用思考（budget=0 或无 reasoning_effort）
	ModeNone ThinkingMode = iota
	// ModeAuto 自动/动态思考（对应 reasoning_effort="" 或 budget=-1）
	ModeAuto
	// ModeBudget 使用 Token 预算模式（如 Anthropic budget_tokens、Gemini thinkingBudget）
	ModeBudget
	// ModeLevel 使用离散等级模式（如 OpenAI reasoning_effort: "high"）
	ModeLevel
)

// Predefined thinking levels.
const (
	LevelNone    = "none"
	LevelAuto    = "auto"
	LevelMinimal = "minimal"
	LevelLow     = "low"
	LevelMedium  = "medium"
	LevelHigh    = "high"
	LevelXHigh   = "xhigh"
	LevelMax     = "max"
)

// ThinkingConfig 统一思考配置。
//
// 根据 Mode 不同，有效字段不同：
//   - ModeNone:   Budget=0, Level 忽略
//   - ModeAuto:   Budget=-1, Level 忽略（由上游自动决定）
//   - ModeBudget: Budget>0 为有效值, Level 忽略
//   - ModeLevel:  Budget 忽略, Level 为有效等级
type ThinkingConfig struct {
	Mode   ThinkingMode
	Budget int    // Token 预算（0=禁用, -1=自动, >0=具体值）
	Level  string // 离散等级（low/medium/high/xhigh/max）
}

// ModelThinkingCap 描述模型对 thinking 的支持能力。
type ModelThinkingCap struct {
	SupportsThinking bool     // 是否支持 thinking
	MinBudget        int      // 最小 budget（0 表示禁止 thinking 时可用 0）
	MaxBudget        int      // 最大 budget
	SupportsLevels   bool     // 是否支持离散等级
	SupportedLevels  []string // 支持的等级列表（有序，从低到高）
}

// KnownModelCaps 已知模型的 thinking 能力声明。
//
// 模型 ID 以全小写存储，使用 Contains 前缀匹配以覆盖变体。
// 值在协议插件的 ToUnified 之后由网关注入。
var KnownModelCaps = map[string]ModelThinkingCap{
	// Gemini 2.5 系列 — 使用 thinkingBudget（Budget 模式）
	"gemini-2.5-pro": {
		SupportsThinking: true,
		MinBudget:        1024,
		MaxBudget:        32768,
	},
	"gemini-2.5-flash": {
		SupportsThinking: true,
		MinBudget:        0,
		MaxBudget:        32768,
	},

	// Gemini 3.x 系列 — 使用 thinkingLevel（Level 模式）
	"gemini-3": {
		SupportsThinking: true,
		SupportsLevels:   true,
		SupportedLevels:  []string{LevelLow, LevelMedium, LevelHigh},
	},

	// Claude 4.x 系列 — 使用 budget_tokens（Budget 模式）
	"claude-sonnet-4": {
		SupportsThinking: true,
		MinBudget:        1024,
		MaxBudget:        64000,
	},
	"claude-opus-4": {
		SupportsThinking: true,
		MinBudget:        1024,
		MaxBudget:        128000,
	},

	// Claude 4.6 自适应思维 — 使用 effort（Level 模式）
	"claude-4.6": {
		SupportsThinking: true,
		SupportsLevels:   true,
		SupportedLevels:  []string{LevelLow, LevelMedium, LevelHigh, LevelMax},
	},

	// OpenAI / DeepSeek reasoning 模型 — 使用 reasoning_effort（Level 模式）
	"o1": {
		SupportsThinking: true,
		SupportsLevels:   true,
		SupportedLevels:  []string{LevelLow, LevelMedium, LevelHigh},
	},
	"o3": {
		SupportsThinking: true,
		SupportsLevels:   true,
		SupportedLevels:  []string{LevelLow, LevelMedium, LevelHigh},
	},
	"deepseek-r1": {
		SupportsThinking: true,
		SupportsLevels:   true,
		SupportedLevels:  []string{LevelLow, LevelMedium, LevelHigh},
	},
	"deepseek-reasoner": {
		SupportsThinking: true,
		SupportsLevels:   true,
		SupportedLevels:  []string{LevelLow, LevelMedium, LevelHigh},
	},
}

// LookupCap 根据模型名查找 Thinking 能力声明（用 Contains 前缀匹配）。
// 返回 nil 表示不认识的模型，不强制校验。
func LookupCap(modelID string) *ModelThinkingCap {
	lower := ""
	for i := 0; i < len(modelID); i++ {
		c := modelID[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		lower += string(c)
	}

	for key, cap := range KnownModelCaps {
		kl := len(key)
		if len(lower) >= kl && lower[:kl] == key {
			return &cap
		}
	}
	return nil
}

// FromUnified 从 UnifiedRequest 构建 ThinkingConfig。
func FromUnified(effort string, budget *int) ThinkingConfig {
	// Budget 模式优先
	if budget != nil {
		v := *budget
		switch {
		case v == 0:
			return ThinkingConfig{Mode: ModeNone, Budget: 0}
		case v < 0:
			return ThinkingConfig{Mode: ModeAuto, Budget: -1}
		default:
			return ThinkingConfig{Mode: ModeBudget, Budget: v}
		}
	}

	// Level 模式
	if effort != "" {
		switch effort {
		case "none":
			return ThinkingConfig{Mode: ModeNone, Budget: 0}
		case "auto":
			return ThinkingConfig{Mode: ModeAuto, Budget: -1}
		default:
			// 尝试映射到已知 Level
			level := normalizeLevel(effort)
			if level != "" {
				return ThinkingConfig{Mode: ModeLevel, Level: level}
			}
			// 不认识的 effort，不强制干预
			return ThinkingConfig{Mode: ModeLevel, Level: effort}
		}
	}

	return ThinkingConfig{Mode: ModeAuto}
}

func normalizeLevel(s string) string {
	canonical := map[string]string{
		"none":    LevelNone,
		"auto":    LevelAuto,
		"minimal": LevelMinimal,
		"low":     LevelLow,
		"medium":  LevelMedium,
		"high":    LevelHigh,
		"xhigh":   LevelXHigh,
		"x-high":  LevelXHigh,
		"max":     LevelMax,
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
	}
	// Simple case-insensitive lookup
	lower := ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		lower += string(c)
	}
	if v, ok := canonical[lower]; ok {
		return v
	}
	return ""
}
