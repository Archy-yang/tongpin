-- 0008: 邮箱 + 密码登录体系
ALTER TABLE users ADD COLUMN email VARCHAR(255) UNIQUE;
ALTER TABLE users ADD COLUMN password_hash VARCHAR(255);
ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT false;

-- 邮箱验证码（找回密码等用途），存哈希不存明文
CREATE TABLE email_codes (
    id         BIGSERIAL PRIMARY KEY,
    email      VARCHAR(255) NOT NULL,
    code_hash  VARCHAR(128) NOT NULL,
    purpose    VARCHAR(16) NOT NULL,           -- 'reset'（后续可扩展 'verify'）
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_email_codes_email ON email_codes(email, purpose, created_at DESC);
