# Tasks: Pipeline 中间件架构

## 1. 核心工具 ✅

- [x] 1.1 `core/urlutil/url.go`：`NormalizeBaseURL`, `BuildRequestURL`, `JoinURL`
- [x] 1.2 `core/errors/upstream.go`：`UpstreamError` + `WrapUpstreamError` + `IsUpstreamError`
- [x] 1.3 `core/registry/errors.go`：`ResponseError` + `ErrorDetail` 统一错误结构

## 2. Pipeline 核心 ✅

- [x] 2.1 `pipeline/pipeline.go`：`Pipeline.Process()` + `Option` 模式 + `Middleware` 接口
- [x] 2.2 便捷构造：`OnRequest`, `OnResponse`, `OnError`

## 3. 中间件 ✅

- [x] 3.1 `pipeline/maxtoken/max_token.go`：`EnsureMaxTokens`, `CapMaxTokens`
- [x] 3.2 `pipeline/retry.go`：`RetryableProvider` 自动重试 5xx/429

## 4. Usage 双向转换 ✅

- [x] 4.1 `core/unified/usage.go`：`DetailedUsage` + `PromptTokensDetails` + `CompletionTokensDetails`
- [x] 4.2 厂商转换：`OpenAIToDetailed`, `AnthropicToDetailed`, `GeminiToDetailed`
- [x] 4.3 双向：`ToDetailedUsage()`, `ToUsage()`

## 5. Anthropic Cache Token 修复 ✅

- [x] 5.1 `anthropic/response_format.go` 非流式路径补充 `CacheHitTokens`, `CacheMissTokens`
- [x] 5.2 流式路径添加注释说明（SDK 不提供流式缓存数据）

## 6. 厂商页 Token 节流 ✅

- [x] 6.1 `RunTest` 签名改为 `RunTest(protocol, baseURL, apiKey, modelID, maxTokens)`
- [x] 6.2 `buildTestBody` 接收 `maxTokens` 参数
- [x] 6.3 `model_testing.go` → `executeTest` 传入 `5`
- [x] 6.4 调试页传入 `1024`
