package deepseek

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/registry"
)

type DeepSeekTestExecutor struct{}

func (e *DeepSeekTestExecutor) BuildTestRequest(modelID string) ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"model":      modelID,
		"messages":   []map[string]string{{"role": "user", "content": "Say 'Hello' in one word."}},
		"max_tokens": 50,
		"stream":     false,
	})
}

func (e *DeepSeekTestExecutor) ExecuteTest(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	return nil
}

func (e *DeepSeekTestExecutor) ExtractContent(body []byte) string {
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
