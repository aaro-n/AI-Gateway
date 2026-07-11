# 修复 MCP debug 模块的错误处理与 gofmt 风格问题

## 动机

代码健康检查发现两类问题：

1. **`server/internal/mcp/debug.go` 静默吞掉错误**：5 处 `os.MkdirAll` 与 `os.OpenFile` 全部丢弃 `err` 返回值（部分用 `_, _ :=`，部分连 `:=` 都没有）。`debugReader.Read` / `debugWriter.Write` 也丢弃了 `file.Write` 的返回值。debug 模式下若目录创建失败或文件打开失败，调用方完全无感知，继续往 `nil` 文件写入会触发 panic；正常模式下 `os.MkdirAll` 的失败也无任何日志。
2. **gofmt 风格问题**：10 个 Go 文件未通过 `gofmt -l`，主要表现为 import 顺序（标准库应在第三方之前）、结构体字段对齐、文件末尾多余空行。`internal/mcp/debug.go:67` 还存在 tab 与空格混用（行首是一个普通空格而非 tab），触发 gofmt 单独报错。

## 方案

### 1. `internal/mcp/debug.go` 重构

- 抽取 `ensureDebugDir()` 统一处理 debug 模式判断与目录创建，错误向上传递。
- 3 个 record 函数改为：debug 关闭或目录创建失败时直接返回原始 reader/writer，文件打开失败时回退到非 debug 路径。
- `recordLocalStream` 中两个 log 文件任一打开失败时，关闭已打开的那个并回退。
- `debugReader.Read` / `debugWriter.Write` 改为正确返回 `file.Write` 的错误，签名改为 `(int, error)`。

### 2. 批量 `gofmt -w`

对其余 9 个文件执行 `gofmt -w`：

```
internal/middleware/resolve_slug.go
internal/protocols/deepseek/response_format.go
internal/protocols/deepseek/response_parse.go
internal/protocols/deepseek/sync.go
internal/protocols/openai/capabilities.go
internal/protocols/openai/response_parse.go
internal/protocols/openai/sync.go
internal/protocols/openrouter/request_parse.go
internal/protocols/openrouter/sync.go
```

## 影响范围

- **后端**：
  - `server/internal/mcp/debug.go` — 行为变更：debug 模式开启但目录不可写时不再 panic，而是降级为非 debug 行为；正常路径完全不受影响
  - 其余 9 个文件 — 仅风格调整，零行为变更
- **API 兼容性**：公开函数签名（`recordRemoteReq`、`recordRemoteResp`、`recordLocalStream`）未变
- **测试**：`go test ./...` 全部通过
- **构建**：`go build ./...`、`go vet ./...`、`vue-tsc -b` 全部通过
