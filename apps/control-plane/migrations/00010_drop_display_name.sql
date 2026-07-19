-- +goose Up
-- เลิกใช้ display name — UI แสดง username (ตกไป email ถ้าเป็น account ที่ไม่มี username) แทน
-- จึงไม่มีที่ไหนอ่านคอลัมน์นี้แล้ว
ALTER TABLE users DROP COLUMN display_name;

-- +goose Down
ALTER TABLE users ADD COLUMN display_name VARCHAR(100) NOT NULL DEFAULT '';
