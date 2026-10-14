-- 0003: 风格画像（style 域）
CREATE EXTENSION IF NOT EXISTS vector;

-- embedding 维度 64 必须与 internal/domain/style/embedding.go 的 embeddingDim 一致
CREATE TABLE style_profiles (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id),
    version    INT NOT NULL,
    profile    JSONB NOT NULL,
    confidence NUMERIC(3,2) NOT NULL,
    embedding  VECTOR(64),
    source     VARCHAR(16) NOT NULL DEFAULT 'analysis',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, version)
);
CREATE INDEX idx_profiles_latest ON style_profiles(user_id, version DESC);

-- 用户编辑仅影响展示层，匹配永远用 analysis 版本（见 docs/product/02-mvp-scope.md 决策记录）
CREATE TABLE style_display_overrides (
    user_id    BIGINT PRIMARY KEY REFERENCES users(id),
    overrides  JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 风格分析进度状态（替代任务表：worker 直接按"上次分析位置之后的新消息数"发现候选用户）
CREATE TABLE style_analysis_state (
    user_id         BIGINT PRIMARY KEY REFERENCES users(id),
    last_message_id BIGINT NOT NULL DEFAULT 0,
    suppressed      BOOLEAN NOT NULL DEFAULT false,  -- 用户删除风格卡后置 true，停止分析
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
