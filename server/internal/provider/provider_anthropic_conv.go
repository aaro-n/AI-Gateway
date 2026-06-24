package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ai-gateway/internal/model"
)

func (m *AnthropicProvider) ExecuteOpenAIRequest(c *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	recordBody("O2A", "raw", body)
	var openAIReq struct {
		Model     string                   `json:"model"`
		MaxTokens int                      `json:"max_tokens"`
		Messages  []map[string]interface{} `json:"messages"`
		Tools     json.RawMessage          `json:"tools,omitempty"`
		Stream    bool                     `json:"stream"`
	}
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}
	openAIReq.Model = pm.ModelID

	anthropicReq := m.convertOpenAIRequestToAnthropic(openAIReq)

	anthropicBody, err := json.Marshal(anthropicReq)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	recordBody("O2A", "converted", anthropicBody)
	req, err := http.NewRequest("POST", m.cfg.BaseURL+"/messages", bytes.NewReader(anthropicBody))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	req = req.WithContext(c.Request.Context())
	req.Header.Set("x-api-key", m.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		c.Status(resp.StatusCode)
		c.Header("Content-Type", resp.Header.Get("Content-Type"))
		c.Writer.Write(respBody)
		recordError("O2A", resp.StatusCode, respBody)
		return &ProviderError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	if m.isStreaming(resp) {
		c.Status(http.StatusOK)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		err = m.streamAnthropicToOpenAI(c.Request.Context(), resp.Body, c.Writer, openAIReq.Model, usage)
		c.Writer.Flush()
	} else {
		anthropicRespBody, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return err
		}
		openAIResp, err := m.convertAnthropicResponseToOpenAI(anthropicRespBody, openAIReq.Model, usage)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return err
		}
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		c.Writer.Write(openAIResp)
		err = nil
	}
	return err
}

func (m *AnthropicProvider) convertOpenAIRequestToAnthropic(openAIReq struct {
	Model     string                   `json:"model"`
	MaxTokens int                      `json:"max_tokens"`
	Messages  []map[string]interface{} `json:"messages"`
	Tools     json.RawMessage          `json:"tools,omitempty"`
	Stream    bool                     `json:"stream"`
}) map[string]interface{} {
	var systemContent string
	anthropicMessages := make([]map[string]interface{}, 0)
	var pendingToolResults []map[string]interface{}

	flushPendingToolResults := func() {
		if len(pendingToolResults) == 0 {
			return
		}
		content := make([]map[string]interface{}, 0, len(pendingToolResults))
		for _, tr := range pendingToolResults {
			content = append(content, tr)
		}
		anthropicMessages = append(anthropicMessages, map[string]interface{}{
			"role":    "user",
			"content": content,
		})
		pendingToolResults = pendingToolResults[:0]
	}

	for _, msg := range openAIReq.Messages {
		role, _ := msg["role"].(string)
		switch role {
		case "system":
			systemContent = m.extractSystemContent(msg["content"])
		case "tool":
			pendingToolResults = append(pendingToolResults, m.convertOpenAIToolResultToAnthropic(msg))
		case "assistant":
			flushPendingToolResults()
			anthropicMessages = append(anthropicMessages, m.convertOpenAIMessageToAnthropic(msg))
		default:
			flushPendingToolResults()
			anthropicMessages = append(anthropicMessages, m.convertOpenAIMessageToAnthropic(msg))
		}
	}
	flushPendingToolResults()

	anthropicReq := map[string]interface{}{
		"model":      openAIReq.Model,
		"max_tokens": openAIReq.MaxTokens,
		"messages":   anthropicMessages,
		"stream":     openAIReq.Stream,
	}
	if systemContent != "" {
		anthropicReq["system"] = systemContent
	}
	if openAIReq.Tools != nil {
		var tools []interface{}
		if err := json.Unmarshal(openAIReq.Tools, &tools); err == nil {
			anthropicTools := make([]map[string]interface{}, 0, len(tools))
			for _, tool := range tools {
				if t, ok := tool.(map[string]interface{}); ok {
					anthropicTools = append(anthropicTools, m.convertOpenAIToolToAnthropic(t))
				}
			}
			anthropicReq["tools"] = anthropicTools
		}
	}
	return anthropicReq
}

func (m *AnthropicProvider) extractSystemContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var texts []string
		for _, part := range v {
			if partMap, ok := part.(map[string]interface{}); ok {
				if partType, _ := partMap["type"].(string); partType == "text" {
					if text, _ := partMap["text"].(string); text != "" {
						texts = append(texts, text)
					}
				}
			}
		}
		if len(texts) > 0 {
			return strings.Join(texts, "\n")
		}
	}
	return ""
}

