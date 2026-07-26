-- +goose Up
-- Game definition: server ทุกตัวผูกกับ "เกม" หนึ่งเกม แล้ว server_type กลายเป็น variant
-- ภายในเกมนั้น (minecraft: vanilla/paper/fabric/forge/velocity) — ความรู้เฉพาะเกมทั้งหมด
-- (variant, กติกาเวอร์ชัน, runtime image, catalog ของ config, กติกาผู้เล่น) ย้ายไปอยู่ใน
-- registry ที่ internal/games ทั้ง control-plane และ node-agent
--
-- เฟสนี้มีเกมเดียวคือ minecraft — DEFAULT จึงทำหน้าที่ backfill row เดิมทั้งตารางในตัว
-- (คอลัมน์ไม่ nullable เพื่อให้ทุก code path มี game ให้ lookup เสมอ ไม่ต้องเดา)
ALTER TABLE servers ADD COLUMN game TEXT NOT NULL DEFAULT 'minecraft';

-- +goose Down
ALTER TABLE servers DROP COLUMN game;
