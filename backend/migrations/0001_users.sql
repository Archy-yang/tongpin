-- 0001: 用户表（auth 域起步所需；其余域的表随模块实现逐步增加）
CREATE TABLE users (
    id          BIGSERIAL PRIMARY KEY,
    phone       VARCHAR(20) UNIQUE,
    device_id   VARCHAR(64) UNIQUE,
    nickname    VARCHAR(32) NOT NULL,
    age_range   VARCHAR(10),
    gender      VARCHAR(10),
    city        VARCHAR(32),
    gender_pref VARCHAR(10),
    status      VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
