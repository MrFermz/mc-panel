-- +goose Up
-- username (optional login identifier) + soft delete: เก็บแถวไว้เพื่อรักษา FK/audit history
-- (owner_id/requested_by เป็น SET NULL แต่ยังอยากคงชื่อไว้อ้างอิงได้) แทน hard delete
ALTER TABLE users ADD COLUMN username VARCHAR(64);
ALTER TABLE users ADD COLUMN deleted_at TIMESTAMPTZ;

-- unique เฉพาะแถวที่ยังไม่ถูกลบ — ปล่อยให้ email/username ของ user ที่ลบไปแล้วถูกใช้ซ้ำได้
DROP INDEX idx_users_email_lower;
CREATE UNIQUE INDEX idx_users_email_lower ON users (lower(email)) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_users_username_lower ON users (lower(username)) WHERE deleted_at IS NULL AND username IS NOT NULL;

-- +goose Down
DROP INDEX idx_users_username_lower;
DROP INDEX idx_users_email_lower;
CREATE UNIQUE INDEX idx_users_email_lower ON users (lower(email));
ALTER TABLE users DROP COLUMN deleted_at;
ALTER TABLE users DROP COLUMN username;
