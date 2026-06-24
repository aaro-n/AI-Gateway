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

func (m *OpenAIProvider) ExecuteOpenAIRequest(c *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	bodyJson := map[string]interface{}{}
	if err := json.Unmarshal(body, &bodyJson); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}
	bodyJson["model"] = pm.ModelID
	body, err = json.Marshal(bodyJson)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	recordBody("O2O", "raw", body)
	req, err := http.NewRequest("POST", m.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	req = req.WithContext(c.Request.Context())
	req.Header.Set("Authorization", "Bearer "+m.cfg.APIKey)
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
		recordError("O2O", resp.StatusCode, respBody)
		return &ProviderError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	if m.isStreaming(resp) {
		c.Status(http.StatusOK)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		err = m.copyOpenAIStreaming(c.Request.Context(), c.Writer, resp.Body, usage)
	} else {
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		err = m.copyOpenAIResponse(c.Writer, resp.Body, usage)
	}
	return err
}

func (m *OpenAIProvider) copyOpenAIStreaming(ctx context.Context, dst io.Writer, src io.Reader, usage *Usage) error {
	src, dst = recordStream("O2O", src, dst)
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
					rError = fmt.Errorf("OpenAI copy stream error, %v", result.err)
					break streamLoop
				}
				continue
			}

			if _, err := fmt.Fprint(dst, result.line); err != nil {
				break streamLoop
			}
			if flusher, ok := dst.(http.Flusher); ok {
				flusher.Flush()
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
			if data == "[DONE]" {
				break streamLoop
			}

			var chunk struct {
				OpenAIUsage openAIUsage `json:"usage"`
			}

			if err := json.Unmarshal([]byte(data), &chunk); err == nil {
				chunk.OpenAIUsage.toUsage(usage)
			}
		}
	}
	return rError
}

func (m *OpenAIProvider) copyOpenAIResponse(dst io.Writer, src io.Reader, usage *Usage) error {
	body, err := io.ReadAll(src)
	if err != nil {
		return err
	}

	dst.Write(body)
	if flusher, ok := dst.(http.Flusher); ok {
		flusher.Flush()
	}

	var resp struct {
		OpenAIUsage openAIUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	resp.OpenAIUsage.toUsage(usage)
	return nil
}

func (m *OpenAIProvider) isStreaming(resp *http.Response) bool {
	contentType := resp.Header.Get("Content-Type")
	return len(resp.Header["Transfer-Encoding"]) > 0 ||
		(len(contentType) > 0 && len(contentType) >= 17 && contentType[:17] == "text/event-stream")
}
