## 1. 重构 `internal/mcp/debug.go` 错误处理

- [x] 抽取 `ensureDebugDir()` 辅助函数
- [x] 3 个 record 函数处理 `os.OpenFile` 错误
- [x] `recordLocalStream` 任一文件失败时关闭已打开文件并回退
- [x] `debugReader.Read` / `debugWriter.Write` 正确返回写入错误
- [x] 修复 `recordLocalStream` 第 67 行混入空格的缩进

## 2. 批量应用 gofmt

- [x] `gofmt -w` 处理 9 个剩余文件
- [x] `gofmt -l .` 输出为空

## 3. 验证

- [x] `go build ./...` 通过
- [x] `go vet ./...` 通过
- [x] `go test ./... -short` 通过
- [x] `npx vue-tsc -b` 通过
- [x] 检查 `mcp/debug.go` API 调用方（`client_local.go`、`client_remote.go`）签名兼容
