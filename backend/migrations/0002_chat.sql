-- 0002: AI 陪伴聊天（chat 域）
CREATE TABLE conversations (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id),
    ai_persona_id  VARCHAR(32) NOT NULL DEFAULT 'default',
    memory_summary TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_conversations_user ON conversations(user_id);

-- sender 只有 'user' | 'ai'，AI 标识入库即确定
CREATE TABLE messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id),
    sender          VARCHAR(8) NOT NULL CHECK (sender IN ('user', 'ai')),
    content         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_messages_conv ON messages(conversation_id, id);
