#!/bin/sh
# 启动前先执行数据库迁移；失败即退出（compose 会重启重试）
set -e
echo "[entrypoint] running migrations..."
./migrate
echo "[entrypoint] starting server..."
exec ./server
