## 1. config.go — DatabaseConfig 增加 URL 字段

- [x] 新增 `URL string` 字段
- [x] `Load()` 增加 `AG_DATABASE_URL` 环境变量
- [x] `DSN()` 方法 URL 优先
- [x] `logConfig()` 增加 URL 日志（密码脱敏）

## 2. db.go — InitDB 支持 URL 参数

- [x] 新增 `dbURL` 参数
- [x] URL 非空时直接 `postgres.Open(dbURL)`

## 3. main.go — 传递 URL 参数

- [x] 传递 `cfg.Database.URL`

## 4. config.yaml.example — 文档更新

- [x] URL 格式示例（postgres://...?sslmode=...）
- [x] key=value 格式示例（Supabase/PgBouncer）
- [x] SSL 参数说明（sslmode、sslrootcert、sslcert、sslkey）
- [x] pgx 高级参数说明（default_query_exec_mode）

## 5. 验证

- [x] `go build ./cmd/server/` 通过

## 6. GitHub Actions 安全修复

- [x] `actions/checkout@v5` → `@v4`（7 个文件）
- [x] `trivy-action@master` → `@v1`（4 处）
- [x] `ggshield.yml` args 修复：`--exit-zero` only，不加命令
- [x] `security-scan.yml` image-scan：`docker/build-push-action` → `docker build`
- [x] `Dockerfile`：`alpine:latest` → `alpine:3.21`
- [x] `ggshield.yml`：PR 时去掉 `--exit-zero`，push/schedule 保留