func (m *AnthropicProvider) convertOpenAIMessageToAnthropic(msg map[string]interface{}) map[string]interface{} {
	role, _ := msg["role"].(string)
	content := msg["content"]

	result := map[string]interface{}{
		"role": role,
	}

	switch v := content.(type) {
	case string:
		result["content"] = []map[string]interface{}{
			{"type": "text", "text": v},
		}
	case []interface{}:
		blocks := make([]map[string]interface{}, 0)
		for _, part := range v {
			if partMap, ok := part.(map[string]interface{}); ok {
				partType, _ := partMap["type"].(string)
				switch partType {
				case "text":
					blocks = append(blocks, map[string]interface{}{
						"type": "text",
						"text": partMap["text"],
					})
				case "image_url":
					imageURL, _ := partMap["image_url"].(map[string]interface{})
					if imageURL != nil {
						url, _ := imageURL["url"].(string)
						if strings.HasPrefix(url, "data:") {
							mediaType, data := m.parseDataURL(url)
							blocks = append(blocks, map[string]interface{}{
								"type": "image",
								"source": map[string]interface{}{
									"type":       "base64",
									"media_type": mediaType,
									"data":       data,
								},
							})
						} else {
							blocks = append(blocks, map[string]interface{}{
								"type": "image",
								"source": map[string]interface{}{
									"type": "url",
									"url":  url,
								},
							})
						}
					}
				}
			}
		}
		result["content"] = blocks
	default:
		if v != nil {
			result["content"] = []map[string]interface{}{
				{"type": "text", "text": fmt.Sprintf("%v", v)},
			}
		}
	}

	if toolCalls, ok := msg["tool_calls"].([]interface{}); ok {
		blocks, _ := result["content"].([]map[string]interface{})
		for _, tc := range toolCalls {
			if tcMap, ok := tc.(map[string]interface{}); ok {
				toolUse := map[string]interface{}{
					"type": "tool_use",
					"id":   tcMap["id"],
				}
				if fn, ok := tcMap["function"].(map[string]interface{}); ok {
					toolUse["name"] = fn["name"]
					if args, _ := fn["arguments"].(string); args != "" {
						var input map[string]interface{}
						if json.Unmarshal([]byte(args), &input) == nil {
							toolUse["input"] = input
						} else {
							toolUse["input"] = args
						}
					}
				}
				blocks = append(blocks, toolUse)
			}
		}
		result["content"] = blocks
	}

	return result
}

func (m *AnthropicProvider) convertOpenAIToolResultToAnthropic(msg map[string]interface{}) map[string]interface{} {
	toolCallID, _ := msg["tool_call_id"].(string)
	content := msg["content"]

	var toolResultContent string
	switch v := content.(type) {
	case string:
		toolResultContent = v
	default:
		toolResultContent = fmt.Sprintf("%v", v)
	}

	return map[string]interface{}{
		"type":        "tool_result",
		"tool_use_id": toolCallID,
		"content":     toolResultContent,
	}
}

func (m *AnthropicProvider) convertOpenAIToolToAnthropic(tool map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"name": tool["name"],
	}
	if desc, ok := tool["description"].(string); ok {
		result["description"] = desc
	}
	if fn, ok := tool["function"].(map[string]interface{}); ok {
		if params, ok := fn["parameters"].(map[string]interface{}); ok {
			result["input_schema"] = params
		}
		if desc, ok := fn["description"].(string); ok {
			result["description"] = desc
		}
		if name, ok := fn["name"].(string); ok {
			result["name"] = name
		}
	}
	return result
}

func (m *AnthropicProvider) parseDataURL(url string) (mediaType, data string) {
	if !strings.HasPrefix(url, "data:") {
		return "", ""
	}
	url = strings.TrimPrefix(url, "data:")
	parts := strings.SplitN(url, ",", 2)
	if len(parts) != 2 {
		return "", ""
	}
	mediaType = parts[0]
	if strings.Contains(mediaType, ";") {
		mediaType = strings.Split(mediaType, ";")[0]
	}
	data = parts[1]
	return mediaType, data
}

