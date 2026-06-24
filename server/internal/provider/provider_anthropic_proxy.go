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

func (m *AnthropicProvider) ExecuteAnthropicRequest(c *gin.Context, pm *model.ProviderModel, usage *Usage) error {
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

	recordBody("A2A", "raw", body)
	req, err := http.NewRequest("POST", m.cfg.BaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return err
	}

	req = req.WithContext(c.Request.Context())
	req.Header.Set("x-api-key", m.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
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
		recordError("A2A", resp.StatusCode, respBody)
		return &ProviderError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	if m.isStreaming(resp) {
		c.Status(http.StatusOK)
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		err = m.copyAnthropicStreaming(c.Request.Context(), c.Writer, resp.Body, usage)
	} else {
		c.Status(http.StatusOK)
		c.Header("Content-Type", "application/json")
		err = m.copyAnthropicResponse(c.Writer, resp.Body, usage)
	}
	return err
}

func (m *AnthropicProvider) ExecuteGeminiRequest(c *gin.Context, pm *model.ProviderModel, usage *Usage) error {
	c.JSON(http.StatusBadRequest, gin.H{"error": "Gemini native requests not supported by Anthropic provider"})
	return fmt.Errorf("Gemini native requests not supported by Anthropic provider")
}

func (m *AnthropicProvider) copyAnthropicStreaming(ctx context.Context, dst io.Writer, src io.Reader, usage *Usage) error {
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
					rError = fmt.Errorf("Anthropic copy stream error, %v", result.err)
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

			var event struct {
				Type  string         `json:"type"`
				Usage anthropicUsage `json:"usage"`
			}

			if err := json.Unmarshal([]byte(data), &event); err == nil {
				switch event.Type {
				case "message_delta":
					event.Usage.toUsage(usage)
				}
			}
		}
	}
	return rError
}

func (m *AnthropicProvider) copyAnthropicResponse(dst io.Writer, src io.Reader, usage *Usage) error {
	body, err := io.ReadAll(src)
	if err != nil {
		return err
	}

	dst.Write(body)
	if flusher, ok := dst.(http.Flusher); ok {
		flusher.Flush()
	}

	var resp struct {
		Usage anthropicUsage `json:"usage"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	resp.Usage.toUsage(usage)
	return nil
}
