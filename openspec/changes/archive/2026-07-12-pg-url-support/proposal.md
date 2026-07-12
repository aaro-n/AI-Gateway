# PostgreSQL 连接 URL 支持

## 动机

当前 PostgreSQL 连接仅支持独立字段配置（host/port/username/password/dbname），
`sslmode=disable` 硬编码，无法使用：

- `DATABASE_URL` 环境变量（云平台常用）
- `postgres://user:pass@host/db` URL 格式
- `host=xxx user=xxx password=xxx` key=value DSN 格式（Supabase/PgBouncer）
- SSL 证书认证（sslmode=verify-full + sslrootcert）
- PgBouncer 等连接池的 `default_query_exec_mode=simple_protocol`

## 方案

在 `DatabaseConfig` 中新增 `URL` 字段（`AG_DATABASE_URL` 环境变量），
优先级高于独立字段。`postgres.Open(dsn)` 直接透传 URL，pgx 驱动原生支持两种格式。

### 修改文件

| 文件 | 变更 |
|---|---|
| `server/internal/config/config.go` | 新增 `DatabaseConfig.URL` 字段、`AG_DATABASE_URL` 环境变量、`DSN()` 优先返回 URL、`logConfig()` 增加 URL 日志 |
| `server/internal/model/db.go` | `InitDB()` 新增 `dbURL` 参数，非空时直接使用 |
| `server/cmd/server/main.go` | 传递 `cfg.Database.URL` 给 `InitDB()` |
| `server/config.yaml.example` | 新增 URL 配置文档（URL 格式、key=value 格式、SSL 参数、pgx 高级参数说明） |

### 向后兼容

URL 为空时完全回退到独立字段拼接，原有配置无需改动。

---

# GitHub Actions 安全审查与修复

## 动机

审查 `.github/workflows/` 下 7 个 workflow 文件，发现多个安全问题。

## 修复项

| 问题 | 影响范围 | 修复 |
|---|---|---|
| `actions/checkout@v5` 不存在 | 全部 7 个 workflow | → `@v4` |
| `aquasecurity/trivy-action@master` 浮动标签 | `security-scan.yml`（4 处） | → `@v1` |
| GitGuardian `args` 传完整命令 `secret scan path .` | `ggshield.yml` | 只传标志 `--exit-zero`，action 内部自动运行 `secret scan ci` |
| Trivy image-scan Docker 构建失败 (`load: true` + buildx) | `security-scan.yml` | 改用原生 `docker build` |
| `alpine:latest` 浮动镜像 | `Dockerfile` | → `alpine:3.21` |
| PR 时 `--exit-zero` 让秘钥检测形同虚设 | `ggshield.yml` | PR 时不加 `--exit-zero` |
