// Package model 提供 ID/Slug 双向解析工具函数。
package model

import "strconv"

// ResolveID 解析路由参数为 uint ID。
// 支持 auto-increment ID（"5"）和短 hash slug（"Xa7F2k"）。
// 先尝试解析为 uint，失败则按 slug 查表。
func ResolveID(param, slugColumn, table string) (uint, bool) {
	if param == "" {
		return 0, false
	}

	// 1. 尝试解析为 uint
	if id, err := strconv.ParseUint(param, 10, 32); err == nil {
		// 验证 ID 确实存在
		var count int64
		DB.Table(table).Where("id = ?", id).Count(&count)
		if count > 0 {
			return uint(id), true
		}
		return 0, false
	}

	// 2. 按 slug 查找
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
