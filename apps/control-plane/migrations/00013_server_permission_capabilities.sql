-- +goose Up
-- ชั้น server (server_permissions) เดิมหยาบ: role (owner/operator/viewer) + 2 bool
-- (can_console_write, can_manage_files). เปลี่ยนเป็น per-feature grant ราย capability
-- ให้ symmetric กับชั้น web (global capability) — effective = global cap AND grant ต่อ server
--
--   role owner   -> owner (ได้ทุก server-scoped cap โดยปริยาย + จัดการ access list)
--   role operator/viewer -> member + capabilities[] ตาม preset เดิม + สิทธิ์จาก bool ที่เคยตั้ง
--
-- server-scoped cap = subset ของ catalog ที่มีความหมายต่อ server ตัวหนึ่ง (ดู capabilities.go)
ALTER TABLE server_permissions
    ADD COLUMN capabilities TEXT[] NOT NULL DEFAULT '{}';

-- backfill capabilities ให้ member เดิมไม่เสียสิทธิ์ที่เคยมี (owner ข้าม = ได้ทุกอย่างอยู่แล้ว)
UPDATE server_permissions
SET capabilities = (
    SELECT ARRAY(
        SELECT DISTINCT c FROM unnest(
            CASE role
                WHEN 'operator' THEN ARRAY[
                    'servers.power',
                    'console.view', 'console.write',
                    'files.view', 'files.write', 'files.delete',
                    'players.view', 'players.manage', 'players.moderate',
                    'settings.view', 'settings.edit'
                ]
                WHEN 'viewer' THEN ARRAY[
                    'console.view', 'files.view', 'players.view', 'settings.view'
                ]
                ELSE ARRAY[]::TEXT[]
            END
            ||
            CASE WHEN can_console_write
                 THEN ARRAY['console.view', 'console.write'] ELSE ARRAY[]::TEXT[] END
            ||
            -- can_manage_files เดิมคุมทั้ง file manager, properties และ player management
            CASE WHEN can_manage_files
                 THEN ARRAY[
                     'files.view', 'files.write', 'files.delete',
                     'players.view', 'players.manage', 'players.moderate',
                     'settings.view', 'settings.edit'
                 ] ELSE ARRAY[]::TEXT[] END
        ) AS c
    )
)
WHERE role IN ('operator', 'viewer');

-- role เก่า (owner/operator/viewer) -> owner/member ; ต้อง drop CHECK เดิมก่อน update
-- ไม่งั้นค่า 'member' ผิด constraint เดิม แล้วค่อย add CHECK ใหม่หลัง update เสร็จ
ALTER TABLE server_permissions DROP CONSTRAINT server_permissions_role_check;
UPDATE server_permissions SET role = 'member' WHERE role IN ('operator', 'viewer');
ALTER TABLE server_permissions
    ADD CONSTRAINT server_permissions_role_check CHECK (role IN ('owner', 'member'));

ALTER TABLE server_permissions
    DROP COLUMN can_console_write,
    DROP COLUMN can_manage_files;

-- +goose Down
ALTER TABLE server_permissions
    ADD COLUMN can_console_write BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN can_manage_files BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE server_permissions
SET can_console_write = 'console.write' = ANY(capabilities),
    can_manage_files  = 'files.write' = ANY(capabilities);

ALTER TABLE server_permissions DROP CONSTRAINT server_permissions_role_check;
-- member -> viewer (lossy: capabilities set หายไป, เหลือ role หยาบเดิม)
UPDATE server_permissions SET role = 'viewer' WHERE role = 'member';
ALTER TABLE server_permissions
    ADD CONSTRAINT server_permissions_role_check CHECK (role IN ('owner', 'operator', 'viewer'));

ALTER TABLE server_permissions DROP COLUMN capabilities;
