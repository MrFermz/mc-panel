-- +goose Up
-- server_players: whitelist ต่อ server — mirror ของ whitelist.json ที่ agent เขียนลง disk
-- (source of truth คือ DB, ไฟล์ rebuild จากตารางนี้ทุกครั้งที่ add/remove)
-- uuid คือ Mojang UUID (online-mode); username เก็บไว้แสดงผล + เขียนลง whitelist.json
CREATE TABLE server_players (
  server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  uuid UUID NOT NULL,
  username VARCHAR(16) NOT NULL,
  added_by UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (server_id, uuid)
);

-- +goose Down
DROP TABLE server_players;
