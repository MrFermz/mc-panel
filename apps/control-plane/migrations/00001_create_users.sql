-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Auth เป็น local อย่างเดียวใน scope ปัจจุบัน (OIDC ค่อยเพิ่ม migration ใหม่ทีหลัง)
-- flow ผู้ใช้ใหม่ทุกคน: ระบบเจน password มั่ว ๆ ให้ -> must_change_password = TRUE
-- -> login ครั้งแรกโดนบังคับตั้ง password ใหม่ก่อนใช้งานอย่างอื่น
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL,
    password_hash TEXT NOT NULL,
    display_name VARCHAR(100) NOT NULL DEFAULT '',
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    must_change_password BOOLEAN NOT NULL DEFAULT TRUE,
    -- bump ทุกครั้งที่เปลี่ยน password / โดน reset -> JWT เก่าทุกใบใช้ไม่ได้ทันที
    token_version INT NOT NULL DEFAULT 1,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_users_email_lower ON users (lower(email));

-- +goose Down
DROP TABLE users;