func (m *AnthropicProvider) convertAnthropicResponseToOpenAI(anthropicResp []byte, model string, usage *Usage) ([]byte, error) {
	var anthropic struct {
		ID         string                   `json:"id"`
		Type       string                   `json:"type"`
		Role       string                   `json:"role"`
		Model      string                   `json:"model"`
		Content    []map[string]interface{} `json:"content"`
		StopReason string                   `json:"stop_reason"`
		Usage      anthropicUsage           `json:"usage"`
	}
	if err := json.Unmarshal(anthropicResp, &anthropic); err != nil {
		return nil, fmt.Errorf("failed to parse Anthropic response: %w", err)
	}

	textContent := ""
	reasoningContent := ""
	var toolCalls []map[string]interface{}

	for _, block := range anthropic.Content {
		blockType, _ := block["type"].(string)
		switch blockType {
		case "text":
			if text, ok := block["text"].(string); ok {
				textContent = text
			}
		case "thinking":
			if thinking, ok := block["thinking"].(string); ok {
				reasoningContent = thinking
			}
		case "tool_use":
			var argsStr string
			if input := block["input"]; input != nil {
				if inputBytes, err := json.Marshal(input); err == nil {
					argsStr = string(inputBytes)
				}
			}
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   block["id"],
				"type": "function",
				"function": map[string]interface{}{
					"name":      block["name"],
					"arguments": argsStr,
				},
			})
		}
	}

	finishReason := "stop"
	switch anthropic.StopReason {
	case "end_turn":
		finishReason = "stop"
	case "max_tokens":
		finishReason = "length"
	case "tool_use":
		finishReason = "tool_calls"
	case "stop_sequence":
		finishReason = "stop"
	}

	message := map[string]interface{}{
		"role":    "assistant",
		"content": textContent,
	}
	if reasoningContent != "" {
		message["reasoning_content"] = reasoningContent
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
	}

	openAIResp := map[string]interface{}{
		"id":      anthropic.ID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     anthropic.Usage.InputTokens,
			"completion_tokens": anthropic.Usage.OutputTokens,
			"total_tokens":      anthropic.Usage.total(),
		},
	}

	result, err := json.Marshal(openAIResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI response: %w", err)
	}
	anthropic.Usage.toUsage(usage)
	return result, nil
}

func (m *AnthropicProvider) streamAnthropicToOpenAI(ctx context.Context, src io.Reader, dst io.Writer, model string, usage *Usage) error {
	src, dst = recordStream("O2A", src, dst)
	reader := bufio.NewReader(src)
	var rError error
	errorCount := 0

	type readResult struct {
		line string
		err  error
	}

streamLoop:
	for {
		readCh := make(chan readResult, 1)

		go func() {
			select {
			case <-ctx.Done():
				readCh <- readResult{err: ctx.Err()}
			default:
				line, err := reader.ReadString('\n')
				readCh <- readResult{line: line, err: err}
			}
		}()

		select {
		case <-ctx.Done():
			return ctx.Err()
		case result := <-readCh:
			if result.err != nil {
				if result.err == io.EOF {
					break streamLoop
				}
				errorCount++
				if errorCount >= 3 {
					rError = fmt.Errorf("Anthropic convert stream error, %v", result.err)
					break streamLoop
				}
				continue
			}

			line := strings.TrimSpace(result.line)
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimSpace(data)

			var event struct {
				Type         string                 `json:"type"`
				Message      map[string]interface{} `json:"message"`
				Index        int                    `json:"index"`
				ContentBlock map[string]interface{} `json:"content_block"`
				Delta        map[string]interface{} `json:"delta"`
				Usage        anthropicUsage         `json:"usage"`
			}

			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			switch event.Type {
			case "message_start":
				// Initialize or record
			case "content_block_start":
				// Handle thinking or text start
			case "content_block_delta":
				deltaType, _ := event.Delta["type"].(string)
				switch deltaType {
				case "text_delta":
					text, _ := event.Delta["text"].(string)
					m.writeOpenAISSE(dst, map[string]interface{}{
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"delta": map[string]interface{}{
									"role":    "assistant",
									"content": text,
								},
							},
						},
					})
				case "thinking_delta":
					thinking, _ := event.Delta["thinking"].(string)
					m.writeOpenAISSE(dst, map[string]interface{}{
						"choices": []map[string]interface{}{
							{
								"index": 0,
								"delta": map[string]interface{}{
									"role":              "assistant",
									"reasoning_content": thinking,
								},
							},
						},
					})
				}
			case "message_delta":
				event.Usage.toUsage(usage)
			case "message_stop":
				m.writeOpenAISSE(dst, map[string]interface{}{
					"choices": []map[string]interface{}{
						{
							"index":         0,
							"delta":         map[string]interface{}{},
							"finish_reason": "stop",
						},
					},
				})
			}
		}
	}
	return rError
}

func (m *AnthropicProvider) writeOpenAISSE(w io.Writer, data interface{}) {
	dataBytes, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", string(dataBytes))
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (m *AnthropicProvider) generateID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 24)
	for i := range b {
		b[i] = charset[i%len(charset)]
	}
	return string(b)
}
