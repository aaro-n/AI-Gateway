package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"ai-gateway/internal/core/registry"
	"ai-gateway/internal/core/schemaclean"
	"ai-gateway/internal/core/unified"
)

// FromUnified 将 UnifiedRequest 转为 Gemini 请求并发送到上游
func (p *GeminiProvider) FromUnified(req *unified.Request) (*unified.Response, <-chan unified.StreamEvent, error) {
	geminiReq := p.unifiedToGemini(req)
	body, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, nil, err
	}

	method := "generateContent"
	if req.Stream {
		method = "streamGenerateContent"
	}
	url := fmt.Sprintf("%s/models/%s:%s?key=%s", p.cfg.BaseURL, req.Model, method, p.cfg.APIKey)
	if req.Stream {
		url += "&alt=sse"
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpPool.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, nil, &registry.HTTPError{StatusCode: resp.StatusCode, Body: respBody}
	}

	if req.Stream {
		events := p.streamGeminiToUnified(resp.Body)
		return nil, events, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	uresp, err := p.parseGeminiResponse(respBody)
	return uresp, nil, err
}

// unifiedToGemini 将 UnifiedRequest 转为 Gemini 原生请求体
func (p *GeminiProvider) unifiedToGemini(req *unified.Request) map[string]interface{} {
	contents := make([]map[string]interface{}, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			continue
		}
		geminiRole := "user"
		if m.Role == "assistant" {
			geminiRole = "model"
		}
		parts := p.unifiedContentToGeminiParts(m)
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var args interface{}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = tc.Function.Arguments
				}
				parts = append(parts, map[string]interface{}{
					"functionCall": map[string]interface{}{"name": tc.Function.Name, "args": args},
				})
			}
		}
		if m.Role == "tool" {
			var resp interface{}
			if err := json.Unmarshal(m.Content, &resp); err != nil {
				resp = unified.ContentString(m.Content)
			}
			parts = append(parts, map[string]interface{}{
				"functionResponse": map[string]interface{}{"name": m.Name, "response": resp},
			})
			geminiRole = "user"
		}
		contents = append(contents, map[string]interface{}{"role": geminiRole, "parts": parts})
	}

	result := map[string]interface{}{"contents": contents}
	if req.SystemPrompt != "" {
		result["systemInstruction"] = map[string]interface{}{
			"parts": []map[string]interface{}{{"text": req.SystemPrompt}},
		}
	}

	genConfig := map[string]interface{}{}
	if req.Temperature != nil {
		genConfig["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		genConfig["topP"] = *req.TopP
	}
	if req.TopK != nil {
		genConfig["topK"] = *req.TopK
	}
	if req.MaxTokens > 0 {
		genConfig["maxOutputTokens"] = req.MaxTokens
	}
	if len(req.Stop) > 0 {
		genConfig["stopSequences"] = req.Stop
	}
	result["generationConfig"] = genConfig

	if len(req.Tools) > 0 {
		var unifiedTools []unified.Tool
		if err := json.Unmarshal(req.Tools, &unifiedTools); err == nil {
			functionDeclarations := make([]map[string]interface{}, 0, len(unifiedTools))
			for _, t := range unifiedTools {
				if t.Function.Name != "" {
					cleanParams := json.RawMessage(schemaclean.ForGemini(t.Function.Parameters))
					fd := map[string]interface{}{
						"name":        t.Function.Name,
						"description": t.Function.Description,
					}
					if len(cleanParams) > 0 && string(cleanParams) != "null" {
						var paramsObj interface{}
						if json.Unmarshal(cleanParams, &paramsObj) == nil {
							fd["parameters"] = paramsObj
						}
					}
					functionDeclarations = append(functionDeclarations, fd)
				}
			}
			if len(functionDeclarations) > 0 {
				result["tools"] = []map[string]interface{}{{"functionDeclarations": functionDeclarations}}
			}
		}
	}
	if len(req.ToolChoice) > 0 {
		tc := string(req.ToolChoice)
		var mode string
		switch tc {
		case `"none"`, "none":
			mode = "NONE"
		case `"auto"`, "auto":
			mode = "AUTO"
		case `"required"`, "required":
			mode = "ANY"
		default:
			var tcMap map[string]interface{}
			if json.Unmarshal(req.ToolChoice, &tcMap) == nil {
				result["toolConfig"] = tcMap
			}
		}
		if mode != "" {
			result["toolConfig"] = map[string]interface{}{"functionCallingConfig": map[string]interface{}{"mode": mode}}
		}
	}
	return result
}

func (p *GeminiProvider) unifiedContentToGeminiParts(m unified.Message) []map[string]interface{} {
	parts := make([]map[string]interface{}, 0)
	if s := unified.ContentString(m.Content); s != "" {
		parts = append(parts, map[string]interface{}{"text": s})
		return parts
	}
	for _, b := range unified.ContentBlocks(m.Content) {
		switch b.Type {
		case "text":
			parts = append(parts, map[string]interface{}{"text": b.Text})
		case "image_url":
			if b.ImageURL != nil {
				url := b.ImageURL.URL
				if strings.HasPrefix(url, "data:") {
					mediaType, data := parseDataURL(url)
					parts = append(parts, map[string]interface{}{
						"inlineData": map[string]interface{}{"mimeType": mediaType, "data": data},
					})
				}
			}
		}
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]interface{}{"text": ""})
	}
	return parts
}

func parseDataURL(url string) (mediaType, data string) {
	if idx := strings.Index(url, ";"); idx > 0 {
		mediaType = strings.TrimPrefix(url[:idx], "data:")
		if comma := strings.Index(url, ","); comma > 0 {
			data = url[comma+1:]
		}
	}
	return
}
