package protocols

import (
	"encoding/json"
	"time"

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
	RawResponse  string `json:"raw_response"` // 上游原始响应体（用于调试）
	Error        string `json:"error"`
}

// RunTest 使用 Unified 管道执行模型测试。
// protocol: 目标测试协议 ("openai" / "anthropic" / "gemini")
// baseURL: 该协议对应的端点
// apiKey: 供应商的 API 密钥
// modelID: 测试的目标模型 ID
func RunTest(protocol, baseURL, apiKey, modelID string, maxTokens int) TestResult {
	bodyBytes := buildTestBody(protocol, modelID, maxTokens)

	start := time.Now()

	// 1. 入口协议 ToUnified
	entryDesc, ok := registry.Get(protocol)
	if !ok {
		return TestResult{Error: "unsupported protocol: " + protocol, CallMethod: "direct"}
	}
	entryProv := entryDesc.NewProvider(&registry.Config{})
	req, err := entryProv.ToUnified(bodyBytes, modelID)
	if err != nil {
		return TestResult{Success: false, Error: "to_unified: " + err.Error(), CallMethod: "direct", LatencyMs: time.Since(start).Milliseconds()}
	}
	req.Stream = false

	// 2. 上游 FromUnified
	upProv := entryDesc.NewProvider(&registry.Config{BaseURL: baseURL, APIKey: apiKey})
	resp, _, err := upProv.FromUnified(req)
	// 捕获原始响应体（如果 Provider 实现了 RawResponseCapturer）
	var rawBody string
	if rc, ok := upProv.(registry.RawResponseCapturer); ok {
		rawBody = string(rc.LastRawResponse())
	}
	if err != nil {
		latency := time.Since(start).Milliseconds()
		if httpErr, ok := err.(*registry.HTTPError); ok {
			return TestResult{Success: false, Error: string(httpErr.Body), CallMethod: "direct", LatencyMs: latency}
		}
		return TestResult{Success: false, Error: err.Error(), CallMethod: "direct", LatencyMs: latency}
	}

	latency := time.Since(start).Milliseconds()

	content := resp.Content
	usage := registry.Usage{
		InputTokens:  resp.Usage.InputTokens,
		OutputTokens: resp.Usage.OutputTokens,
	}

	return TestResult{
		Success:      true,
		CallMethod:   "direct",
		LatencyMs:    latency,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		Response:     content,
		RawResponse:  rawBody,
	}
}

// RunTestWithRetry wraps RunTest using the pipeline.RetryableProvider
// for automatic retry on 5xx/429 errors. See internal/protocols/pipeline.

func buildTestBody(protocol, modelID string, maxTokens int) []byte {
	var body map[string]interface{}
	if protocol == "gemini" {
		body = map[string]interface{}{
			"contents": []map[string]interface{}{
				{"parts": []map[string]interface{}{{"text": "Say 'hello' in one word."}}},
			},
			"generationConfig": map[string]interface{}{
				"maxOutputTokens": maxTokens,
			},
		}
	} else {
		body = map[string]interface{}{
			"model":                 modelID,
			"messages":              []map[string]string{{"role": "user", "content": "Say 'hello' in one word."}},
			"max_completion_tokens": maxTokens,
			"stream":                false,
		}
	}
	b, _ := json.Marshal(body)
	return b
}
