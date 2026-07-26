-- +goose Up
-- cache รูปประจำตัวผู้เล่นต่อ uuid เก็บเป็น bytes ในแถวเลย เหมือน user avatar
-- (ระบบยังไม่มี object storage). ตารางนี้ไม่รู้จักเกม — "วิธีได้รูปมา" อยู่ใน game definition
-- ส่วนชั้น cache เป็นของกลางที่ internal/avatarcache. key ด้วย uuid ของผู้เล่นอย่างเดียว
-- (รูปเป็น global ต่อ account ไม่ผูก server) → แชร์ cache ข้ามทุก server ได้
--
-- เก็บลง storage แทน in-memory เพื่อให้ยังเสิร์ฟรูปเก่าได้ตอน upstream ของเกมติดต่อไม่ได้
-- (graceful degradation) และรอดข้าม restart
--
-- png = NULL คือ negative cache (uuid นี้ไม่มีรูป เช่น offline-mode / ไม่มี texture)
-- fetched_at ใช้ตัดสิน staleness (refresh เมื่อรูปเปลี่ยน) — อ่านค่าเมื่อ TTL หมด
CREATE TABLE player_avatars (
	uuid       UUID PRIMARY KEY,
	png        BYTEA,
	fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE player_avatars;
