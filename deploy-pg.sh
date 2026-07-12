#!/bin/bash
# 杀死旧容器 + 清理数据 + docker compose 全新启动 PG
set -e
cd /home/works/ai-gateway

echo "=== 1. 杀死并删除旧容器 ==="
docker stop ai-gateway-pg 2>/dev/null || true
docker rm ai-gateway-pg 2>/dev/null || true
echo "OK"

echo "=== 2. 清理旧数据目录 ==="
sudo rm -rf pgdata
echo "OK"

echo "=== 3. Docker Compose 启动 ==="
docker compose up -d
echo "OK"

echo "=== 4. 等待 PG 就绪 ==="
sleep 3
docker compose ps

echo "=== 5. 连接测试 ==="
docker compose exec -T postgres pg_isready -U postgres -d ai_gateway

echo "=== 6. 数据库表 ==="
docker compose exec -T postgres psql -U postgres -d ai_gateway -c "\dt"

echo "=== 完成 ==="
