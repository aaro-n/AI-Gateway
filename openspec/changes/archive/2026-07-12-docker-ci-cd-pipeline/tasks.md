## 1. Dockerfile 重写

- [x] 1.1 修正 Go 版本 1.26 → 1.24
- [x] 1.2 多阶段构建：golang:1.24-alpine builder + alpine:latest runtime
- [x] 1.3 依赖缓存层（先 COPY package.json/go.mod，再 COPY 源码）
- [x] 1.4 非 root 用户（appuser）
- [x] 1.5 二进制优化（-s -w ldflags）
- [x] 1.6 移除 config.yaml，纯环境变量运行

## 2. CI/CD 镜像流水线

- [x] 2.1 docker-test.yml：push 触发 → `:test` 标签
- [x] 2.2 docker-prerelease.yml：Pre-Release 触发 → `:prerelease` + `:{version}` 标签
- [x] 2.3 docker-release.yml：Release 触发 → `:latest` + `:{version}` 标签
- [x] 2.4 build-and-release.yml：自动检测 pre-release tag 后缀

## 3. docker-compose.yml 完善

- [x] 3.1 新增 app 服务（GHCR 镜像 + AG_* 环境变量）
- [x] 3.2 healthcheck（wget /api/v1/health）
- [x] 3.3 depends_on postgres: service_healthy
- [x] 3.4 数据卷挂载（./data:/app/data）

## 4. 文档

- [x] 4.1 .env.example：全部环境变量及默认值
- [x] 4.2 web/README.md：最小部署指南 + 版本拉取说明
