-- 0012: 订阅与支付（billing 域）
CREATE TABLE subscriptions (
    id                 BIGSERIAL PRIMARY KEY,
    user_id            BIGINT NOT NULL REFERENCES users(id),
    provider           VARCHAR(16) NOT NULL,   -- stripe | apple | google
    external_id        VARCHAR(255) NOT NULL,  -- 三方订阅标识（stripe subscription id 等）
    status             VARCHAR(24) NOT NULL,   -- active | past_due | incomplete | canceled
    plan               VARCHAR(32) NOT NULL,   -- member_monthly
    current_period_end TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, external_id)
);
CREATE INDEX idx_subscriptions_user ON subscriptions (user_id);

-- webhook 幂等去重：事件 ID 全局唯一，重复投递直接跳过
CREATE TABLE billing_events (
    event_id     VARCHAR(255) PRIMARY KEY,
    provider     VARCHAR(16) NOT NULL,
    payload      JSONB,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
