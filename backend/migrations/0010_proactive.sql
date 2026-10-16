-- 0010: AI 主动开话
ALTER TABLE conversations ADD COLUMN proactive_enabled BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE conversations ADD COLUMN last_proactive_date DATE;
