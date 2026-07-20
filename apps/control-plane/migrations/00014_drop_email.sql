-- +goose Up
-- เลิกใช้ email ทั้งระบบ: username เป็น login identifier เดียว (ไม่มี OIDC/ส่งเมลใน scope นี้
-- email จึงเป็นแค่ field ที่ต้องดูแลโดยไม่ได้ใช้งานจริง)

-- backfill ก่อนบังคับ NOT NULL — บัญชีที่เคยมีแต่ email จะ login ไม่ได้เลยถ้าไม่มี username
-- ใช้ local-part ของ email เป็นฐาน แล้วต่อเลขถ้าชนของเดิม (เทียบ lower ทั้งตาราง รวมแถวที่ soft delete
-- ไปแล้ว — เข้มกว่า partial unique index จริง แต่กันชื่อชนกันตอน user ถูกกู้คืนในอนาคต)
-- +goose StatementBegin
DO $$
DECLARE
    r         RECORD;
    base      TEXT;
    candidate TEXT;
    n         INT;
BEGIN
    FOR r IN SELECT id, email FROM users WHERE username IS NULL ORDER BY created_at LOOP
        base := left(regexp_replace(split_part(coalesce(r.email, ''), '@', 1),
                                    '[^a-zA-Z0-9_.-]', '', 'g'), 60);
        IF length(base) < 3 THEN
            base := 'user';
        END IF;
        candidate := base;
        n := 1;
        WHILE EXISTS (SELECT 1 FROM users WHERE lower(username) = lower(candidate)) LOOP
            n := n + 1;
            candidate := base || n::text;
        END LOOP;
        UPDATE users SET username = candidate WHERE id = r.id;
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE users ALTER COLUMN username SET NOT NULL;

-- index เดิมมีเงื่อนไข `username IS NOT NULL` ซึ่งไม่จำเป็นแล้ว
DROP INDEX idx_users_username_lower;
CREATE UNIQUE INDEX idx_users_username_lower ON users (lower(username)) WHERE deleted_at IS NULL;

DROP INDEX idx_users_email_lower;
ALTER TABLE users DROP COLUMN email;

-- +goose Down
ALTER TABLE users ADD COLUMN email VARCHAR(255);
CREATE UNIQUE INDEX idx_users_email_lower ON users (lower(email)) WHERE deleted_at IS NULL;

DROP INDEX idx_users_username_lower;
ALTER TABLE users ALTER COLUMN username DROP NOT NULL;
CREATE UNIQUE INDEX idx_users_username_lower ON users (lower(username))
    WHERE deleted_at IS NULL AND username IS NOT NULL;
