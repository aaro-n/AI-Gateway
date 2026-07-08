// Package toolnamesafe 清洗 Tool Name 使其符合各协议的命名规范。
// 参考: one-api 的 ToolNameSafe, AxonHub 的 SanitizeToolName。
package toolnamesafe

import (
	"regexp"
	"strings"
)

// AnthropicNamePattern Anthropic tool name 强制正则: ^[a-zA-Z0-9_-]{1,64}$
// 不合规的名字会被 Anthropic API 直接拒绝（400）。
var anthropicNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// invalidChars 匹配不符合 [a-zA-Z0-9_-] 的字符
var invalidChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// SanitizeAnthropicToolName 清洗 tool name 使其符合 Anthropic 规范。
// 不合法字符替换为下划线，截断到 64 字符，空名返回 "tool"。
func SanitizeAnthropicToolName(name string) string {
	if name == "" {
		return "tool"
	}
	// 有效则直接返回
	if anthropicNamePattern.MatchString(name) {
		return name
	}
	// 替换不合法字符
	cleaned := invalidChars.ReplaceAllString(name, "_")
	// 去重连续下划线
	cleaned = collapseUnderscores(cleaned)
	// 截断到 64
	if len(cleaned) > 64 {
		cleaned = cleaned[:64]
	}
	// 去头尾下划线
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		return "tool"
	}
	return cleaned
}

func collapseUnderscores(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevUnderscore := false
	for _, r := range s {
		if r == '_' {
			if !prevUnderscore {
				b.WriteRune('_')
				prevUnderscore = true
			}
		} else {
			b.WriteRune(r)
			prevUnderscore = false
		}
	}
	return b.String()
}
