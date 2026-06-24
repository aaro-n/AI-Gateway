package openai

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/registry"
)

type OpenAITestExecutor struct{}

func (e *OpenAITestExecutor) BuildTestRequest(modelID string) ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"model":      modelID,
		"messages":   []map[string]string{{"role": "user", "content": "Say 'Hello' in one word."}},
		"max_tokens": 50,
		"stream":     false,
	})
}

func (e *OpenAITestExecutor) ExecuteTest(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	// 注：调用者已经构造好 ctx.Request.Body
	// 这里需要在 provider 实例上执行
	// 实际由外层 core/handler 处理 Provider 实例的创建和调用
	// 这里仅做接口占位，实际测试逻辑在 core/handler/model_testing.go 中
	return nil
}

func (e *OpenAITestExecutor) ExtractContent(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	if len(resp.Choices) > 0 {
		return trimContent(resp.Choices[0].Message.Content)
	}
	return ""
}

func trimContent(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
