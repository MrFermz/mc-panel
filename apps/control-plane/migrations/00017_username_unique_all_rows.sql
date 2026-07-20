-- +goose Up
-- username ต้องไม่ซ้ำกัน "ทั้งตาราง" — เดิม unique index เป็น partial (`WHERE deleted_at IS NULL`)
-- ชื่อของบัญชีที่อยู่ในถังขยะจึงถูกเอาไปใช้ใหม่ได้ ทำให้ตอนกู้คืนอาจชนกันจนกู้ไม่ได้
-- ตอนนี้ชื่อถูกจองไว้ตลอด: ลบแล้ว restore ได้เสมอ แลกกับที่ชื่อนั้นจะสร้างซ้ำไม่ได้จนกว่าจะกู้คืน

-- 1) เคลียร์ชื่อที่ซ้ำอยู่ก่อน ไม่งั้น CREATE UNIQUE INDEX จะล้มทั้ง migration
--    ผู้ชนะ (ได้ใช้ชื่อเดิม) = แถวที่ยังไม่ถูกลบก่อน ถ้าเสมอกันเอาแถวที่เก่าที่สุด
--    แถวที่เหลือ (อยู่ในถังขยะทั้งนั้น เพราะ partial index กันแถว active ซ้ำกันอยู่แล้ว)
--    ถูกต่อท้ายด้วย `-oldN` — login ไม่ได้อยู่แล้วจึงไม่กระทบใคร แต่ audit ยังตามรอยได้
-- +goose StatementBegin
DO $$
DECLARE
    r         RECORD;
    candidate TEXT;
    n         INT;
BEGIN
    FOR r IN
        SELECT u.id, u.username
        FROM users u
        WHERE EXISTS (
                  SELECT 1 FROM users o
                  WHERE lower(o.username) = lower(u.username) AND o.id <> u.id
              )
          AND u.id <> (
                  SELECT w.id FROM users w
                  WHERE lower(w.username) = lower(u.username)
                  ORDER BY (w.deleted_at IS NULL) DESC, w.created_at
                  LIMIT 1
              )
        ORDER BY u.created_at
    LOOP
        n := 0;
        LOOP
            n := n + 1;
            -- left(...,48) กัน VARCHAR(64) ล้นเมื่อชื่อเดิมยาวเต็มพิกัด
            candidate := left(r.username, 48) || '-old' || n::text;
            EXIT WHEN NOT EXISTS (
                SELECT 1 FROM users WHERE lower(username) = lower(candidate)
            );
        END LOOP;
        UPDATE users SET username = candidate WHERE id = r.id;
    END LOOP;
END $$;
-- +goose StatementEnd

-- 2) partial -> unique ทั้งตาราง
DROP INDEX idx_users_username_lower;
CREATE UNIQUE INDEX idx_users_username_lower ON users (lower(username));

-- +goose Down
-- ชื่อที่ถูก rename ไปแล้วไม่ถูกย้อนกลับ (ข้อมูลเดิมหาย ไม่มีทางรู้ว่าอันไหนเคยชื่ออะไร)
DROP INDEX idx_users_username_lower;
CREATE UNIQUE INDEX idx_users_username_lower ON users (lower(username)) WHERE deleted_at IS NULL;
