-- 0011: 免费聊天额度（订阅制收费的硬前置）
-- 0 = 使用全局默认额度；负数 = 不限额（付费系统接入后由支付侧写入，作为会员占位）
ALTER TABLE users ADD COLUMN free_quota INT NOT NULL DEFAULT 0;
