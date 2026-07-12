-- +goose Up
-- Global capability ต่อ user (แยกจาก server_permissions ที่เป็นสิทธิ์ต่อ server)
-- admin ตั้งให้ user แต่ละคนเข้าถึงหน้า/เมนูของ panel + ทำ CRUD ได้ระดับไหน
-- is_admin = superuser ครอบทุก capability โดยปริยาย (ไม่ต้องใส่ในคอลัมน์นี้)
--
-- capability key ที่รู้จัก (catalog อยู่ใน control-plane, ห้าม hardcode value อื่น):
--   users.manage    เข้าหน้า Users + CRUD user
--   nodes.manage    เข้าหน้า Nodes + CRUD node
--   servers.create  สร้าง server ใหม่ได้
--   servers.view_all เห็น server ทุกตัว (เหมือน admin) ไม่จำกัดเฉพาะที่มี permission
ALTER TABLE users
    ADD COLUMN capabilities TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE users DROP COLUMN capabilities;
