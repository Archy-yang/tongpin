# 数据模型（PostgreSQL）

> 需启用扩展：`pgvector`。所有表 `created_at`/`updated_at` 默认 `now()`。逻辑删除统一用 `deleted_at`（风格卡删除是物理删除，见下）。

## 表结构

```sql
CREATE EXTENSION IF NOT EXISTS vector;

-- 用户
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    phone         VARCHAR(20) UNIQUE,              -- 手机号登录（合规要求，MVP 可先设备匿名+后绑手机）
    device_id     VARCHAR(64) UNIQUE,              -- 设备匿名登录
    nickname      VARCHAR(32) NOT NULL,            -- 匿名昵称，系统生成可改
    age_range     VARCHAR(10),                     -- '18-24','25-30','31-40','40+'
    gender        VARCHAR(10),                     -- 'male','female','other','unset'
    city          VARCHAR(32),
    gender_pref   VARCHAR(10),                     -- 匹配性别偏好
    status        VARCHAR(16) NOT NULL DEFAULT 'active',  -- active|banned
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- AI 陪伴会话
CREATE TABLE conversations (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id),
    ai_persona_id   VARCHAR(32) NOT NULL DEFAULT 'default',
    memory_summary  TEXT,                           -- 跨会话长期记忆摘要（异步生成）
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 消息（AI 陪伴）
CREATE TABLE messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id),
    sender          VARCHAR(8) NOT NULL,            -- 'user' | 'ai'（AI 标识技术强制）
    content         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_messages_conv ON messages(conversation_id, created_at);

-- 风格分析状态（实现时替代了原设计的任务表：worker 直接按
-- "上次分析位置之后的新消息数 >= 20" 发现候选用户，无需任务队列）
CREATE TABLE style_analysis_state (
    user_id         BIGINT PRIMARY KEY REFERENCES users(id),
    last_message_id BIGINT NOT NULL DEFAULT 0,
    suppressed      BOOLEAN NOT NULL DEFAULT false,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 用户对风格卡的展示层编辑（不影响匹配，匹配只用 analysis 版本）
CREATE TABLE style_display_overrides (
    user_id    BIGINT PRIMARY KEY REFERENCES users(id),
    overrides  JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 风格卡（多版本，历史可回溯）
CREATE TABLE style_profiles (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id),
    version      INT NOT NULL,
    profile      JSONB NOT NULL,                   -- 风格卡 JSON（白名单字段校验后入库）
    confidence   NUMERIC(3,2) NOT NULL,
    embedding    VECTOR(64),                       -- 匹配向量（代码从 profile 计算）
    source       VARCHAR(16) NOT NULL DEFAULT 'analysis', -- analysis|display_edit(仅展示层)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, version)
);
-- 用户"删除风格卡"= 删除该用户全部 style_profiles + 停止分析任务（物理删除，隐私要求）
CREATE INDEX idx_profiles_latest ON style_profiles(user_id, version DESC);

-- 匹配池 opt-in
CREATE TABLE match_opt_ins (
    user_id             BIGINT PRIMARY KEY REFERENCES users(id),
    relationship_intent VARCHAR(16) NOT NULL,       -- casual|friends|serious
    age_pref_min        INT,
    age_pref_max        INT,
    accept_real_contact BOOLEAN NOT NULL DEFAULT true,
    status              VARCHAR(16) NOT NULL DEFAULT 'active',  -- active|paused|withdrawn
    opted_in_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 契合提示 / 匹配记录
CREATE TABLE matches (
    id            BIGSERIAL PRIMARY KEY,
    user_a        BIGINT NOT NULL REFERENCES users(id),
    user_b        BIGINT NOT NULL REFERENCES users(id),
    score         NUMERIC(4,3) NOT NULL,
    score_detail  JSONB,                            -- 各分项明细，便于调参复盘
    status        VARCHAR(16) NOT NULL DEFAULT 'suggested',
                  -- suggested|dismissed_a|dismissed_b|contact_requested|in_chat|closed
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (user_a < user_b)                        -- 去重约定：总是小 id 在前
);
CREATE UNIQUE INDEX idx_matches_pair ON matches(user_a, user_b);

-- 真人接触请求
CREATE TABLE contact_requests (
    id          BIGSERIAL PRIMARY KEY,
    match_id    BIGINT NOT NULL REFERENCES matches(id),
    from_user   BIGINT NOT NULL REFERENCES users(id),
    to_user     BIGINT NOT NULL REFERENCES users(id),
    status      VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending|approved|rejected|expired|revoked
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 真人一对一聊天（与 AI 聊天物理分表，AI 无发送通道）
CREATE TABLE direct_messages (
    id            BIGSERIAL PRIMARY KEY,
    match_id      BIGINT NOT NULL REFERENCES matches(id),
    sender_id     BIGINT NOT NULL REFERENCES users(id),
    content       TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_dm_match ON direct_messages(match_id, created_at);

-- 授权记录（审计用，只追加不修改）
CREATE TABLE consents (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id),
    target_type VARCHAR(16) NOT NULL,              -- 'match_pool'|'real_contact'|'ai_icebreak'(P1)
    target_id   BIGINT,                            -- 具体对象（如 match_id），池级授权为 NULL
    granted     BOOLEAN NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_consents_user ON consents(user_id, target_type, created_at DESC);

-- 举报
CREATE TABLE reports (
    id          BIGSERIAL PRIMARY KEY,
    reporter_id BIGINT NOT NULL REFERENCES users(id),
    target_id   BIGINT NOT NULL REFERENCES users(id),
    reason      VARCHAR(32) NOT NULL,
    detail      TEXT,
    status      VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending|resolved|dismissed
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 拉黑
CREATE TABLE blocks (
    blocker_id BIGINT NOT NULL REFERENCES users(id),
    blocked_id BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id)
);
```

## 设计要点

1. **AI 会话与真人会话分表**（`messages` vs `direct_messages`）：技术层面保证 AI 无法向真人会话写入消息（模式 E 红线）
2. **matches 约束 `user_a < user_b`**：同一对人只有一条匹配记录，状态机单向推进
3. **consents 只追加**：撤回 = 新增一条 `granted=false` 记录，完整审计链
4. **风格卡物理删除**：隐私承诺"删除即删除"，不做软删除
5. **embedding 64 维**：由代码从风格卡 JSONB 确定性计算（枚举 one-hot + 数值归一化 + 多标签编码），风格卡版本更新时同步重算
6. **score_detail JSONB**：存各分项得分，用于匹配效果复盘与公式调参
