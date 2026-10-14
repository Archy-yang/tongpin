-- 0005: matches.status 放宽到 24（'contact_requested' 有 17 字符，原 16 不够）
ALTER TABLE matches ALTER COLUMN status TYPE VARCHAR(24);
ALTER TABLE contact_requests ALTER COLUMN status TYPE VARCHAR(24);
ALTER TABLE match_opt_ins ALTER COLUMN status TYPE VARCHAR(24);
