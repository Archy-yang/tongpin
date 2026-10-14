# 技术架构

## 技术选型

| 层 | 选型 | 理由 |
|---|---|---|
| 后端语言 | Go 1.22+ | 单二进制部署、并发模型适合长连接聊天、个人开发者生态成熟 |
| Web 框架 | Gin | 生态最大、中间件丰富、文档多 |
| 前端 | Vue 3 + Vite + Pinia + Vue Router | 移动端 H5 优先；MVP 不做原生 App |
| 数据库 | PostgreSQL 16 | JSONB（风格卡）+ pgvector（匹配向量）+ 关系数据，一库三用，省掉独立向量库 |
| 缓存 | 暂不引入 Redis | MVP 用 PostgreSQL + 进程内状态够用；需要会话缓存时再加 |
| 异步任务 | 进程内 worker（goroutine + DB 任务表） | 风格分析是低频批量任务，无需消息队列；量大后迁移 asynq |
| LLM | OpenAI 兼容协议的 Provider 接口 | 可配置切换 DeepSeek / GLM / Claude，按成本选择；MVP 阶段分析任务可用便宜模型 |
| 实时通信 | WebSocket（gorilla/websocket 或 melody） | 聊天场景需要；降级方案为短轮询 |
| 内容审核 | 第三方 API（如数美、网易易盾）+ 规则引擎 | 接口抽象，MVP 可先用规则引擎 + 人工抽检 |
| 部署 | Docker Compose 单机 | backend + frontend(nginx) + postgres 三容器 |

## 单体架构与模块划分

MVP 阶段是**单体服务**，按领域划分内部包（不拆微服务），包间只通过接口依赖：

```
backend/
├── cmd/server/main.go        # 入口：路由装配、worker 启动
├── internal/
│   ├── auth/                 # 注册、登录、JWT、匿名昵称生成
│   ├── chat/                 # AI 陪伴聊天：会话、消息、LLM 调用、记忆
│   ├── style/                # 风格分析：任务调度、Prompt 组装、结果校验入库
│   ├── match/                # 匹配池 opt-in、硬条件过滤、向量相似、契合提示
│   ├── contact/              # 真人接触：请求/同意/拒绝、一对一文本会话
│   ├── consent/              # 授权与撤回：授权记录、审计日志
│   ├── safety/               # 举报、拉黑、审核钩子、AI 标识
│   └── llm/                  # LLM Provider 接口 + 各厂商实现
├── pkg/                      # 可复用工具（配置、日志、校验）
├── migrations/               # SQL 迁移文件（golang-migrate）
└── docker/
```

各模块职责（一个模块回答三个问题：做什么、怎么用、依赖谁）：

| 模块 | 做什么 | 对外接口 | 依赖 |
|---|---|---|---|
| auth | 身份认证 | HTTP handler | DB |
| chat | 管理 AI 陪伴会话与消息，调用 LLM 生成回复 | HTTP handler、Service | llm、DB、style（读取画像用于人格调优，可选） |
| style | 异步分析消息生成/更新风格卡与向量 | Service（被 worker 调用）、HTTP handler | llm、DB |
| match | opt-in 管理、候选过滤、打分、生成契合提示 | HTTP handler、Service | DB（pgvector） |
| contact | 真人接触请求与一对一聊天 | HTTP handler、WS | consent、safety、DB |
| consent | 授权授予/撤回的记录与查询，供其他模块校验 | Service | DB |
| safety | 内容审核管道（发消息必经）、举报拉黑 | Service、HTTP handler | DB、审核 API |
| llm | 统一的 LLM 调用接口 | `Complete(ctx, req) (resp, error)` | 外部 API |

## 关键设计决策

### 1. 风格分析是异步批任务，不在聊天链路上

- 聊天接口同步做：存消息、（规则引擎）审核、调 LLM 生成回复
- worker 定时扫 `style_analysis_jobs` 任务表（或按"用户新消息数 ≥ 20"触发），逐用户分析
- 好处：聊天延迟不受分析影响；LLM 挂了只影响画像更新，不影响聊天可用性

### 2. LLM Provider 接口化

```go
type Provider interface {
    Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}
```

- 聊天用质量较好的模型，风格分析用便宜模型，通过配置指定
- Prompt 模板从代码里分离为文本文件，便于迭代测试

### 3. 匹配打分在应用层完成

```
总分 = 0.45 * 风格相似度(pgvector 余弦)
     + 0.20 * 关系意图一致
     + 0.15 * 活跃时段重叠
     + 0.10 * 地理接近
     + 0.10 * 互补奖励(部分维度差异加分)
```

- pgvector 只负责召回 top-N（`<=>` 余弦距离排序），精细打分在 Go 内存中算——分数公式会频繁调整，放代码里而不是 SQL 里
- 互补奖励需要读对方风格卡字段，召回后批量取 JSONB 即可，MVP 数据量无性能问题

### 4. AI 标识是技术强制，不是产品约定

- 所有 AI 生成内容入库时带 `sender='ai'`，前端渲染 AI 消息必须带标识样式
- 真人会话接口（contact 模块）与 AI 会话接口（chat 模块）物理分离，真人会话的消息 sender 只能是 user，无 AI 发送通道

### 5. 配置与环境

- 环境变量 + `.env`，用 viper 或标准库加载
- 密钥（LLM API Key、JWT Secret）不入库不入 git

## 可扩展路径（不是现在做，只留接口）

- 匹配池扩大后：召回下沉到 pgvector 的 HNSW 索引
- 需要实时推送匹配提示：加 Redis pub/sub 或 SSE
- 需要语音：chat 模块加 ASR/TTS 通道，架构不变
- 单体撑不住：按 internal 包边界拆服务，接口层已隔离
