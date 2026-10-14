# 后端项目结构与分层规范

## 目录结构

```
backend/
├── cmd/
│   ├── server/main.go            # 入口：配置 → DB → 装配 → 启动
│   └── migrate/main.go           # 极简迁移执行器（按文件序执行 migrations/*.sql）
├── internal/
│   ├── config/                   # 环境变量配置加载
│   ├── server/                   # 装配层：依赖注入、路由注册（唯一知道所有域的地方）
│   ├── platform/                 # 基础设施，不含业务逻辑，不依赖任何 domain
│   │   ├── database/             # pgx 连接池
│   │   ├── jwt/                  # 令牌签发/校验
│   │   ├── httpx/                # 统一响应格式与业务错误码
│   │   └── middleware/           # Auth 等通用中间件
│   └── domain/                   # 业务域（核心）
│       ├── auth/                 # 认证与用户
│       ├── chat/                 # AI 陪伴聊天
│       ├── style/                # 风格分析与风格卡
│       ├── match/                # 匹配池与契合提示
│       ├── contact/              # 真人接触与一对一聊天
│       ├── consent/              # 授权与撤回审计
│       ├── safety/               # 举报、拉黑、内容审核
│       └── llm/                  # LLM Provider 接口与实现
├── migrations/                   # SQL 迁移，NNNN_描述.sql，按序执行
├── Makefile
└── go.mod
```

## 域内分层（每个 domain 包统一三件套）

```
domain/<域>/
├── model.go        # 领域模型 + 请求/响应 DTO
├── repository.go   # 数据访问：只写 SQL，返回领域模型
├── service.go      # 业务逻辑：领域规则、跨 repo 编排
└── handler.go      # HTTP 层：参数绑定、调用 service、响应包装（不含业务逻辑）
```

- handler 的 `RegisterPublicRoutes(rg)` / `RegisterRoutes(rg)` 是统一的路由注册入口
- service 只依赖本域 repository + platform 组件 + 其他域的接口（见下）

## 边界规则（包结构强制）

1. **按域分包，不按层分包**：一个域的所有层都在同一个包内，域内自洽；改一个功能不跨五个目录
2. **跨域依赖只走接口**：域 A 需要域 B 的能力时，A 自己定义所需的最小接口，由 `server` 装配层注入 B 的实现；**禁止 A 直接 import B 的 repository 或 struct**
3. **依赖方向单向**：
   - `platform` 不 import 任何 `domain`
   - `domain` 不 import `server`（装配层）
   - `cmd` 只 import `config` / `platform` / `server`
4. **AI 会话与真人会话物理隔离**：`chat` 与 `contact` 互不 import，各自独立的数据表；真人消息表没有 AI 写入通道（产品红线的技术保证）
5. **装配唯一入口**：所有跨域接线只发生在 `internal/server/server.go`，新域上线 = 写域代码 + 在 server.go 加几行装配

## 新增一个业务域的步骤

1. 建包 `internal/domain/<name>/`，写 model / repository / service / handler 四件套
2. 需要的表加进 `migrations/NNNN_xxx.sql`
3. 在 `server.go` 的 `registerRoutes` 里构造 repo → service → handler，注册路由
4. 若被其他域依赖：其他域定义自己的最小接口，这里提供实现注入

## 约定

- 错误码：0 成功；1xxx 参数；2xxx 认证/权限；3xxx 业务规则；5xxx 服务错误（见 `httpx`）
- 分页统一游标式（`before` + `limit`）
- SQL 全部手写在 repository 内，不引入 ORM；迁移不引入 golang-migrate，用 `cmd/migrate` 极简执行器
- 配置只走环境变量（`.env.example` 为模板），密钥不入库
- 模块路径 `github.com/Archy-yang/tongpin/backend`（仓库 github.com/Archy-yang/tongpin）
