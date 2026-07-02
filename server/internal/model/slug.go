// Package model 提供 ID/Slug 双向解析工具函数。
package model

// ResolveID 通过 slug 查找对应记录的 ID。
// 仅接受 slug 字符串（如 "Xa7F2k"），纯数字 ID 将被拒绝。
func ResolveID(param, slugColumn, table string) (uint, bool) {
	if param == "" {
		return 0, false
	}
	if slugColumn == "" {
		slugColumn = "slug"
	}
	var result struct {
		ID uint
	}
	if err := DB.Table(table).Select("id").Where(slugColumn+" = ?", param).First(&result).Error; err != nil {
		return 0, false
	}
	return result.ID, true
}

// ResolveKeyID 快捷方法
func ResolveKeyID(param string) (uint, bool) {
	return ResolveID(param, "slug", "keys")
}

// ResolveProviderID 快捷方法
func ResolveProviderID(param string) (uint, bool) {
	return ResolveID(param, "slug", "providers")
}

// ResolveModelID 快捷方法
func ResolveModelID(param string) (uint, bool) {
	return ResolveID(param, "slug", "models")
}

// ResolveMCPID 快捷方法
func ResolveMCPID(param string) (uint, bool) {
	return ResolveID(param, "slug", "mcps")
}
