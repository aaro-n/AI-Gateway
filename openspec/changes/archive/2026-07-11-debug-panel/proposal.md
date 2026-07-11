# 调试面板 (Debug Panel)

为 AI Gateway 新增独立调试页面，支持测试模型供应商连接、模拟 API Key 访问、查看实时运行日志。

## 动机

- 模型厂商配置时排查连接问题需要反复切页面，效率低
- 无法直观看到 API Key 的请求/响应全链路
- 服务端运行时日志只能看终端，前端无日志可观测性

## 方案

### 后端

- 新增 `handler/debug.go`：`DebugHandler` 提供 4 个端点
  - `POST /debug/test-providers`：用 vendor API 直连测试连通性，返回 curl 格式请求 + 原始 JSON 响应
  - `POST /debug/test-key`：模拟客户端 SDK 通过网关发请求，dump 完整 HTTP 交互 + 协议转换损失分析
  - `GET /debug/recent-logs`：从 `model_logs` 表返回最近 50 条调用记录
  - `GET /debug/server-logs`：内存环形缓冲区 (`ringbuffer.go`) 返回运行时 slog 日志
- `registry.Provider` 扩展 `RawResponseCapturer` 可选接口，暴露 `FromUnified` 原始响应体
- 4 个 Provider (OpenAI/Anthropic/Gemini/DeepSeek) 实现 `LastRawResponse()` 捕获功能

### 前端

- 新增路由 `/debug`，页面分 3 个可折叠区域
- **测试模型供应商**：厂商选择器 + 模型选择器 + curl 风格请求/响应展示
- **测试 API 密钥**：密钥选择器 + 绑定的模型下拉框 + HTTP dump 日志 + 协议转换损失徽章 (ConversionBadge)
- **服务运行日志**：5s 自动轮询 + 智能滚动（离开底部不抢焦点，回到底部自动跟随）+ `自动跟随`/`查看历史` 状态标签

### 基础设施

- 新增 `core/errors/ringbuffer.go`：500 条环形缓冲区，包装 `slog.Handler` 拦截所有日志
- `tracker.go` 的 `init()`/`SetLevel()`/`SetOutput()` 全部改为使用 `ringBufferHandler`
- `main.go` 调用 `coreErrors.EnableRingBuffer(500)` 启动

## 影响范围

| 层 | 文件 | 改动类型 |
|----|------|----------|
| Handler | `debug.go` | 新增 |
| Core | `ringbuffer.go`, `tracker.go` | 新增 + 修改 |
| Registry | `registry.go` | `RawResponseCapturer` 接口 |
| Protocols | 4 个 `provider.go` + `request_build.go` | `lastRawResponse` 字段 |
| Middleware | `logger.go` | 无需改动 |
| Frontend | `views/Debug/` (3 文件) + 路由 + i18n + 菜单 | 新增 |
