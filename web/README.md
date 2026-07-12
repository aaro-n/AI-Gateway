# Vue 3 + TypeScript + Vite

This template should help get you started developing with Vue 3 and TypeScript in Vite. The template uses Vue 3 `<script setup>` SFCs, check out the [script setup docs](https://v3.vuejs.org/api/sfc-script-setup.html#sfc-script-setup) to learn more.

Learn more about the recommended Project Setup and IDE Support in the [Vue Docs TypeScript Guide](https://vuejs.org/guide/typescript/overview.html#project-setup).

## Docker 部署 — 最小环境变量

Docker 镜像已内置 `config.yaml` 的所有默认值，**为零配置运行**（默认 SQLite + 端口 18080 + 管理员 admin/admin）。

```bash
# 最小启动（SQLite，所有默认值）
docker run -d -p 18080:18080 ghcr.io/aaro-n/ai-gateway:latest
```

### 生产环境至少需要设置

| 环境变量 | 必填 | 默认值 | 说明 |
|----------|:----:|--------|------|
| `AG_DATABASE_TYPE` | ✓ | `sqlite` | `postgres` 或 `sqlite` |
| `AG_DATABASE_URL` | — | — | PostgreSQL 连接 URL/DSN（推荐，优先级高于字段） |
| `AG_DATABASE_HOST` | — | — | PostgreSQL 主机地址（`url` 为空时使用） |
| `AG_DATABASE_PORT` | — | `5432` | PostgreSQL 端口 |
| `AG_DATABASE_USERNAME` | — | — | PostgreSQL 用户名 |
| `AG_DATABASE_PASSWORD` | — | — | PostgreSQL 密码 |
| `AG_DATABASE_DBNAME` | — | — | PostgreSQL 数据库名 |
| `AG_ADMIN_USERNAME` | 建议 | `admin` | 管理员账号 |
| `AG_ADMIN_PASSWORD` | 建议 | `admin` | **生产环境务必修改** |
| `AG_SERVER_SESSION_SECRET` | — | 自动生成 | 会话加密密钥 |

> 所有环境变量见 `.env.example`（项目根目录）和 `docker-compose.yml`。

### docker-compose 一键部署（PostgreSQL）

```bash
cp .env.example .env
# 编辑 .env，修改 AG_ADMIN_PASSWORD
docker compose up -d
```

### 获取 Docker 镜像

所有镜像推送到 [GitHub Container Registry (GHCR)](https://github.com/aaro-n/AI-Gateway/pkgs/container/ai-gateway)。

```bash
# 无需登录，直接拉取（GHCR 公开包）
docker pull ghcr.io/aaro-n/ai-gateway:test        # 测试版（每次 push 构建，仅 amd64）
docker pull ghcr.io/aaro-n/ai-gateway:prerelease   # 预发行通用版（amd64 + arm64）
docker pull ghcr.io/aaro-n/ai-gateway:latest       # 正式发行通用版（amd64 + arm64）
docker pull ghcr.io/aaro-n/ai-gateway:v0.0.1-rc3   # 指定版本号

# 直接运行
docker run -d -p 18080:18080 ghcr.io/aaro-n/ai-gateway:latest
```

#### 架构支持

| 镜像标签 | linux/amd64 | linux/arm64 |
|----------|:-----------:|:-----------:|
| `:test` | ✅ | ❌ |
| `:prerelease` | ✅ | ✅ |
| `:latest` | ✅ | ✅ |
| `:v*` | ✅ | ✅ |

> Docker 会自动根据你的机器架构拉取匹配的镜像层。

### 二进制文件（GitHub Release）

每次版本发布（`v*` tag）会自动构建 Windows/Linux 二进制文件并上传到 [Releases 页面](https://github.com/aaro-n/AI-Gateway/releases)。
