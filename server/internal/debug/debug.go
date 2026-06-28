// Package debug 提供调试模式的全局开关，用于控制请求/响应体的文件记录。
package debug

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const debugDir = "debug_model"

var enabled = false

// SetEnabled 设置全局调试模式开关。
func SetEnabled(on bool) {
	enabled = on
}

// IsEnabled 返回当前调试模式状态。
func IsEnabled() bool {
	return enabled
}

// RecordBody 将请求/响应体记录到调试文件。
func RecordBody(method, bodyType string, body []byte) {
	if !enabled {
		return
	}
	os.MkdirAll(debugDir, 0755)
	timestamp := time.Now().Format("20060102150405.000")
	filename := filepath.Join(debugDir, timestamp+"_"+method+"_"+bodyType+".log")
	f, _ := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if f != nil {
		defer f.Close()
		io.Copy(f, bytes.NewReader(body))
	}
}

// RecordError 将错误响应体记录到调试文件。
func RecordError(method string, httpCode int, body []byte) {
	if !enabled {
		return
	}
	os.MkdirAll(debugDir, 0755)
	timestamp := time.Now().Format("20060102150405.000")
	filename := filepath.Join(debugDir, timestamp+"_"+method+"_error_"+strconv.Itoa(httpCode)+".log")
	f, _ := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if f != nil {
		defer f.Close()
		io.Copy(f, bytes.NewReader(body))
	}
}

// NewReadCloser 包装 io.Reader 使其同步记录读取内容。
func NewReadCloser(src io.Reader, method string) io.ReadCloser {
	if !enabled {
		if rc, ok := src.(io.ReadCloser); ok {
			return rc
		}
		return io.NopCloser(src)
	}
	os.MkdirAll(debugDir, 0755)
	timestamp := time.Now().Format("20060102150405.000")
	filename := filepath.Join(debugDir, timestamp+"_"+method+"_stream.log")
	f, _ := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	return &debugReader{src: src, file: f}
}

type debugReader struct {
	src  io.Reader
	file *os.File
}

func (r *debugReader) Read(p []byte) (n int, err error) {
	n, err = r.src.Read(p)
	if n > 0 && r.file != nil {
		r.file.Write(p[:n])
	}
	return
}

func (r *debugReader) Close() error {
	if r.file != nil {
		r.file.Close()
	}
	if rc, ok := r.src.(io.ReadCloser); ok {
		return rc.Close()
	}
	return nil
}
