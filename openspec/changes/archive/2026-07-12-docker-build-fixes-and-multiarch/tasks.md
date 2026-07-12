## 1. Docker 构建修复

- [x] 1.1 SQLite CGO musl 编译错误
  - 添加 `CGO_CFLAGS="-D_LARGEFILE64_SOURCE"` 修复 `pread64`/`off64_t` 在 Alpine 上未定义
- [x] 1.2 build-and-release.yml token 错误
  - `GHP_TOKEN` → `GITHUB_TOKEN`（GitHub 自动提供）

## 2. 跨平台 Docker 镜像

- [x] 2.1 docker-test.yml: `platforms: linux/amd64` 仅 x86_64
- [x] 2.2 docker-prerelease.yml: QEMU + `linux/amd64,linux/arm64`
- [x] 2.3 docker-release.yml: QEMU + `linux/amd64,linux/arm64`

## 3. 构建优化

- [x] 3.1 .dockerignore: 排除 pgdata/、.env、node_modules、构建产物
- [x] 3.2 减小 build context 发送体积

## 4. 文档

- [x] 4.1 README.md: 快速部署章节（拉取、启动、架构表）
- [x] 4.2 web/README.md: 去掉 GHCR 登录步骤（公开包无需登录）
- [x] 4.3 v0.0.1-rc1→rc2→rc3 逐步验证和修复
