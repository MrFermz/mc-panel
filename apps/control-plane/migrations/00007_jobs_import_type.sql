-- +goose Up
-- import_server เป็น job lifecycle ชนิดใหม่ (import server/world จาก zip) — ขยาย CHECK ของ jobs.type
ALTER TABLE jobs DROP CONSTRAINT jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check
    CHECK (type IN ('create_server', 'start_server', 'stop_server',
                    'kill_server', 'delete_server', 'import_server'));

-- +goose Down
ALTER TABLE jobs DROP CONSTRAINT jobs_type_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_type_check
    CHECK (type IN ('create_server', 'start_server', 'stop_server',
                    'kill_server', 'delete_server'));
