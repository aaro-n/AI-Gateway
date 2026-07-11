# Tasks: HTTP 请求日志全面优化

## 1. 日志级别重分级 ✅

- [x] 1.1 `middleware/logger.go` 三分支：`status>=500`→ERROR，`>=400`→WARN，`/debug/server-logs`→DEBUG，其余→INFO
- [x] 1.2 验证成功请求 (2xx) 在默认日志级别下可见

## 2. 响应体策略 ✅

- [x] 2.1 成功请求改用 `resp_bytes=%d` 替代 `resp=%q`
- [x] 2.2 失败请求保留 `resp=%q` 用于诊断

## 3. 轮询端点降噪 ✅

- [x] 3.1 `/api/v1/debug/server-logs` 降级为 DEBUG 级别，默认不显示

## 4. 智能滚动 ✅

- [x] 4.1 `fetchServerLogs` 追加前检测 `wasAtBottom`
- [x] 4.2 `reset=true` 用 `nextTick + requestAnimationFrame` 等待 DOM 渲染后滚到底
- [x] 4.3 `onLogScroll` 实时更新 `isAtBottom`
- [x] 4.4 始终轮询，`isAtBottom` 仅控制是否自动滚动
- [x] 4.5 状态标签 `自动跟随`/`查看历史` 替换切换开关
