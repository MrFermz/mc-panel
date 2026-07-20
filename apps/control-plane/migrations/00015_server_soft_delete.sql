-- +goose Up
-- Soft delete ของ server: DELETE /api/servers/{id} เปลี่ยนจาก "ลบจริง" เป็น mark deleted_at
-- (ไฟล์ใน MC_DATA_DIR ยังอยู่ครบ → restore กลับมาใช้ได้ทันที) ส่วนการลบจริงย้ายไปเป็น
-- POST /api/servers/{id}/purge ที่ยิง job delete_server เหมือนเดิมแล้วลบ row ทิ้ง
--
-- deleted_at ยังนับใน UNIQUE (node_id, host_port) และใน SumServerMemoryMBOnNode โดยตั้งใจ —
-- ทรัพยากรของ server ที่ถูกลบยังถูกจองไว้จริงบน node จน purge เพื่อให้ restore สำเร็จเสมอ
ALTER TABLE servers ADD COLUMN deleted_at TIMESTAMPTZ;

-- query ปกติทั้งหมด filter deleted_at IS NULL — partial index ครอบ row ที่ยัง active เท่านั้น
CREATE INDEX idx_servers_active ON servers (created_at) WHERE deleted_at IS NULL;

-- capability ใหม่ 2 ตัวคู่กับ flow นี้ (catalog อยู่ใน internal/httpapi/capabilities.go):
--   servers.restore = กู้ server ที่ถูกลบกลับมา
--   servers.purge   = ลบถาวรพร้อมไฟล์ (ทางเดียวที่ข้อมูลหายจริง)
-- backfill ให้คนที่เคยมี servers.delete อยู่แล้ว — เมื่อก่อน delete = ลบถาวร คนกลุ่มนี้จึง
-- ทำสองอย่างนี้ได้อยู่แล้วโดยปริยาย ไม่ให้เสียสิทธิ์ที่เคยมี
UPDATE users
SET capabilities = array_cat(
    capabilities,
    ARRAY(
        SELECT key FROM unnest(ARRAY['servers.restore', 'servers.purge']) AS key
        WHERE NOT key = ANY(capabilities)
    )
)
WHERE NOT is_admin AND 'servers.delete' = ANY(capabilities);

-- +goose Down
UPDATE users
SET capabilities = ARRAY(
    SELECT key FROM unnest(capabilities) AS key
    WHERE key NOT IN ('servers.restore', 'servers.purge')
)
WHERE NOT is_admin;

DROP INDEX idx_servers_active;

-- row ที่ถูก soft delete ไว้จะกลับมาเป็น server ปกติ (ไฟล์ยังอยู่ครบอยู่แล้ว)
ALTER TABLE servers DROP COLUMN deleted_at;
