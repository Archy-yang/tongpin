-- 0004: 匹配、真人接触、授权审计、安全（match / contact / consent / safety 域）

-- 匹配池 opt-in
CREATE TABLE match_opt_ins (
    user_id             BIGINT PRIMARY KEY REFERENCES users(id),
    relationship_intent VARCHAR(16) NOT NULL,   -- casual|friends|serious|serious_but_slow
    age_pref_min        INT,
    age_pref_max        INT,
    accept_real_contact BOOLEAN NOT NULL DEFAULT true,
    status              VARCHAR(24) NOT NULL DEFAULT 'active',  -- active|withdrawn
    opted_in_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_optins_active ON match_opt_ins(status);

-- 契合提示 / 匹配记录（user_a < user_b 去重约定）
CREATE TABLE matches (
    id           BIGSERIAL PRIMARY KEY,
    user_a       BIGINT NOT NULL REFERENCES users(id),
    user_b       BIGINT NOT NULL REFERENCES users(id),
    score        NUMERIC(4,3) NOT NULL,
    score_detail JSONB,                     -- 各分项得分 + highlights，用于调参复盘
    status       VARCHAR(24) NOT NULL DEFAULT 'suggested',
                 -- suggested|contact_requested|in_chat|closed
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (user_a < user_b)
);
CREATE UNIQUE INDEX idx_matches_pair ON matches(user_a, user_b);
CREATE INDEX idx_matches_user ON matches(user_a, status);
CREATE INDEX idx_matches_user_b ON matches(user_b, status);

-- 真人接触请求
CREATE TABLE contact_requests (
    id         BIGSERIAL PRIMARY KEY,
    match_id   BIGINT NOT NULL REFERENCES matches(id),
    from_user  BIGINT NOT NULL REFERENCES users(id),
    to_user    BIGINT NOT NULL REFERENCES users(id),
    status     VARCHAR(24) NOT NULL DEFAULT 'pending',  -- pending|approved|rejected|revoked
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_contact_requests_pending
    ON contact_requests(match_id) WHERE status = 'pending';
CREATE INDEX idx_contact_requests_to ON contact_requests(to_user, status);
CREATE INDEX idx_contact_requests_from ON contact_requests(from_user, status);

-- 真人一对一聊天（与 AI 聊天分表：sender_id 必须是 match 参与者，无 AI 通道）
CREATE TABLE direct_messages (
    id         BIGSERIAL PRIMARY KEY,
    match_id   BIGINT NOT NULL REFERENCES matches(id),
    sender_id  BIGINT NOT NULL REFERENCES users(id),
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_dm_match ON direct_messages(match_id, id);

-- 授权审计（只追加；撤回 = 新增 granted=false 记录）
CREATE TABLE consents (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id),
    target_type VARCHAR(16) NOT NULL,       -- match_pool|real_contact
    target_id   BIGINT,                     -- real_contact 时为 match_id；match_pool 为 NULL
    granted     BOOLEAN NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_consents_user ON consents(user_id, target_type, created_at DESC);

-- 拉黑
CREATE TABLE blocks (
    blocker_id BIGINT NOT NULL REFERENCES users(id),
    blocked_id BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id)
);

-- 举报
CREATE TABLE reports (
    id          BIGSERIAL PRIMARY KEY,
    reporter_id BIGINT NOT NULL REFERENCES users(id),
    target_id   BIGINT NOT NULL REFERENCES users(id),
    reason      VARCHAR(32) NOT NULL,
    detail      TEXT,
    status      VARCHAR(16) NOT NULL DEFAULT 'pending',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
