# Tasks: 连接测试 + Token 统计 BUG 修复

## 1. TestConnection 完整修复 ✅

- [x] 1.1 从 `req.Endpoints` map 补充 5 个厂商 URL
- [x] 1.2 `findStoredAPIKey()` —— 优先查 `endpoints` JSON，兼容扁平列
- [x] 1.3 API Key 缺失时提前返回友好错误
- [x] 1.4 `AutoSyncModels` 保留 `lastErr` 并附加到最终错误消息

## 2. Gemini 2.5 Pro 空响应 ✅

- [x] 2.1 调试页 `maxOutputTokens`: 5 → 1024
- [x] 2.2 `buildTestBody` 统一 `maxTokens` 参数
- [x] 2.3 厂商页 `executeTest` → `maxTokens=5`（节流）

## 3. Anthropic Cache Token ✅

- [x] 3.1 `FormatUnified` 补充 `CacheHitTokens` + `CacheMissTokens`
- [x] 3.2 流式路径注释说明（SDK 不支持）

## 4. 统一错误格式 ✅

- [x] 4.1 `core/registry/errors.go`：`ResponseError` + `ErrorDetail`

## 5. 测试请求体一致性 ✅

- [x] 5.1 `buildTestBody` 和 `buildDebugRequestBody` 使用相同 prompt + maxTokens
