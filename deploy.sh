#!/usr/bin/env bash
# 同频一键部署脚本：在目标主机上执行（需要 docker + docker compose 插件）
# 用法：
#   ./deploy.sh          # 构建并启动/更新全部服务
#   ./deploy.sh logs     # 跟踪日志
#   ./deploy.sh backup   # 备份数据库到 ./backups/
set -euo pipefail
cd "$(dirname "$0")"

COMPOSE="docker compose -f docker-compose.prod.yml"

case "${1:-up}" in
  up)
    if [[ ! -f .env ]]; then
      echo "错误：缺少 .env 文件。先执行：cp .env.example .env 并填好配置"
      echo "必填：LLM_API_KEY；务必修改：JWT_SECRET（可用 openssl rand -hex 32 生成）"
      exit 1
    fi
    echo "==> 构建镜像"
    $COMPOSE build
    echo "==> 启动服务（含自动数据库迁移）"
    $COMPOSE up -d
    echo "==> 等待健康检查"
    sleep 5
    $COMPOSE ps
    echo
    echo "部署完成。访问 http://<主机IP> 即可使用。"
    echo "查看日志：./deploy.sh logs"
    ;;
  logs)
    $COMPOSE logs -f --tail=100
    ;;
  backup)
    mkdir -p backups
    docker exec tongpin-postgres pg_dump -U "${POSTGRES_USER:-tongpin}" "${POSTGRES_DB:-tongpin}" \
      | gzip > "backups/tongpin_$(date +%Y%m%d_%H%M%S).sql.gz"
    echo "已备份到 backups/"
    ;;
  *)
    echo "用法：./deploy.sh [up|logs|backup]"
    exit 1
    ;;
esac
