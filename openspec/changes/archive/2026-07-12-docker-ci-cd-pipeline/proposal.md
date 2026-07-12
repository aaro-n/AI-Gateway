## Why

项目缺乏容器化部署方案和自动化镜像构建流程：
1. Dockerfile 使用不存在的 Go 1.26 版本，无依赖缓存，以 root 运行
2. 无 GitHub Actions 自动构建 Docker 镜像
3. 原有 docker-compose.yml 仅含 PostgreSQL，没有应用服务
4. 环境变量未文档化，用户不清楚如何配置

## What Changes

### Dockerfile 重写
- 修正 Go 版本：`golang:1.26-alpine` → `golang:1.24-alpine`
- 多阶段构建优化：先复制依赖文件利用 Docker 缓存，代码变更无需重装 npm/go 依赖
- 安全加固：创建 `appuser` 非 root 用户运行
- 二进制优化：`-s -w` 去除调试符号，减小 ~30% 体积
- 移除 config.yaml 复制：全部配置通过 `AG_*` 环境变量提供
- 添加 `ARG VERSION` 支持版本注入

### CI/CD 镜像流水线（3 个 workflow）
- `docker-test.yml`：每次 push 自动构建 `:test` 镜像
- `docker-prerelease.yml`：Pre-Release 发布时构建 `:prerelease` + `:vX.Y.Z-rcN` 镜像
- `docker-release.yml`：正式 Release 发布时构建 `:latest` + `:vX.Y.Z` 镜像

### build-and-release.yml 增强
- 自动检测 tag 后缀（`-rc`/`-beta`/`-alpha`/`-pre`）设置 `prerelease: true`

### docker-compose.yml 完善
- 新增 `app` 服务：引用 GHCR 镜像，配置全部 `AG_*` 环境变量
- 添加 healthcheck（`/api/v1/health`）
- `depends_on postgres: service_healthy`

### 环境变量文档化
- `.env.example`：列出全部 20+ 环境变量及默认值
- `web/README.md`：最小部署指南 + 生产必设变量表

### 开发工具
- `server/cmd/dumpdb/`：数据库导出工具
- `server/cmd/dump_providers/`：提供商配置导出
- `server/cmd/querydb/`：数据库查询工具
