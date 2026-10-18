# 部署手册

> 首次真实构建验证（2026-10-18）补充：Dockerfile 已固定 `golang:1.26-alpine` 并内置
> `GOPROXY=https://goproxy.cn`（国内网络必需）；compose 的 build 已显式指向
> `docker/Dockerfile` 子路径。基础镜像拉不动时先走镜像源 retag：
> `docker pull docker.m.daocloud.io/library/golang:1.26-alpine && docker tag docker.m.daocloud.io/library/golang:1.26-alpine golang:1.26-alpine`

## 架构

```
用户浏览器
   ↓ HTTP :80 / WSS
[frontend]  nginx 容器：托管 Vue 静态文件，/api 反代（含 WebSocket 升级）
   ↓
[backend]   Go 单体容器：启动时自动跑数据库迁移
   ↓
[postgres]  pgvector 镜像，数据卷 tongpin_pgdata 持久化
```

单机 Docker Compose 三容器，无外部依赖（无 Redis / 消息队列 / 独立向量库）。

## 首次部署

前提：主机装有 Docker 与 docker compose 插件，80 端口可用。

```bash
# 1. 拿到代码
git clone https://github.com/Archy-yang/tongpin.git && cd tongpin

# 2. 配置
cp .env.example .env
# 必改项：
#   LLM_API_KEY=...                        # 千帆/OpenAI 兼容 Key
#   JWT_SECRET=$(openssl rand -hex 32)     # 必须换掉默认值
# 可选项：
#   POSTGRES_PASSWORD=强密码               # 仅本机回环访问，但建议仍改掉
#   HTTP_PORT=80                           # 对外端口
#   MATCH_POOL_THRESHOLD=500               # 契合提示灰度阈值

# 3. 一键部署（构建 + 启动 + 自动迁移）
chmod +x deploy.sh && ./deploy.sh
```

完成后访问 `http://<主机IP>` 即可。

## 日常运维

```bash
./deploy.sh            # 代码更新后重新构建并滚动更新
./deploy.sh logs       # 跟踪全部日志
./deploy.sh backup     # 数据库备份到 ./backups/*.sql.gz（建议加 crontab 每日一次）
docker compose -f docker-compose.prod.yml down   # 停止（数据卷保留）
```

crontab 备份示例（每日 4:05）：

```
5 4 * * * cd /path/to/tongpin && ./deploy.sh backup
```

## 上线前检查单

- [ ] `.env` 中 `JWT_SECRET` 已更换、`APP_ENV=prod`（compose 强制）
- [ ] `MATCH_POOL_THRESHOLD` 恢复 500（开发期可能调低过）
- [ ] 合规：ICP 备案（域名 + 服务器在国内）、生成式 AI 相关评估（使用已备案三方模型 API）
- [ ] HTTPS：套一层 Caddy / certbot / 云负载均衡终止 TLS；nginx 容器只暴露 80
- [ ] `deploy.sh backup` 已配置定时任务
- [ ] 首次内测前把匹配池阈值、worker 间隔按真实流量确认一遍

## 已知边界（上线后按需处理）

- 单机部署：数据库容器与备份在同一台主机；上量后把 postgres 拆到独立实例/云数据库
- TLS 未内置：在 compose 前加一层反代（推荐 Caddy，两行配置自动 HTTPS）
- 镜像构建需要访问 Docker Hub / Go 模块代理 / npm 源；国内主机建议配置镜像加速与 `GOPROXY=https://goproxy.cn,direct`
- 日志直接写 stdout，由 docker 日志驱动管理；上量后接集中日志
