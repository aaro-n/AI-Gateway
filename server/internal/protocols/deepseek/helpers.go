package deepseek

import (
	"regexp"
	"strings"
)

var thinkTagPattern = regexp.MustCompile(`(?s)<think>(.*?)</think>`)

// stripThinkTag extracts reasoning text from `<think>...</think>` tags
// and returns the cleaned content without the tag.
func stripThinkTag(content string) (reasoning string, cleanContent string) {
	matches := thinkTagPattern.FindStringSubmatch(content)
	if matches == nil {
		return "", content
	}
	reasoning = strings.TrimSpace(matches[1])
	cleanContent = strings.TrimSpace(thinkTagPattern.ReplaceAllString(content, ""))
	return reasoning, cleanContent
}

// wrapThinkTag wraps reasoning content in `<think>...</think>` and prepends to content.
func wrapThinkTag(reasoning, content string) string {
	if reasoning == "" {
		return content
	}
	return "<think>" + reasoning + "</think>" + "\n" + content
}
