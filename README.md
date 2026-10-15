# 同频 (TongPin)

一个**渐进式社交陪伴产品**：用户先和 AI 聊天，AI 在对话中理解其沟通风格与偏好，生成风格画像；当系统发现风格契合的人时，提示用户，由用户决定是否进入真人社交。

## 核心理念

- **先单边，再双边**：先做 AI 陪伴 + 风格画像，匹配池达到规模后再开放真人匹配
- **先画像，再匹配**：用户不填标签，风格从真实对话中提取
- **先 AI 暖场，再真人接触**：AI 只做缓冲和破冰，不冒充真人，不代替用户回复
- **用户始终可选择**：可停留在 AI 陪伴，不强迫真人社交
- **隐私可撤回**：风格卡可查看、编辑、删除；匹配授权可随时撤回

详细的产品原则与 AI 接触模式定义见 [docs/product/01-product-overview.md](docs/product/01-product-overview.md)。

## 技术栈

| 层 | 选型 | 说明 |
|---|---|---|
| 后端 | Go | 单体服务，按模块划分内部包 |
| 前端 | Vue 3 + Vite | 移动端 H5 优先 |
| 数据库 | PostgreSQL 16 | 关系数据 + JSONB（风格卡）+ pgvector（风格向量） |
| LLM | 可插拔 Provider 接口 | OpenAI 兼容协议，支持 DeepSeek / GLM / Claude 等按成本切换 |
| 部署 | Docker Compose | 单机起步 |

## 目录结构

```
tongpin/
├── backend/          # Go 后端
├── frontend/         # Vue 3 前端
├── docs/             # 产品与技术文档
│   ├── product/      # 产品规划：定位、MVP 范围、风格画像、用户流程
│   ├── tech/         # 技术方案：架构、数据模型、API 设计
│   └── plan/         # 里程碑、风险与指标
└── README.md
```

## 文档索引

### 产品
- [产品概览](docs/product/01-product-overview.md) — 定位、目标用户、核心原则、AI 接触模式
- [MVP 范围](docs/product/02-mvp-scope.md) — P0 / P1 / P2 功能划分与边界
- [风格画像设计](docs/product/03-style-profile.md) — 画像维度、风格卡 JSON、分析 Prompt
- [核心用户流程](docs/product/04-user-flow.md) — 从注册到真人接触的完整链路

### 技术
- [技术架构](docs/tech/01-architecture.md) — 技术选型、服务模块、关键设计决策
- [数据模型](docs/tech/02-data-model.md) — PostgreSQL 表结构设计
- [API 设计](docs/tech/03-api-design.md) — REST API 草案

### 计划
- [里程碑](docs/plan/01-milestones.md) — 分阶段开发计划
- [风险与验收指标](docs/plan/02-risks-metrics.md) — 风险对策与目标值

## 当前状态

- [x] 后端项目骨架（Go 单体，分层与域边界见 [docs/tech/04-project-structure.md](docs/tech/04-project-structure.md)）
- [x] 用户系统：设备匿名登录 + JWT（已验证：登录 / 鉴权 / 未登录拦截）
- [x] AI 陪伴聊天（chat 域）：消息收发、历史分页、LLM 上下文组装（待配 API Key 后端到端验证 AI 回复）
- [x] 异步风格分析 + 风格卡（style 域）：worker 定时分析、风格卡 CRUD、确定性向量化（已用真实 LLM 验证：画像维度与对话内容一致）
- [x] 匹配池 opt-in + 契合提示（match 域）：硬条件过滤 + 五项加权打分 + 代码规则生成契合点文案
- [x] 真人接触 + 一对一聊天（contact 域）：双向同意、静默拒绝、请求撤回
- [x] 同意/撤回/举报/拉黑（consent + safety 域）：授权审计只追加、撤回即关闭匹配
- [x] AI 回复风格渐进对齐（chat 域）：即时镜像 + 画像对齐分档注入，AI 越聊越像你
- [x] AI 暖场（icebreak 域）：双 AI 代理按画像试聊 3 轮，生成破冰摘要（共同点/话题/雷点/开场建议）
- [x] 聊后反馈（match 域）：like / mismatch 落库，匹配质量数据闭环起步
- [x] WebSocket 实时推送（realtime 包）：真人聊天秒达，断线重连 + 轮询兜底
- [x] 前端（Vue 3）：夜聊 / 同频卡 / 遇见 / 真人聊天 / 我的 五页，移动端 H5，深色夜聊风格（开发：`cd frontend && npm run dev`）
- [x] 产品全链路 UI：聊天 → 画像 → AI 风格对齐 → 契合提示 → AI 暖场 → 双方同意 → 真人聊天 → 聊后反馈（双账号实测）
- [ ] 部署上线（Dockerfile + 云主机 + 域名）
- [ ] P1 剩余：语音、付费订阅、关系阶段引导

> 注：原计划的"阶段 0 风格分析 Spike"经决策跳过，风格分析质量随 style 域实现直接验证。
