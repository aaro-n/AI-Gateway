## 1. config.go — 时区配置

- [x] 新增 `ServerConfig.TimeZone` 字段
- [x] `Load()` 读取 `AG_TIME_ZONE` 环境变量
- [x] `applyDefaults()` 默认 `Asia/Shanghai`
- [x] 新增 `applyTimeZone()` 设置全局 `time.Local`
- [x] `logConfig()` 输出时区配置
- [x] 无效时区回退 UTC 并告警
- [x] `DSN()` SQLite `_loc=auto` → `_loc=UTC`

## 2. db.go — 数据库层改动

- [x] 布尔赋值 `enabled = 1/0` → `enabled = true/false`（PG 兼容）
- [x] SQLite DSN `_loc=auto` → `_loc=UTC`（读取位置 UTC）
- [x] GORM `NowFunc` 设为 `time.Now().UTC()`（写入 UTC）
- [x] `User` 表新增 `TimeZone` 字段

## 3. ringbuffer.go — 日志时间格式

- [x] `LogEntry.Timestamp` 改存 RFC3339Nano UTC
- [x] `appendToRing` 和 `PushRingEntry` 用 `time.Now().UTC().Format(time.RFC3339Nano)`
- [x] `GetRingBufferEntriesSince` 字符串比较（RFC3339 定长可排序）

## 4. usage.go — 统计分组重构

- [x] 移除 3 处 `DATE(created_at)` 原生 SQL
- [x] 新增 `groupByDate` 泛型辅助函数（Go 端分组）
- [x] 新增 `userTimeZone()` 从 session 获取用户时区
- [x] `Dashboard`/`ModelLogs`/`MCPLogs` 按用户时区计算日边界
- [x] `generateLastNDays` → `generateLastNDaysIn` 接收时区参数

## 5. handler 时间字段统一

- [x] `provider.go`: `CreatedAt string` → `time.Time`, 移除 `.Format()`
- [x] `model.go`: `CreatedAt string` → `time.Time`, 移除 `.Format()`
- [x] `provider_model.go`: `CreatedAt string` → `time.Time`, 移除 `.Format()`
- [x] `debug.go`: `modelLogEntry.CreatedAt string` → `time.Time`, `now()` 用 RFC3339Nano UTC
- [x] `key.go`: `time.Parse` → `time.ParseInLocation(time.Local)`

## 6. auth.go — 用户时区 API

- [x] `Me()` 返回 `time_zone` 字段
- [x] 新增 `UpdateTimeZone()` API（`PUT /auth/timezone`）
- [x] 路由注册 `protected.PUT("/auth/timezone", ...)`

## 7. pg_compat_test.go — PG 集成测试

- [x] `TestPG_AutoMigrate`：所有模型 PG auto-migrate
- [x] `TestPG_BooleanColumn`：boolean 列 true/false 赋值回归测试
- [x] `TestPG_TimestampRoundTrip`：时间戳往返 + 时区分组归属
- [x] 无 `AG_TEST_POSTGRES_URL` 时 `t.Skip` 跳过

## 8. 前端改动

- [x] `format.ts`: `formatDateTime`/`formatDate` 加 `timeZone` 参数, 用 `Intl.DateTimeFormat`
- [x] `format.ts`: 新增 `formatLogTime`、`setUserTimeZone`、`getUserTimeZone`
- [x] `stores/user.ts`: `User` 加 `time_zone`，新增 `updateTimeZone`
- [x] `router/index.ts`: `fetchUser` 后同步时区
- [x] `Debug/index.vue`: 日志时间用 `formatLogTime` 按时区显示
- [x] `Settings/index.vue`: 新增时区选择卡片
- [x] `locales/zh.ts`、`en.ts`: 新增时区相关翻译

## 9. 配置文档

- [x] `config.yaml.example` 新增 `server.timezone` 说明
- [x] `docker-compose.yml` 新增 `AG_TIME_ZONE` 环境变量（默认 `Asia/Shanghai`）
- [x] `README.md` 环境变量表新增 `AG_TIME_ZONE` 行
- [x] 确认 `Dockerfile` 已安装 `tzdata`

## 10. 验证

- [x] `go build ./...` ✅
- [x] `go vet ./...` ✅
- [x] `go test -count=1 ./...` ✅（PG 测试跳过）
- [x] `vue-tsc -b --noEmit` ✅
- [x] `npm run build` / `make build` ✅
- [x] `AG_TIME_ZONE=Asia/Shanghai` 启动日志正确
- [x] 无效时区回退 UTC 并告警
- [x] 默认时区为 Asia/Shanghai