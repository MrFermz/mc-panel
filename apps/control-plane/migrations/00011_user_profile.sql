-- +goose Up
-- profile ที่ user แก้เองได้: display name + avatar
-- display_name กลับมาอีกครั้ง (00010 ตัดทิ้งไปตอนที่ยังไม่มีหน้าให้ user แก้เอง) —
-- คราวนี้เป็นค่าที่เจ้าของบัญชีตั้งเอง ว่าง = ตกไปใช้ username/email เหมือนเดิม
ALTER TABLE users ADD COLUMN display_name VARCHAR(64) NOT NULL DEFAULT '';

-- เก็บรูป avatar เป็น bytes ในแถว user เลย (ระบบยังไม่มี object storage และรูปถูกจำกัด
-- ขนาดไว้แล้วที่ชั้น handler) — คอลัมน์ avatar ไม่เคยอยู่ใน SELECT ปกติ อ่านเฉพาะตอน
-- serve /api/users/{id}/avatar. avatar_updated_at ใช้ทั้ง cache-buster ของ URL และ ETag
ALTER TABLE users ADD COLUMN avatar BYTEA;
ALTER TABLE users ADD COLUMN avatar_mime VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN avatar_updated_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN avatar_updated_at;
ALTER TABLE users DROP COLUMN avatar_mime;
ALTER TABLE users DROP COLUMN avatar;
ALTER TABLE users DROP COLUMN display_name;
