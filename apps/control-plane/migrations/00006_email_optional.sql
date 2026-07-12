-- +goose Up
-- email เป็น optional ได้แล้ว: user สร้างด้วย email หรือ username อย่างใดอย่างหนึ่งก็พอ
-- (login รับ email-or-username อยู่แล้ว) — partial unique index idx_users_email_lower
-- เป็น unique บน lower(email) ซึ่งอนุญาต NULL ซ้ำได้ จึงปลอดภัยกับ username-only account
ALTER TABLE users ALTER COLUMN email DROP NOT NULL;

-- +goose Down
-- reinstate NOT NULL — จะ fail ถ้ามีแถว email IS NULL อยู่ (ยอมรับได้สำหรับ dev)
ALTER TABLE users ALTER COLUMN email SET NOT NULL;
