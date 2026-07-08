package toolnamesafe

import (
	"strings"
	"testing"
)

func TestSanitizeAnthropicToolName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"valid_name", "valid_name"},
		{"Valid-Name_123", "Valid-Name_123"},
		{"", "tool"},
		{"有中文name", "name"},
		{"name with spaces", "name_with_spaces"},
		// 不合法字符被替换后产生连续下划线需要去重
		{"name.with.dots", "name_with_dots"},
		{"name/with/slashes", "name_with_slashes"},
		{"name...test", "name_test"},
		{"//////", "tool"},
		// 64 字符边界：合法名直接返回
		{strings.Repeat("a", 64), strings.Repeat("a", 64)},
		// 超长合法名截断
		{strings.Repeat("a", 100), strings.Repeat("a", 64)},
	}

	for _, tc := range tests {
		got := SanitizeAnthropicToolName(tc.input)
		if got != tc.want {
			t.Errorf("SanitizeAnthropicToolName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
