# ============================================================
# 多阶段构建 Dockerfile
# 阶段1: 构建前端 + 编译 Go 后端（嵌入前端资源）
# 阶段2: 精简运行时镜像
# ============================================================

# ---- 阶段1: 构建 ----
FROM golang:1.24-alpine AS builder

# 构建参数
ARG VERSION=dev
ARG TZ=Asia/Shanghai

# 安装构建依赖
# gcc + musl-dev: CGO_ENABLED=1 编译所需（SQLite 依赖 CGO）
# nodejs + npm: 前端构建
RUN apk add --no-cache \
    git \
    nodejs \
    npm \
    gcc \
    make \
    musl-dev \
    tzdata

# 设置工作目录
WORKDIR /build

# 先复制依赖文件，利用 Docker 缓存层
COPY web/package.json web/package-lock.json* ./web/
COPY server/go.mod server/go.sum ./server/

# 安装依赖（此层可缓存，代码变更无需重装）
RUN cd web && npm install
RUN cd server && go mod download

# 复制全部源码
COPY . .

# 构建前端（输出到 server/res/web/，供 Go embed 嵌入）
RUN make build-web VERSION=${VERSION}

# 构建 Linux 二进制（CGO_ENABLED=1，嵌入前端资源）
RUN cd server && CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
    go build -ldflags "-X ai-gateway/res.Version=${VERSION} -s -w" \
    -o bin/ai-gateway-server -v ./cmd/server/main.go

# ---- 阶段2: 运行时 ----
FROM alpine:latest

# 运行时依赖
# ca-certificates: HTTPS/TLS 证书验证
# gcompat: glibc 兼容层（部分 CGO 依赖需要）
# tzdata: 时区支持
RUN apk add --no-cache ca-certificates gcompat tzdata

# 创建非 root 用户
RUN adduser -D -h /app -s /sbin/nologin appuser

WORKDIR /app

# 从构建阶段复制二进制
COPY --from=builder /build/server/bin/ai-gateway-server /app/ai-gateway-server

# 注：无需 config.yaml，全部配置通过环境变量提供（AG_ 前缀）
# 详见 server/config.yaml.example 中的环境变量说明

# 设置文件权限
RUN chown -R appuser:appuser /app

# 切换到非 root 用户
USER appuser

# 暴露端口
# 18080: API 网关端口
# 6060:  pprof 性能分析端口
EXPOSE 18080 6060

# 数据持久化卷
VOLUME ["/app/data"]

# 启动
CMD ["/app/ai-gateway-server"]
