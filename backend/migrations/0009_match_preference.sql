-- 0009: 用户匹配偏好向量（聊后反馈的协同信号）
-- 偏好向量 = 该用户打过分的对象画像嵌入的加权平均（like +1 / mismatch -0.5）
CREATE TABLE user_match_preferences (
    user_id        BIGINT PRIMARY KEY REFERENCES users(id),
    embedding      VECTOR(64),          -- 归一化偏好向量（维度与 style_profiles 一致）
    like_count     INT NOT NULL DEFAULT 0,
    mismatch_count INT NOT NULL DEFAULT 0,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
