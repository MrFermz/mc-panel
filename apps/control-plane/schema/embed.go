// Package schema embed schema.sql เข้า binary เพื่อให้ control-plane สร้าง schema เองตอน boot ได้
// — final image เป็น distroless ไม่มีไฟล์ระบบอื่น
package schema

import _ "embed"

// SQL คือ schema ปัจจุบันทั้งก้อน (idempotent) — ยังไม่มีระบบ migration ในเฟสนี้
//
//go:embed schema.sql
var SQL string
