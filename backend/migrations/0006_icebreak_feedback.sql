-- 0006: AI 暖场（icebreak 域）+ 聊后反馈（match 域）

-- AI 暖场会话：双方同意后，两个 AI 代理以其用户画像为性格聊 3 轮，产出破冰摘要
CREATE TABLE ai_icebreak_sessions (
    id          BIGSERIAL PRIMARY KEY,
    match_id    BIGINT NOT NULL REFERENCES matches(id),
    from_user   BIGINT NOT NULL REFERENCES users(id),   -- 发起方
    to_user     BIGINT NOT NULL REFERENCES users(id),   -- 同意方
    status      VARCHAR(16) NOT NULL DEFAULT 'pending',
                -- pending|generating|ready|declined|failed
    transcript  JSONB,                                  -- 双 AI 对话记录 [{side,text}]
    summary     JSONB,                                  -- 破冰摘要（见 docs/product）
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_icebreak_match ON ai_icebreak_sessions(match_id);
CREATE INDEX idx_icebreak_to ON ai_icebreak_sessions(to_user, status);

-- 聊后反馈：匹配质量数据闭环（每人每匹配一条，可覆盖更新）
CREATE TABLE conversation_feedback (
    id         BIGSERIAL PRIMARY KEY,
    match_id   BIGINT NOT NULL REFERENCES matches(id),
    user_id    BIGINT NOT NULL REFERENCES users(id),
    direction  VARCHAR(8) NOT NULL CHECK (direction IN ('like', 'mismatch')),
    note       TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (match_id, user_id)
);
