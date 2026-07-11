# 连接测试 + Token 统计 BUG 修复

修复厂商添加流程中 TestConnection 失败、模型测试响应为空、Token 用量统计不正确等一组连锁 BUG。

## BUG 1：添加厂商时 `no valid models found`

**现象**：在「模型厂商」页面添加 Gemini Provider，测试连接时返回 `Connection test failed: no valid models found`。

**根因**：前端通过 `{"endpoints": {"gemini": "https://..."}}` 传参，后端 `TestConnection` 只读扁平字段 `req.GeminiBaseURL`，忽略 `req.Endpoints` map。

**修复**：
- 从 `req.Endpoints` 补充 URL：`if req.Endpoints["gemini"] != "" && geminiURL == "" { geminiURL = req.Endpoints["gemini"] }`
- `findStoredAPIKey()`：查找已有 Provider 的 API Key，优先查 `endpoints` JSON 列，其次查扁平列

**影响范围**：所有厂商（OpenAI/Anthropic/DeepSeek 同样受影响）

## BUG 2：编辑已有厂商时 API Key 查找失败

**根因**：`TestConnection` 用 `WHERE gemini_base_url = ?` 查已有 Provider 的 API Key，但新 Provider 的 URL 存在 `endpoints` JSON 列，扁平列为空。

**修复**：`findStoredAPIKey()` 遍历所有 Provider，优先匹配 `endpoints` JSON，兼容扁平列 fallback。

## BUG 3：API Key 缺失时无友好提示

**现象**：未填 API Key 时点击「测试连接」，返回 `no valid models found`，无法定位原因。

**修复**：`apiKey == ""` 时直接返回 `"Connection test failed: API key is required. Please enter your API key."`

## BUG 4：`no valid models found` 吞没真实原因

`AutoSyncModels` 中每个 provider 的 `SyncModels` 出错时（如 Gemini 返回 `API_KEY_INVALID`），错误被丢弃，只返回通用信息。

**修复**：保留 `lastErr`，最终错误消息附带实际 API 错误。

## BUG 5：Gemini 2.5 Pro 测试返回空响应

**根因**：Gemini 2.5 Pro 是思考模型，`maxOutputTokens=5` 全部被 `thoughtsTokenCount` 消耗，`content.parts` 为空。

**修复**：
- 调试页 `maxOutputTokens` 改为 1024（预留思考+输出空间）
- 厂商页模型测试 `maxTokens=5`（仅验证连通性，节流）

## BUG 6：Anthropic 缓存用量丢失

**根因**：`AnthropicProvider.FormatUnified` 非流式路径只设置 `InputTokens` 和 `OutputTokens`，SDK 返回的 `CacheReadInputTokens` / `CacheCreationInputTokens` 未传。

**修复**：补充 `usage.CacheHitTokens` / `usage.CacheMissTokens`。

## 影响范围

| 文件 | 改动 |
|------|------|
| `handler/provider.go` | `endpoints` map 读取 + `findStoredAPIKey()` + 友好提示 |
| `protocols/automated.go` | `lastErr` 保留 |
| `protocols/testrunner.go` | `maxTokens` 参数 |
| `handler/model_testing.go` | `executeTest` 传 5 |
| `handler/debug.go` | 调试页 `RunTest` 传 1024 |
| `protocols/anthropic/response_format.go` | CacheHit/CacheMiss 补充 |
