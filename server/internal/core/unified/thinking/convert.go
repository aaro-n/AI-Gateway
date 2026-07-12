package thinking

import "sort"

// levelBudgetPairs 定义 Level → Budget 的映射区间。
// Level 是离散等级，Budget 是对应的 Token 预算范围（含最小值/默认值/最大值）。
type levelBudgetPair struct {
	Level         string
	DefaultBudget int // 该等级的默认 Token 预算
	MinBudget     int // 该等级的最小 Token 预算
}

// 通用 Level↔Budget 映射表。
// 预算值参考主流模型的 thinking budget 范围，确保跨协议兼容。
var levelBudgetTable = []levelBudgetPair{
	{Level: LevelMinimal, DefaultBudget: 1024, MinBudget: 512},
	{Level: LevelLow, DefaultBudget: 4096, MinBudget: 1024},
	{Level: LevelMedium, DefaultBudget: 16384, MinBudget: 4096},
	{Level: LevelHigh, DefaultBudget: 32768, MinBudget: 8192},
	{Level: LevelXHigh, DefaultBudget: 65536, MinBudget: 16384},
	{Level: LevelMax, DefaultBudget: 128000, MinBudget: 32768},
}

func init() {
	// 确保预算表从低到高排序
	sort.Slice(levelBudgetTable, func(i, j int) bool {
		return levelBudgetTable[i].DefaultBudget < levelBudgetTable[j].DefaultBudget
	})
}

// LevelToBudget 将 Level 转换为 Token 预算（默认值）。
// 返回 -1 表示不认识的等级。
func LevelToBudget(level string) int {
	for _, p := range levelBudgetTable {
		if p.Level == level {
			return p.DefaultBudget
		}
	}
	return -1
}

// BudgetToLevel 将 Token 预算转换为最近的 Level。
// 返回空字符串表示预算太小（<512）或不在正常范围。
func BudgetToLevel(budget int) string {
	if budget <= 0 {
		return ""
	}
	for i := len(levelBudgetTable) - 1; i >= 0; i-- {
		if budget >= levelBudgetTable[i].MinBudget {
			return levelBudgetTable[i].Level
		}
	}
	return ""
}

// ClampBudget 将预算限制到模型能力的合理范围。
// minBudget/0 = 不允许自动 → clamp to exact budget（用户必须精确指定）
// minBudget>0 = 有最小要求 → clamp to [min, max]
func ClampBudget(budget int, cap *ModelThinkingCap) int {
	if cap == nil {
		return budget
	}
	if budget <= 0 {
		return budget
	}
	if budget < cap.MinBudget && cap.MinBudget > 0 {
		// 预算小于最小值，提升到最小值（自动模式下不强制）
		return cap.MinBudget
	}
	if budget > cap.MaxBudget {
		return cap.MaxBudget
	}
	return budget
}

// ValidateLevel 校验 Level 值是否在模型支持的范围内。
// 返回 true 表示模型支持该等级（cap==nil 时不校验，返回 true）。
func ValidateLevel(level string, cap *ModelThinkingCap) bool {
	if cap == nil || cap.SupportedLevels == nil {
		return true
	}
	for _, ok := range cap.SupportedLevels {
		if ok == level {
			return true
		}
	}
	return false
}
