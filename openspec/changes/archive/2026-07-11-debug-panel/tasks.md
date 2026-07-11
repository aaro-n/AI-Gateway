# Tasks: 调试面板

## 1. 环形日志缓冲区 ✅

- [x] 1.1 创建 `core/errors/ringbuffer.go`：`LogEntry` + `ringBufferHandler` + `EnableRingBuffer`
- [x] 1.2 修改 `tracker.go`：`init()`/`SetLevel()`/`SetOutput()` 用 `ringBufferHandler` 包装
- [x] 1.3 `main.go` 调用 `EnableRingBuffer(500)`

## 2. RawResponseCapturer ✅

- [x] 2.1 `registry.go` 新增 `RawResponseCapturer` 接口
- [x] 2.2 Gemini/OpenAI/Anthropic/DeepSeek 实现 `LastRawResponse()`
- [x] 2.3 各 Provider 的 `request_build.go` 中 OK 和错误路径都存储 `lastRawResponse`

## 3. 调试 API ✅

- [x] 3.1 `TestProviders(c *gin.Context)`：遍历 Provider 的 SupportedProtocols，用 `RunTest` 直连测试
- [x] 3.2 `TestKey(c *gin.Context)`：查询 Key → 构造 HTTP 请求 → `httputil.DumpRequestOut/Response` → 发送
- [x] 3.3 `ServerLogs(c *gin.Context)`：读取环形缓冲区（支持 `?since=` 增量轮询）
- [x] 3.4 `RecentLogs(c *gin.Context)`：从 `model_logs` 表返回最近 50 条
- [x] 3.5 `buildCurlCommand()`：curl 格式日志（含 URL + headers + body，API Key 脱敏）
- [x] 3.6 注册路由：`POST /debug/test-providers`，`POST /debug/test-key`，`GET /debug/recent-logs`，`GET /debug/server-logs`

## 4. 前端 ✅

- [x] 4.1 创建 `views/Debug/index.vue`：3 区域可折叠布局
- [x] 4.2 Provider 测试：厂商选择器 + 模型下拉框 + curl 日志展示
- [x] 4.3 Key 测试：密钥选择器 + 绑定模型选择器 + HTTP dump 展示 + ConversionBadge
- [x] 4.4 服务运行日志：暗色终端风格 + 5s 自动轮询 + 智能滚动
- [x] 4.5 创建 `ConversionBadge.vue` + `RuntimeConversionBadge.vue`
- [x] 4.6 路由 + i18n (zh/en) + 菜单项

## 5. 响应日志修复迭代 ✅

- [x] 5.1 Gemini 2.5 Pro 思考模型 `maxOutputTokens=5` 全部被思考消耗 → 改为 1024
- [x] 5.2 `debugLogEntry.Detail` 去掉 `omitempty` 防止空串被丢弃
- [x] 5.3 前端始终显示原始 JSON 响应，不再只显示解析文本
- [x] 5.4 智能滚动：离开底部不抢焦点，回到底部自动跟随
