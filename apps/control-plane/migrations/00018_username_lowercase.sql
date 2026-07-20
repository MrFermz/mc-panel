-- +goose Up
-- username เก็บเป็นตัวพิมพ์เล็กเสมอ — เดิมเก็บตามที่พิมพ์มาแล้วเทียบด้วย lower() ทุกครั้ง
-- ทำให้ชื่อเดียวกันแสดงผลไม่เหมือนกันแล้วแต่ว่าใครสร้าง (`Alice` / `alice` / `ALICE`)
-- ตอนนี้ canonical เป็น lowercase ที่ชั้น DB ไปเลย ไม่ต้องเดาว่าค่าที่อ่านมาถูก normalize รึยัง
--
-- ปลอดภัยกับข้อมูลเดิม: unique index `idx_users_username_lower` เป็น unique บน lower(username)
-- ทั้งตารางอยู่แล้ว (00017) จึงไม่มีทางมีสองแถวที่ lower() ตรงกัน — lower ทั้งคอลัมน์จึงชนกันไม่ได้
UPDATE users SET username = lower(username) WHERE username <> lower(username);

-- กันโค้ดที่ลืม normalize เขียนตัวใหญ่กลับเข้ามาอีก (DB เป็นด่านสุดท้าย)
ALTER TABLE users ADD CONSTRAINT users_username_lowercase
    CHECK (username = lower(username));

-- +goose Down
ALTER TABLE users DROP CONSTRAINT users_username_lowercase;
-- ตัวพิมพ์ใหญ่เดิมกู้กลับไม่ได้ (ข้อมูลหายไปตอน UPDATE) — ปล่อยเป็น lowercase ตามเดิม
