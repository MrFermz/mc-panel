-- +goose Up
-- ลบ user = soft delete มาตั้งแต่ 00004 แล้ว แต่ยังไม่มีทางกู้กลับ — เพิ่ม capability
-- `users.restore` คู่กับ `users.delete` (แนวเดียวกับ servers.delete/servers.restore)
-- backfill ให้คนที่ลบ user ได้อยู่แล้ว จะได้ไม่มีของค้างถังขยะโดยไม่มีใครกู้ได้
UPDATE users
SET capabilities = array_append(capabilities, 'users.restore')
WHERE 'users.delete' = ANY (capabilities)
  AND NOT ('users.restore' = ANY (capabilities));

-- +goose Down
UPDATE users SET capabilities = array_remove(capabilities, 'users.restore');
