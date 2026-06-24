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

// ExecuteOpenAIRequest converts OpenAI request to Gemini format and executes it
func (m *GeminiProvider) ExecuteOpenAIRequest(c *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	var openAIReq struct {
		Model       string                   `json:"model"`
		Messages    []map[string]interface{} `json:"messages"`
		Stream      bool                     `json:"stream"`
		Temperature *float64                 `json:"temperature,omitempty"`
		MaxTokens   *int                     `json:"max_tokens,omitempty"`
	}
	if err := json.Unmarshal(body, &openAIReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	geminiReq := m.convertOpenAIToGemini(openAIReq.Messages, openAIReq.Temperature, openAIReq.MaxTokens)
	geminiBody, err := json.Marshal(geminiReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return err
	}

	method := "generateContent"
	if openAIReq.Stream {
		method = "streamGenerateContent"
	}

	url := fmt.Sprintf("%s/models/%s:%s?key=%s", m.cfg.BaseURL, pm.ModelID, method, m.cfg.APIKey)
	if openAIReq.Stream {
		url = url + "&alt=sse"
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(geminiBody))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}
	req = req.WithContext(c.Request.Context())
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
		return fmt.Errorf("Gemini API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	if openAIReq.Stream {
		c.Status(http.StatusOK)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		err = m.copyGeminiStreamingToOpenAI(c.Request.Context(), c.Writer, resp.Body, openAIReq.Model, usage)
	} else {
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		err = m.copyGeminiResponseToOpenAI(c.Writer, resp.Body, openAIReq.Model, usage)
	}
	return err
}

// ExecuteAnthropicRequest converts Anthropic request to Gemini format and executes it
func (m *GeminiProvider) ExecuteAnthropicRequest(c *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	// Standard hub strategy: direct Anthropic requests can be handled via format conversions, but for now we direct users to OpenAI protocol
	c.JSON(http.StatusBadRequest, gin.H{"error": "Direct Anthropic to Gemini requests not implemented yet. Please use OpenAI protocol standard."})
	return fmt.Errorf("Direct Anthropic to Gemini requests not implemented yet")
}

// Conversions
type geminiPart struct {
	Text string `json:"text,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts,omitempty"`
}

type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
}

type geminiRequest struct {
	Contents          []geminiContent          `json:"contents"`
	SystemInstruction *geminiSystemInstruction `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig  `json:"generationConfig,omitempty"`
}

func (m *GeminiProvider) convertOpenAIToGemini(messages []map[string]interface{}, temp *float64, maxTokens *int) geminiRequest {
	var req geminiRequest
	contents := []geminiContent{}
	var sysPart []geminiPart

	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)

		if role == "system" {
			sysPart = append(sysPart, geminiPart{Text: content})
		} else {
			geminiRole := "user"
			if role == "assistant" {
				geminiRole = "model"
			}
			contents = append(contents, geminiContent{
				Role:  geminiRole,
				Parts: []geminiPart{{Text: content}},
			})
		}
	}

	req.Contents = contents
	if len(sysPart) > 0 {
		req.SystemInstruction = &geminiSystemInstruction{Parts: sysPart}
	}

	if temp != nil || maxTokens != nil {
		req.GenerationConfig = &geminiGenerationConfig{
			Temperature:     temp,
			MaxOutputTokens: maxTokens,
		}
	}

	return req
}

type geminiResponseChunk struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

func (m *GeminiProvider) copyGeminiStreamingToOpenAI(ctx context.Context, dst io.Writer, src io.Reader, modelName string, usage *Usage) error {
	reader := bufio.NewReader(src)
	id := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					dst.Write([]byte("data: [DONE]\n\n"))
					return nil
				}
				return err
			}

			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				continue
			}

			// Handle SSE prefix
			if bytes.HasPrefix(trimmed, []byte("data:")) {
				trimmed = bytes.TrimPrefix(trimmed, []byte("data:"))
				trimmed = bytes.TrimSpace(trimmed)
			}

			// Strip array element prefixes/suffixes for raw streaming arrays
			trimmed = bytes.TrimPrefix(trimmed, []byte(","))
			trimmed = bytes.TrimPrefix(trimmed, []byte("["))
			trimmed = bytes.TrimSuffix(trimmed, []byte("]"))
			trimmed = bytes.TrimSpace(trimmed)

			if len(trimmed) == 0 {
				continue
			}

			var chunk geminiResponseChunk
			if err := json.Unmarshal(trimmed, &chunk); err != nil {
				continue
			}

			if chunk.UsageMetadata.PromptTokenCount > 0 {
				usage.InputTokens = chunk.UsageMetadata.PromptTokenCount
				usage.OutputTokens = chunk.UsageMetadata.CandidatesTokenCount
			}

			var text string
			if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
				text = chunk.Candidates[0].Content.Parts[0].Text
			}

			// Generate OpenAI format chunk
			openAIChunk := map[string]interface{}{
				"id":      id,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   modelName,
				"choices": []map[string]interface{}{
					{
						"index": 0,
						"delta": map[string]interface{}{
							"role":    "assistant",
							"content": text,
						},
						"finish_reason": nil,
					},
				},
			}

			if len(chunk.Candidates) > 0 && chunk.Candidates[0].FinishReason != "" && chunk.Candidates[0].FinishReason != "STOP" {
				openAIChunk["choices"].([]map[string]interface{})[0]["finish_reason"] = strings.ToLower(chunk.Candidates[0].FinishReason)
			}

			outputBytes, _ := json.Marshal(openAIChunk)
			fmt.Fprintf(dst, "data: %s\n\n", string(outputBytes))
			if flusher, ok := dst.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}
}

func (m *GeminiProvider) copyGeminiResponseToOpenAI(dst io.Writer, src io.Reader, modelName string, usage *Usage) error {
	body, err := io.ReadAll(src)
	if err != nil {
		return err
	}

	var geminiResp geminiResponseChunk
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return err
	}

	if geminiResp.UsageMetadata.PromptTokenCount > 0 {
		usage.InputTokens = geminiResp.UsageMetadata.PromptTokenCount
		usage.OutputTokens = geminiResp.UsageMetadata.CandidatesTokenCount
	}

	var text string
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		text = geminiResp.Candidates[0].Content.Parts[0].Text
	}

	openAIResp := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   modelName,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": text,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     usage.InputTokens,
			"completion_tokens": usage.OutputTokens,
			"total_tokens":      usage.TotalTokens(),
		},
	}

	outputBytes, _ := json.Marshal(openAIResp)
	dst.Write(outputBytes)
	return nil
}
