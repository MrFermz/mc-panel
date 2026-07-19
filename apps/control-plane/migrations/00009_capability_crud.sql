-- +goose Up
-- ขยาย global capability จาก 4 key รวม ๆ เป็น key แบบ "{feature}.{action}" ครบทุกฟีเจอร์
-- (catalog อยู่ใน internal/httpapi/capabilities.go — ที่นี่แค่ migrate ข้อมูลเดิม)
--
--   users.manage  -> users.view/create/edit/delete/reset_password
--   nodes.manage  -> nodes.view/create/delete
--   servers.create / servers.view_all  คงเดิม
--
-- ส่วน feature ที่เมื่อก่อนคุมด้วย server_permissions อย่างเดียว (console/files/players/
-- settings/access + edit/delete/power ของ server) ตอนนี้ต้องมี global capability ด้วย
-- (enforce แบบ AND) — จึง backfill ให้ user เดิมทุกคนเพื่อไม่ให้ใครเสียสิทธิ์ที่เคยมี
-- (สิทธิ์จริงต่อ server ยังถูกจำกัดด้วย server_permissions เหมือนเดิม)
UPDATE users
SET capabilities = (
    SELECT ARRAY(
        SELECT DISTINCT key FROM unnest(
            CASE WHEN 'users.manage' = ANY(capabilities)
                 THEN ARRAY['users.view', 'users.create', 'users.edit', 'users.delete', 'users.reset_password']
                 ELSE ARRAY[]::TEXT[] END
            ||
            CASE WHEN 'nodes.manage' = ANY(capabilities)
                 THEN ARRAY['nodes.view', 'nodes.create', 'nodes.delete']
                 ELSE ARRAY[]::TEXT[] END
            ||
            CASE WHEN 'servers.create' = ANY(capabilities)
                 THEN ARRAY['servers.create'] ELSE ARRAY[]::TEXT[] END
            ||
            CASE WHEN 'servers.view_all' = ANY(capabilities)
                 THEN ARRAY['servers.view_all'] ELSE ARRAY[]::TEXT[] END
            ||
            ARRAY[
                'servers.edit', 'servers.delete', 'servers.power',
                'console.view', 'console.write',
                'files.view', 'files.write', 'files.delete',
                'players.view', 'players.manage', 'players.moderate',
                'settings.view', 'settings.edit',
                'access.view', 'access.manage'
            ]
        ) AS key
    )
)
WHERE NOT is_admin;

-- admin ครอบทุก capability อยู่แล้ว — ล้าง key เก่าทิ้งไม่ให้ค้างเป็นค่าที่ catalog ไม่รู้จัก
UPDATE users
SET capabilities = '{}'
WHERE is_admin;

-- +goose Down
-- ย้อนกลับเป็น key ชุดเดิม (key ย่อยที่ไม่มีในชุดเก่าถูกทิ้ง)
UPDATE users
SET capabilities = (
    SELECT ARRAY(
        SELECT DISTINCT key FROM unnest(
            CASE WHEN 'users.view' = ANY(capabilities)
                 THEN ARRAY['users.manage'] ELSE ARRAY[]::TEXT[] END
            ||
            CASE WHEN 'nodes.view' = ANY(capabilities)
                 THEN ARRAY['nodes.manage'] ELSE ARRAY[]::TEXT[] END
            ||
            CASE WHEN 'servers.create' = ANY(capabilities)
                 THEN ARRAY['servers.create'] ELSE ARRAY[]::TEXT[] END
            ||
            CASE WHEN 'servers.view_all' = ANY(capabilities)
                 THEN ARRAY['servers.view_all'] ELSE ARRAY[]::TEXT[] END
        ) AS key
    )
)
WHERE NOT is_admin;
