-- +goose Up
-- cache รูปหน้าผู้เล่น (crop จาก skin ของ Mojang) เก็บเป็น bytes ในแถวเลย เหมือน user avatar
-- (ระบบยังไม่มี object storage). key ด้วย uuid ของผู้เล่น — skin เป็น global ต่อ account
-- ไม่ผูก server → แชร์ cache ข้ามทุก server ได้. เก็บลง storage แทน in-memory เพื่อให้
-- ยังเสิร์ฟรูปเก่าได้ตอน Mojang ติดต่อไม่ได้ (graceful degradation) และรอดข้าม restart
--
-- png = NULL คือ negative cache (uuid นี้ไม่มี skin เช่น offline-mode / ไม่มี texture)
-- fetched_at ใช้ตัดสิน staleness (refresh เมื่อ skin/ชื่อเปลี่ยน) — อ่านค่าเมื่อ TTL หมด
CREATE TABLE player_faces (
	uuid       UUID PRIMARY KEY,
	png        BYTEA,
	fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE player_faces;
