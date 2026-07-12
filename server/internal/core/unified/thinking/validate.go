package thinking

// ValidationResult 验证/转换结果。
type ValidationResult struct {
	// Config 应用验证后的 ThinkingConfig（可能被自动转换或修正）
	Config ThinkingConfig
	// Advice 建议日志信息（如跨等级互转时的说明）
	Advice string
	// Clamped 是否做了修正（如预算被限制到范围）
	Clamped bool
}

// ValidateAndConvert 校验 ThinkingConfig 并自动进行 Level↔Budget 转换。
//
// 管道逻辑：
//  1. 如果 Config.Mode == ModeBudget 但 cap.SupportsLevels=true → 转换为 Level
//  2. 如果 Config.Mode == ModeLevel 但 cap.SupportsLevels=false（Budget 模型）→ 转换为 Budget
//  3. Clamp Budget 到 [Min, Max]
//  4. 校验 Level 是否在 SupportedLevels 内
func ValidateAndConvert(tc ThinkingConfig, cap *ModelThinkingCap) ValidationResult {
	result := ValidationResult{Config: tc}

	// 无能力声明 → 透传不校验
	if cap == nil {
		return result
	}

	if !cap.SupportsThinking {
		return ValidationResult{
			Config:  ThinkingConfig{Mode: ModeNone, Budget: 0},
			Advice:  "model does not support thinking; forced ModeNone",
			Clamped: true,
		}
	}

	// 自动转换 Budget ↔ Level
	switch tc.Mode {

	case ModeBudget:
		if cap.SupportsLevels && len(cap.SupportedLevels) > 0 {
			// Budget → Level 转换，优先保持用户的预算意图
			level := BudgetToLevel(tc.Budget)
			if level != "" && ValidateLevel(level, cap) {
				result.Config = ThinkingConfig{Mode: ModeLevel, Level: level}
				result.Advice = "converted budget " + itoa(tc.Budget) + " → level " + "\"" + level + "\""
			} else {
				// 预算无法映射到等级，使用最低等级
				fallback := cap.SupportedLevels[0]
				result.Config = ThinkingConfig{Mode: ModeLevel, Level: fallback}
				result.Advice = "unable to convert budget " + itoa(tc.Budget) + ", fallback to level " + "\"" + fallback + "\""
				result.Clamped = true
			}
		} else {
			// Budget 模式不变，但 clamp 范围
			clamped := ClampBudget(tc.Budget, cap)
			if clamped != tc.Budget {
				result.Config = ThinkingConfig{Mode: ModeBudget, Budget: clamped}
				result.Advice = "clamped budget " + itoa(tc.Budget) + " → " + itoa(clamped)
				result.Clamped = true
			}
		}

	case ModeLevel:
		if !cap.SupportsLevels {
			// Level → Budget 转换
			budget := LevelToBudget(tc.Level)
			if budget > 0 {
				clamped := ClampBudget(budget, cap)
				result.Config = ThinkingConfig{Mode: ModeBudget, Budget: clamped}
				result.Advice = "converted level " + "\"" + tc.Level + "\"" + " → budget " + itoa(clamped)
				if clamped != budget {
					result.Clamped = true
				}
			} else {
				// 未知 level，使用最小预算
				result.Config = ThinkingConfig{Mode: ModeBudget, Budget: cap.MinBudget}
				result.Advice = "unknown level " + "\"" + tc.Level + "\"" + ", fallback to min budget " + itoa(cap.MinBudget)
				result.Clamped = true
			}
		} else {
			// Level 模式不变，校验等级
			if !ValidateLevel(tc.Level, cap) {
				// 尝试 BudgetToLevel 反向查找
				fallback := cap.SupportedLevels[0]
				result.Config = ThinkingConfig{Mode: ModeLevel, Level: fallback}
				result.Advice = "level " + "\"" + tc.Level + "\"" + " not supported, fallback to " + "\"" + fallback + "\""
				result.Clamped = true
			}
		}

	case ModeNone:
		result.Config = ThinkingConfig{Mode: ModeNone, Budget: 0}

	case ModeAuto:
		// Auto → 不做转换，让上游自己决定
	}

	return result
}

// itoa 快速整数转字符串（依赖 fmt 太重量级，直接 hand-roll）
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	// 16 位够用（最大 budget ~ 8 digits）
	var buf [16]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
