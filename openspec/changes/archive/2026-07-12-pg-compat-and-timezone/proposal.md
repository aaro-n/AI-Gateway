# PostgreSQL 兼容性修复 + 时区配置支持

## 动机

代码审查发现 PostgreSQL 模式存在两个兼容性问题，以及统计时区行为不一致：

1. **布尔列整数赋值（启动失败）**：`db.go:585-586` 用 `enabled = 1` / `enabled = 0`
   更新 boolean 列。SQLite 接受（boolean 存为 INTEGER），但 PostgreSQL 严格类型检查
   会报错 `column "enabled" is of type boolean but expression is of type integer`，
   导致 PG 模式启动时 `InitDB` 失败。

2. **DATE() 时区行为差异（统计错位）**：`usage.go` 3 处原生 SQL 用 `DATE(created_at)`
   按日分组。SQLite 的 `DATE()` 按 UTC 转换，PostgreSQL 的 `DATE()` 按服务器时区转换，
   导致同一记录在两种数据库下归属到不同日期，仪表盘每日统计错位。

3. **无 PG 集成测试**：所有测试仅针对 SQLite，PG 兼容性问题无法在 CI 中捕获。

## 方案

### 1. 布尔赋值修复（db.go）

将 `enabled = 1` / `enabled = 0` 改为 `enabled = true` / `enabled = false`。
`true`/`false` 字面量在 SQLite（true=1, false=0）和 PostgreSQL（boolean 类型）都合法。

### 2. 时区配置 + 统计分组重构

**设计原则**：数据库统一存 UTC（GORM `time.Time` 自带时区信息），程序通过环境变量
设置用户时区，展示/统计时在 Go 端按用户时区转换。

- `config.go`：新增 `ServerConfig.TimeZone` 字段，环境变量 `AG_TIME_ZONE`，默认 `Asia/Shanghai`。
  启动时调用 `time.LoadLocation(tz)` 设置全局 `time.Local`，无效时区回退 UTC 并告警。
- `usage.go`：移除 3 处 `DATE(created_at)` 原生 SQL，改为查询 `created_at` 后在 Go 端
  用 `groupByDate` 辅助函数按 `time.Local` 分组。彻底消除 SQLite/PG 方言差异。
- `Dashboard` 和 `generateLastNDays` 改用 `time.Now().Local()` 计算日边界。

### 3. PostgreSQL 集成测试

新增 `server/internal/model/pg_compat_test.go`，通过 `AG_TEST_POSTGRES_URL` 环境变量
连接外部 PG，未设置时 `t.Skip` 跳过。覆盖：
- `TestPG_AutoMigrate`：所有模型在 PG 上 auto-migrate
- `TestPG_BooleanColumn`：boolean 列 true/false 赋值（db.go 修复回归测试）
- `TestPG_TimestampRoundTrip`：时间戳存储往返 + 按时区分组归属

CI 中可配合 `docker-compose up -d postgres` 运行。

### 修改文件

| 文件 | 变更 |
|---|---|
| `server/internal/config/config.go` | 新增 `ServerConfig.TimeZone`、`AG_TIME_ZONE`、`applyTimeZone()`、`DSN()` 改 `_loc=UTC` |
| `server/internal/model/db.go` | 布尔赋值 `true/false`、GORM `NowFunc` 返回 `UTC()`、SQLite `_loc=UTC`、`User` 表新增 `TimeZone` |
| `server/internal/core/errors/ringbuffer.go` | `LogEntry.Timestamp` 改存 RFC3339Nano UTC |
| `server/internal/handler/usage.go` | 移除 `DATE()` 原生 SQL、`groupByDate` Go 端分组、`userTimeZone()` 按用户时区 |
| `server/internal/handler/auth.go` | `Me()` 返回 `time_zone`、新增 `UpdateTimeZone()` API |
| `server/internal/handler/provider.go` | `CreatedAt string` → `time.Time`，移除 `.Format()` |
| `server/internal/handler/model.go` | `CreatedAt string` → `time.Time`，移除 `.Format()` |
| `server/internal/handler/provider_model.go` | `CreatedAt string` → `time.Time`，移除 `.Format()` |
| `server/internal/handler/debug.go` | `modelLogEntry.CreatedAt` → `time.Time`、`now()` 用 RFC3339Nano |
| `server/internal/handler/key.go` | `time.Parse` → `time.ParseInLocation(time.Local)` |
| `server/cmd/server/main.go` | 注册 `PUT /auth/timezone` 路由 |
| `server/internal/model/pg_compat_test.go` | 新增 PG 集成测试（3 个用例） |
| `server/config.yaml.example` | 新增 `server.timezone` 配置文档 |
| `docker-compose.yml` | 新增 `AG_TIME_ZONE` 环境变量（默认 `Asia/Shanghai`） |
| `README.md` | 新增 `AG_TIME_ZONE` 行 |
| `web/src/utils/format.ts` | `formatDateTime`/`formatDate` 加 `timeZone`、新增 `formatLogTime`/`setUserTimeZone` |
| `web/src/stores/user.ts` | `User` 加 `time_zone`、新增 `updateTimeZone` |
| `web/src/router/index.ts` | `fetchUser` 后同步时区到 `setUserTimeZone` |
| `web/src/views/Debug/index.vue` | 日志时间用 `formatLogTime` |
| `web/src/views/Settings/index.vue` | 新增时区选择卡片 |
| `web/src/locales/zh.ts`、`en.ts` | 新增时区相关翻译 |

### 向后兼容

- `AG_TIME_ZONE` 未设置时默认 `Asia/Shanghai`，符合主要用户群体习惯
- 布尔赋值 `true/false` 在 SQLite 上等价于 `1/0`
- `usage.go` 重构不改变 API 响应结构，仅修正日期归属

## 验证

- `go build ./...` ✅
- `go vet ./...` ✅
- `go test -count=1 ./...` ✅（5 个测试包通过，PG 测试跳过）
- `vue-tsc -b --noEmit` ✅
- `npm run build` / `make build` ✅
- `AG_TIME_ZONE=Asia/Shanghai` 启动日志显示 `Time Zone: Asia/Shanghai` ✅
- 无效时区 `Foo/Bar` 回退 UTC 并告警 ✅
- 默认时区为 `Asia/Shanghai` ✅