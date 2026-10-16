# API 设计

> REST + JSON。认证用 JWT（`Authorization: Bearer <token>`）。所有响应统一包装：
>
> ```json
> { "code": 0, "msg": "ok", "data": { } }
> ```
>
> 错误码：0 成功；4xx 业务错误（如 1001 参数错误、2001 未授权、3001 未进入匹配池）；5xx 服务错误。

## 认证

```
POST /api/v1/auth/device          # 设备匿名注册/登录（MVP 主路径）
  body: { "device_id": "..." }
  resp: { "token": "...", "user": { "id", "nickname", "is_new" } }

POST /api/v1/auth/phone           # 绑定/登录手机号（合规要求，MVP 可后置）
  body: { "phone": "...", "code": "..." }
```

## 用户

```
GET  /api/v1/me                   # 当前用户信息
PUT  /api/v1/me                   # 更新基础设置
  body: { "nickname"?, "age_range"?, "gender"?, "city"?, "gender_pref"? }
```

## AI 陪伴聊天

```
POST /api/v1/chat/messages        # 发送消息，返回 AI 回复（同步）
  body: { "content": "..." }
  resp: { "user_message": {...}, "ai_message": {...} }   # ai_message 带 ai 标识字段

GET  /api/v1/chat/messages?before=<cursor>&limit=50   # 历史（游标分页）
```

- MVP 用同步请求-响应（发一条等一条），前端渲染 AI 消息必须带 AI 标识
- WebSocket 推送为 P1 优化，接口预留 `/api/v1/ws`

## 风格卡

```
GET     /api/v1/style-profile             # 当前风格卡（未生成时返回 status: 'collecting' + message_count）
  resp: { "status": "ready", "profile": {...}, "confidence": 0.78, "version": 3, "message_count": 120 }

PUT     /api/v1/style-profile             # 编辑（仅展示层，存储于 style_display_overrides，不影响匹配）
  body: { "display_overrides": { ... } }   # 顶层字段覆盖，需合法 JSON，<= 8KB

DELETE  /api/v1/style-profile             # 删除（物理删除全部版本+向量，置 suppressed 停止分析）

POST    /api/v1/style-profile/resume      # 删除后重新开启分析
```

## 匹配

```
POST /api/v1/match/opt-in          # 进入匹配池
  body: {
    "relationship_intent": "serious",
    "age_pref": { "min": 22, "max": 30 },
    "accept_real_contact": true
  }

PUT  /api/v1/match/opt-in          # 更新 / 暂停（status: paused）
DELETE /api/v1/match/opt-in        # 退出匹配池（撤回授权）

GET  /api/v1/match/suggestions     # 契合提示（每次最多 3 条）
  resp: { "suggestions": [
    { "match_id": 42, "score": 0.82,
      "highlights": ["你们都偏好深夜慢节奏的深度对话", "两人的幽默风格都是冷幽默型"],
      "created_at": "..." }
  ]}
  # 匹配池人数 < 500 时返回 { "suggestions": [], "pool_status": "warming_up" }
```

## 真人接触

```
POST /api/v1/contact/requests               # 对某条契合提示发起真人接触
  body: { "match_id": 42 }
  resp: { "id": 7, "match_id": 42, "from_user": 1, "to_user": 2, "status": "pending", ... }

POST /api/v1/contact/requests/{id}/approve  # 同意（被请求方）→ 双方进入真人聊天
POST /api/v1/contact/requests/{id}/reject   # 拒绝（静默，不通知请求方）→ 匹配关闭
POST /api/v1/contact/requests/{id}/revoke   # 撤回（请求方，未决时）→ 匹配回到 suggested

GET  /api/v1/contact/requests?box=inbox|outbox   # 收到的/发出的请求列表
```

## 真人聊天

```
GET  /api/v1/dm/{match_id}/messages?before=<cursor>&limit=50
POST /api/v1/dm/{match_id}/messages
  body: { "content": "..." }
  # 仅双方 approved 后可用；sender 只能是当前用户（AI 无写入通道）
```

- MVP 短轮询（3-5s）；WebSocket 为 P1

## 授权与审计

```
GET  /api/v1/consents              # 我的授权记录（审计可见）
POST /api/v1/consents/revoke
  body: { "target_type": "real_contact", "target_id": 42 }   # target_id = match_id，撤回即关闭该匹配
  # match_pool 撤回请使用 DELETE /api/v1/match/opt-in
```

## 安全

```
POST /api/v1/reports
  body: { "target_id": 88, "reason": "harassment", "detail": "..." }

POST /api/v1/blocks
  body: { "user_id": 88 }
  # 拉黑后：互相不出现在契合提示，无法互发消息
```

## 订阅与支付（billing）

```
POST /api/v1/billing/checkout          # 登录态；建 Stripe Checkout 会话
  → { "url": "https://checkout.stripe.com/..." }   # 前端整页跳转
  # 未配置支付密钥时返回 3001「支付暂未开放」

GET  /api/v1/billing/subscription      # 登录态；会员卡状态
  → { "active": true, "plan": "member_monthly", "status": "active",
      "current_period_end": "2026-11-15T19:44:16+08:00" }

POST /api/v1/billing/webhook/:provider # 公开路由：验签即鉴权（Stripe-Signature HMAC-SHA256）
  # 事件归一化为 subscription_upserted / subscription_canceled，
  # billing_events 表幂等去重（三方会重投）；处理失败返回 500 让三方重试
```

- 分层：billing 域只认 `PaymentProvider` 接口；渠道适配器在 `internal/payments/`（stripe 已实现，apple/google 预留），装配层按 `STRIPE_*` 环境变量注册
- 会员权益经 `chat.EntitlementProvider` 消费方接口接入聊天额度（会员不限量），chat 不感知 billing
- 环境变量：`STRIPE_SECRET_KEY`、`STRIPE_WEBHOOK_SECRET`、`STRIPE_PRICE_MONTHLY`、`PUBLIC_BASE_URL`

## WebSocket 实时推送

```
GET /api/v1/ws?token=<jwt>        # 浏览器 WS 无法带 Authorization 头，token 走查询参数
```

- 单向推送通道：客户端不发送业务数据（业务写操作仍走 REST）
- 帧格式：`{ "type": "dm", "match_id": 42, "message": { ... } }`
- 心跳：服务端 30s ping，60s 无 pong 断开；客户端断线 3s 自动重连
- 客户端保留低频轮询兜底，WS 在线时降频
- 推送是尽力而为：离线用户靠拉取接口补齐，不丢数据

## 接口约束

1. 所有写接口做速率限制（聊天 30 条/分钟，匹配 opt-in 1 次/小时）
2. 真人消息发送前过内容审核管道（safety 模块），未过审不入库
3. 分页统一游标式（`before` + `limit`，最大 50），不用 offset
4. 所有涉及他人信息的响应只暴露匿名昵称与风格摘要，不暴露风格卡原始 JSON
