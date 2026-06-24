package anthropic

import (
	"encoding/json"

	"ai-gateway/internal/core/registry"

	"github.com/gin-gonic/gin"
)

type AnthropicTestExecutor struct{}

func (e *AnthropicTestExecutor) BuildTestRequest(modelID string) ([]byte, error) {
	// 测试也走 OpenAI 格式，通过 FromOpenAI 转换
	return json.Marshal(map[string]interface{}{
		"model":      modelID,
		"messages":   []map[string]string{{"role": "user", "content": "Say 'Hello' in one word."}},
		"max_tokens": 50,
		"stream":     false,
	})
}

func (e *AnthropicTestExecutor) ExecuteTest(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	return nil
}

func (e *AnthropicTestExecutor) ExtractContent(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	// Anthropic 原始响应格式
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	for _, c := range resp.Content {
		if c.Type == "text" && c.Text != "" {
			return trimStr(c.Text)
		}
	}
	return ""
}

func trimStr(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
