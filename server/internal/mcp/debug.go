package mcp

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"time"
)

const DEBUG_DIR = "debug_mcp"

var debugEnabled = false

func SetDebugMode(enabled bool) {
	debugEnabled = enabled
}

type debugReader struct {
	src  io.Reader
	file *os.File
}

func (r *debugReader) Read(p []byte) (int, error) {
	n, err := r.src.Read(p)
	if n > 0 {
		if _, werr := r.file.Write(p[:n]); werr != nil {
			return n, werr
		}
	}
	return n, err
}

type debugWriter struct {
	dst  io.Writer
	file *os.File
}

func (w *debugWriter) Write(p []byte) (int, error) {
	if _, err := w.file.Write(p); err != nil {
		return 0, err
	}
	return w.dst.Write(p)
}

// ensureDebugDir creates the debug log directory if debug mode is enabled.
// Returns the directory path on success, or an empty string if debug is
// disabled. Errors from MkdirAll are propagated so callers can fail loudly
// instead of silently writing to a missing directory.
func ensureDebugDir() (string, error) {
	if !debugEnabled {
		return "", nil
	}
	if err := os.MkdirAll(DEBUG_DIR, 0755); err != nil {
		return "", err
	}
	return DEBUG_DIR, nil
}

func recordRemoteReq(body []byte) {
	dir, err := ensureDebugDir()
	if err != nil || dir == "" {
		return
	}
	timestamp := time.Now().Format("20060102150405.000")
	reqFile, err := os.OpenFile(filepath.Join(dir, timestamp+"_remote_req.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer reqFile.Close()
	io.Copy(reqFile, bytes.NewReader(body))
}

func recordRemoteResp(body io.Reader) io.Reader {
	dir, err := ensureDebugDir()
	if err != nil || dir == "" {
		return body
	}
	timestamp := time.Now().Format("20060102150405.000")
	respFile, err := os.OpenFile(filepath.Join(dir, timestamp+"_remote_resp.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return body
	}
	return &debugReader{src: body, file: respFile}
}

func recordLocalStream(stdin io.Writer, stdout io.Reader) (io.Writer, io.Reader) {
	dir, err := ensureDebugDir()
	if err != nil || dir == "" {
		return stdin, stdout
	}
	timestamp := time.Now().Format("20060102150405.000")
	stdinLogFile, err := os.OpenFile(filepath.Join(dir, timestamp+"_local_stdin.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return stdin, stdout
	}
	stdoutLogFile, err := os.OpenFile(filepath.Join(dir, timestamp+"_local_stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		stdinLogFile.Close()
		return stdin, stdout
	}
	return &debugWriter{dst: stdin, file: stdinLogFile}, &debugReader{src: stdout, file: stdoutLogFile}
}
