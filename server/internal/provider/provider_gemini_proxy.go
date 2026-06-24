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

// ExecuteGeminiRequest handles direct native Gemini requests
func (m *GeminiProvider) ExecuteGeminiRequest(c *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	method := "generateContent"
	if strings.Contains(c.Request.URL.Path, "streamGenerateContent") {
		method = "streamGenerateContent"
	}

	url := fmt.Sprintf("%s/models/%s:%s", m.cfg.BaseURL, pm.ModelID, method)
	if c.Request.URL.RawQuery != "" {
		url = url + "?" + c.Request.URL.RawQuery
		if !strings.Contains(url, "key=") {
			url = url + "&key=" + m.cfg.APIKey
		}
	} else {
		url = url + "?key=" + m.cfg.APIKey
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	req = req.WithContext(c.Request.Context())
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
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
		return &ProviderError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	// Copy response headers and set status
	c.Status(http.StatusOK)
	for k, vv := range resp.Header {
		for _, v := range vv {
			c.Header(k, v)
		}
	}

	if method == "streamGenerateContent" {
		err = m.copyGeminiStreaming(c.Request.Context(), c.Writer, resp.Body, usage)
	} else {
		err = m.copyGeminiResponse(c.Writer, resp.Body, usage)
	}
	return err
}

func (m *GeminiProvider) copyGeminiStreaming(ctx context.Context, dst io.Writer, src io.Reader, usage *Usage) error {
	reader := bufio.NewReader(src)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}

			if _, err := dst.Write(line); err != nil {
				return err
			}
			if flusher, ok := dst.(http.Flusher); ok {
				flusher.Flush()
			}

			var usageMeta struct {
				UsageMetadata struct {
					PromptTokenCount     int `json:"promptTokenCount"`
					CandidatesTokenCount int `json:"candidatesTokenCount"`
				} `json:"usageMetadata"`
			}
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				if bytes.HasPrefix(trimmed, []byte("data:")) {
					trimmed = bytes.TrimPrefix(trimmed, []byte("data:"))
					trimmed = bytes.TrimSpace(trimmed)
				}
				trimmed = bytes.TrimPrefix(trimmed, []byte(","))
				trimmed = bytes.TrimPrefix(trimmed, []byte("["))
				trimmed = bytes.TrimSuffix(trimmed, []byte("]"))
				trimmed = bytes.TrimSpace(trimmed)

				if err := json.Unmarshal(trimmed, &usageMeta); err == nil && usageMeta.UsageMetadata.PromptTokenCount > 0 {
					usage.InputTokens = usageMeta.UsageMetadata.PromptTokenCount
					usage.OutputTokens = usageMeta.UsageMetadata.CandidatesTokenCount
				}
			}
		}
	}
}

func (m *GeminiProvider) copyGeminiResponse(dst io.Writer, src io.Reader, usage *Usage) error {
	body, err := io.ReadAll(src)
	if err != nil {
		return err
	}

	if _, err := dst.Write(body); err != nil {
		return err
	}
	if flusher, ok := dst.(http.Flusher); ok {
		flusher.Flush()
	}

	var usageMeta struct {
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(body, &usageMeta); err == nil {
		usage.InputTokens = usageMeta.UsageMetadata.PromptTokenCount
		usage.OutputTokens = usageMeta.UsageMetadata.CandidatesTokenCount
	}
	return nil
}
