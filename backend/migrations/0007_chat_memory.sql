-- 0007: AI 长期记忆——记录每次记忆更新覆盖到的最大消息 ID
ALTER TABLE conversations ADD COLUMN last_memory_msg_id BIGINT NOT NULL DEFAULT 0;
