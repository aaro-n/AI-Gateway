# 使用多阶段构建：第一阶段拉取代码并构建  
FROM golang:1.26-alpine AS builder  
  
# 安装构建所需的全套环境  
RUN apk add --no-cache \  
    git \  
    nodejs \  
    npm \  
    gcc \  
    make \  
    musl-dev  
  
# 设置工作目录  
WORKDIR /build  
  
# 将本地当前的全部源码复制到构建镜像中  
COPY . .  
  
# 初始化依赖（前端和后端）  
RUN make init  
  
# 构建前端（输出到 server/res/web/）  
RUN make build-web  
  
# 只构建 Linux 版本（跳过 Windows）  
RUN cd server && make build-linux  
  
# 第二阶段：创建精简运行镜像  
FROM alpine:latest  
  
# 安装运行时必要依赖  
RUN apk add --no-cache ca-certificates gcompat  
  
WORKDIR /app  
  
# 从构建阶段复制生成的 Linux 二进制文件  
COPY --from=builder /build/server/bin/ai-gateway-server-* /app/ai-gateway-server  
  
# 暴露端口  
EXPOSE 18080 6060  
VOLUME ["/app/data"]  
  
# 启动程序  
CMD ["/app/ai-gateway-server"]
