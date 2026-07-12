## Why

v0.0.1-rc1 Docker 构建在 CI 中失败，需要修复并增强。

## What Changes

### Bug Fixes
- Dockerfile: 添加 `CGO_CFLAGS="-D_LARGEFILE64_SOURCE"` 修复 go-sqlite3 在 Alpine musl 上的 `pread64`/`pwrite64`/`off64_t` 未定义错误
- build-and-release.yml: token `GHP_TOKEN` → `GITHUB_TOKEN`，修复 Release 创建权限

### 多架构支持
- docker-test.yml: 限制 `platforms: linux/amd64`（测试版仅 x86_64）
- docker-prerelease.yml: 新增 QEMU + `platforms: linux/amd64,linux/arm64`
- docker-release.yml: 新增 QEMU + `platforms: linux/amd64,linux/arm64`

### 部署文档
- .dockerignore: 排除 pgdata/、.env、node_modules、构建产物
- README.md: 新增快速部署章节（GHCR 拉取命令、零配置启动、架构支持表）
- web/README.md: 更新 Docker 拉取说明（去掉不必要的 GHCR 登录）
