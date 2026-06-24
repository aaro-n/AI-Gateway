package gemini

import (
	"encoding/json"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/registry"
)

type GeminiTestExecutor struct{}

func (e *GeminiTestExecutor) BuildTestRequest(modelID string) ([]byte, error) {
	// Gemini 原生测试格式
	return json.Marshal(map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"role": "user",
				"parts": []map[string]interface{}{
					{"text": "Say 'Hello' in one word."},
				},
			},
		},
		"generationConfig": map[string]interface{}{
			"maxOutputTokens": 50,
		},
	})
}

func (e *GeminiTestExecutor) ExecuteTest(ctx *gin.Context, modelID string, usage *registry.Usage) error {
	return nil
}

func (e *GeminiTestExecutor) ExtractContent(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		s := resp.Candidates[0].Content.Parts[0].Text
		if len(s) > 200 {
			return s[:200] + "..."
		}
		return s
	}
	return ""
}
