# Pipeline 中间件架构

借鉴 AxonHub 的可组合 Pipeline 模式，为 AI Gateway 引入重试、空响应检测、MaxToken 守卫、错误分类等可组合中间件。

## 动机

- `gateway_proxy.go` 中 failover 逻辑全部内联（~200 行 for 循环），难以测试和复用
- 没有统一的重试机制（429/5xx 直接返回失败）
- 部分厂商（Anthropic）缓存用量未传到日志
- 缺少 MaxToken 安全上限

## 方案

### Pipeline 核心

`protocols/pipeline/pipeline.go`：
- `Pipeline.Process(ctx, body, modelID)` → `inbound.ToUnified → middlewares → outbound.FromUnified → return`
- `Option` 模式：`WithRetry(n, delay)` / `WithTimeout(d)` / `WithEmptyResponseDetection()` / `WithMiddlewares(mws...)`
- `Middleware` 接口：`OnRequest`(Forward) + `OnResponse`(Reverse) + `OnError`(Reverse)
- 便捷构造：`OnRequest(name, fn)` / `OnResponse(name, fn)` / `OnError(name, fn)`

### MaxToken 守卫

`pipeline/maxtoken/max_token.go`：
- `EnsureMaxTokens(default, max)`：未设置则用默认值，超上限则截断
- `CapMaxTokens(max)`：必须设置且不超上限

### RetryableProvider

`pipeline/retry.go`：
- 包装 `registry.Provider`，在 `FromUnified` 层自动重试
- 默认重试策略：5xx + 429 可重试，4xx 不重试
- `CanRetry` 回调支持定制

### UpstreamError

`core/errors/upstream.go`：
- `UpstreamError{StatusCode, Body, Err}`：区分上游错误与本地错误
- `IsRateLimit()` / `IsRetryable()` / `WrapUpstreamError()` / `IsUpstreamError()`

### 统一错误格式

`core/registry/errors.go`：
- `ResponseError{StatusCode, ErrorDetail{Message, Type, Param, Code, RequestID}}`

### 厂商页 Token 节流

`model_testing.go` → `executeTest` 调用 `RunTest(protocol, baseURL, key, modelID, 5)`

## 影响范围

| 层 | 文件 | 改动类型 |
|----|------|----------|
| Protocols/Pipeline | `pipeline.go`, `retry.go`, `maxtoken/max_token.go` | 新增 |
| Core/Errors | `upstream.go` | 新增 |
| Core/Registry | `errors.go` | 新增 |
| Core/Unified | `usage.go` | 新增 |
| Core/URL | `urlutil/url.go` | 新增 |
| Handler | `model_testing.go` | `RunTest` 签名变更 |
| Protocols | `testrunner.go` | maxTokens 参数 |
