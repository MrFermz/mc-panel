// Package migrations embed ไฟล์ SQL เข้า binary เพื่อให้ control-plane
// รัน goose เองตอน boot ได้ — final image เป็น distroless ไม่มีไฟล์ระบบอื่น
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
