# HTTP 请求日志全面优化

诊断并修复 AI Gateway 服务运行日志中的四个长期问题：日志级别错配导致成功请求不可见、响应体完整 JSON 导致日志爆炸、轮询端点造成指数级套娃、日志面板滚动体验不佳。

## 问题

### 1. 成功请求完全不可见
`RequestLogger` 中间件将成功请求（status < 400）记录为 `TraceDebug` → `slog.LevelDebug`。默认配置下 `AG_LOG_LEVEL` 未设置，slog Handler 级别为 `Info`，DEBUG 日志全部被过滤。只有 4xx/5xx 错误才显示。

### 2. 响应体爆炸
`/api/v1/providers` 返回完整 JSON（~7KB 含所有模型详情），`trimBody` 截断到 600 字符后仍溢出日志行，且包含多层转义引号。

### 3. 轮询日志套娃
`/api/v1/debug/server-logs` 本身返回日志 JSON，每次轮询的 `resp` 包含前一次轮询的 `resp` → 指数级嵌套 → `\\\\\\\"logs\\\\\\\"` 完全不可读。

### 4. 滚动体验
首次打开不滚到最新日志；滚到底部后不自动跟随新日志。

## 方案

### 日志级别重分级

```go
// 旧：全部 4xx+ → ERROR，其余 → DEBUG
// 新：三级
status >= 500 → TraceError   (ERROR, 始终可见)
status >= 400 → TraceWarn    (WARN,  始终可见)
path = /debug/server-logs → TraceDebug (DEBUG, 不可见)
其余 2xx      → TraceInfo    (INFO,  始终可见)
```

### 响应体策略

成功请求（2xx）：`resp_bytes=%d` 替换 `resp=%q`，不记录体内容。
失败请求（4xx/5xx）：保留 `resp=%q` 用于诊断。

### 智能滚动

- `fetchServerLogs` 追加前先检测 `wasAtBottom`
- `reset=true` 时用 `nextTick + requestAnimationFrame` 等 DOM 渲染完成后滚到底
- `onLogScroll` 事件实时更新 `isAtBottom`
- 始终轮询不停，`isAtBottom` 仅控制是否自动滚动
- 状态标签实时显示 `自动跟随` / `查看历史`

## 影响范围

| 文件 | 改动 |
|------|------|
| `middleware/logger.go` | 日志级别三分支 + `resp_bytes` + `/debug/server-logs` 降级 |
| `views/Debug/index.vue` | 智能滚动逻辑 + 状态标签替换开关 |
