-- 0013: 行为埋点（analytics）
-- 只记行为计数，不记内容本身（内容在各自的业务表里）。
CREATE TABLE analytics_events (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL DEFAULT 0,  -- 0 = 系统级/未知归属
    name       VARCHAR(32) NOT NULL,       -- app_open | chat_user_msg | llm_call | icebreak_create | dm_sent
    props      JSONB,                      -- 如 llm_call: {"in_tokens":N,"out_tokens":M}
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_analytics_name_time ON analytics_events (name, created_at);
CREATE INDEX idx_analytics_user_time ON analytics_events (user_id, created_at);
