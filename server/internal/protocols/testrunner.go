package protocols

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/core/registry"
)

// TestResult 模型测试结果
type TestResult struct {
	Success      bool   `json:"success"`
	CallMethod   string `json:"call_method"`
	LatencyMs    int64  `json:"latency_ms"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Response     string `json:"response"`
	Error        string `json:"error"`
}

// RunTest 使用 Unified 管道执行模型测试。
// protocol: 目标测试协议 ("openai" / "anthropic" / "gemini")
// openaiURL/anthropicURL/geminiURL: provider 已配置的各协议端点
// apiKey: provider 的 API 密钥
// modelID: 测试的目标模型 ID
func RunTest(protocol, openaiURL, anthropicURL, geminiURL, deepseekURL, apiKey, modelID string) TestResult {
	// 构建测试请求体
	bodyBytes := buildTestBody(protocol, modelID)

	// 选择上游协议：优先同协议直通，否则任选一个
	upProto := pickUpstream(protocol, openaiURL, anthropicURL, geminiURL, deepseekURL)
	callMethod := "direct"
	if upProto != protocol {
		callMethod = "convert"
	}

	w := httptest.NewRecorder()
	testCtx, _ := gin.CreateTestContext(w)
	testCtx.Request = httptest.NewRequest("POST", "/", bytes.NewReader(bodyBytes))
	testCtx.Request.Header.Set("Content-Type", "application/json")

	start := time.Now()

	// 1. 入口协议 ToUnified
	entryDesc, ok := registry.Get(protocol)
	if !ok {
		return TestResult{Error: "unsupported protocol: " + protocol, CallMethod: callMethod}
	}
	entryProv := entryDesc.NewProvider(&registry.Config{})
	req, err := entryProv.ToUnified(bodyBytes, modelID)
	if err != nil {
		return TestResult{Success: false, Error: "to_unified: " + err.Error(), CallMethod: callMethod, LatencyMs: time.Since(start).Milliseconds()}
	}
	req.Stream = false

	// 2. 上游 FromUnified
	upDesc, ok := registry.Get(upProto)
	if !ok {
		return TestResult{Error: "unsupported upstream: " + upProto, CallMethod: callMethod, LatencyMs: time.Since(start).Milliseconds()}
	}
	baseURL := pickBaseURL(upProto, openaiURL, anthropicURL, geminiURL, deepseekURL)
	upProv := upDesc.NewProvider(&registry.Config{BaseURL: baseURL, APIKey: apiKey})

	resp, _, err := upProv.FromUnified(req)
	if err != nil {
		latency := time.Since(start).Milliseconds()
		if httpErr, ok := err.(*registry.HTTPError); ok {
			return TestResult{Success: false, Error: string(httpErr.Body), CallMethod: callMethod, LatencyMs: latency}
		}
		return TestResult{Success: false, Error: err.Error(), CallMethod: callMethod, LatencyMs: latency}
	}

	latency := time.Since(start).Milliseconds()

	// 3. 提取响应
	content := resp.Content
	usage := registry.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}

	return TestResult{
		Success:      true,
		CallMethod:   callMethod,
		LatencyMs:    latency,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		Response:     content,
	}
}

func buildTestBody(protocol, modelID string) []byte {
	var body map[string]interface{}
	if protocol == "gemini" {
		body = map[string]interface{}{
			"contents": []map[string]interface{}{
				{"parts": []map[string]interface{}{{"text": "ping"}}},
			},
			"generationConfig": map[string]interface{}{"maxOutputTokens": 1},
		}
	} else {
		body = map[string]interface{}{
			"model":      modelID,
			"messages":   []map[string]string{{"role": "user", "content": "简短介绍一下自己。"}},
			"max_tokens": 1,
			"stream":     false,
		}
	}
	b, _ := json.Marshal(body)
	return b
}

func pickUpstream(target string, openaiURL, anthropicURL, geminiURL, deepseekURL string) string {
	if target == "openai" && openaiURL != "" {
		return "openai"
	}
	if target == "anthropic" && anthropicURL != "" {
		return "anthropic"
	}
	if target == "gemini" && geminiURL != "" {
		return "gemini"
	}
	if target == "deepseek" && deepseekURL != "" {
		return "deepseek"
	}
	// 回退到第一个可用端点
	if openaiURL != "" {
		return "openai"
	}
	if anthropicURL != "" {
		return "anthropic"
	}
	if geminiURL != "" {
		return "gemini"
	}
	if deepseekURL != "" {
		return "deepseek"
	}
	return "openai"
}

func pickBaseURL(protocol, openaiURL, anthropicURL, geminiURL, deepseekURL string) string {
	switch protocol {
	case "openai":
		return openaiURL
	case "anthropic":
		return anthropicURL
	case "gemini":
		return geminiURL
	case "deepseek":
		return deepseekURL
	}
	return ""
}

// ExtractResponseContent 从原始响应体中提取文本内容
func ExtractResponseContent(body []byte, protocol string) string {
	if protocol == "gemini" {
		var resp struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if json.Unmarshal(body, &resp) == nil && len(resp.Candidates) > 0 {
			var text string
			for _, p := range resp.Candidates[0].Content.Parts {
				text += p.Text
			}
			return text
		}
		return string(body)
	}
	if protocol == "anthropic" {
		var resp struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(body, &resp) == nil {
			var text string
			for _, c := range resp.Content {
				if c.Type == "text" {
					text += c.Text
				}
			}
			return text
		}
		return string(body)
	}
	// OpenAI
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(body, &resp) == nil && len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content
	}
	return string(body)
}
