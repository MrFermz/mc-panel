# API Contract — control-plane

Source of truth ของ HTTP/WS interface ระหว่าง web กับ control-plane
**แก้ไฟล์นี้ก่อนแก้โค้ดเสมอ** แล้วค่อย implement ให้ตรงทั้งสองฝั่ง

Base: ผ่าน Caddy ที่ origin เดียวกัน — `/api/*` และ `/ws/*` ชี้ไป control-plane
Dev (ไม่ผ่าน Caddy): web dev server proxy `/api`,`/ws` ไป `localhost:8080` ผ่าน Next rewrites

## รูปแบบ response

- สำเร็จ: `200/201` + JSON ตรง ๆ (ไม่มี envelope)
- ผิดพลาด: status code + `{"code": "machine_readable_code", "message": "อธิบายให้คนอ่าน"}`
- code ที่ web ต้อง handle เป็นพิเศษ:
  - `unauthorized` (401) — ไม่มี/หมดอายุ session → redirect `/login`
  - `password_change_required` (403) — ต้อง redirect ไป `/change-password`
  - `forbidden` (403) — ไม่มีสิทธิ์
  - `rate_limited` (429)

## Auth

Session = JWT (HS256) ใน cookie `mc_session` (HttpOnly, SameSite=Lax, Path=/, อายุ 24 ชม.)
Claims: `sub` (user id), `ver` (token_version), `iat`, `exp`
Middleware โหลด user จาก DB ทุก request — เช็ค `is_active` และ `ver == token_version`

CSRF: อาศัย SameSite=Lax + endpoint เขียนทั้งหมดเป็น POST/PATCH/PUT/DELETE + GET ห้ามมี side effect
WS: ตรวจ `Origin` header ตอน handshake เสมอ

เมื่อ `must_change_password = true`: ทุก endpoint ตอบ `403 password_change_required`
ยกเว้น `POST /api/auth/change-password`, `GET /api/auth/me`, `POST /api/auth/logout`

| Method | Path | ใคร | Body → Response |
|---|---|---|---|
| POST | `/api/auth/login` | public (rate limit 10/นาที/IP ผ่าน redis) | `{identifier, password}` → `{user}` + Set-Cookie; 401 `invalid_credentials`. `identifier` = email หรือ username ก็ได้ (legacy `{email}` ยังใช้ได้ — fallback เมื่อไม่ส่ง `identifier`) |
| POST | `/api/auth/logout` | ทุกคน | → 204 + ลบ cookie |
| GET | `/api/auth/me` | ทุกคน | → `{user}` |
| POST | `/api/auth/change-password` | ทุกคน | `{current_password, new_password}` → `{user}` + Set-Cookie ใหม่ (bump token_version) |
| PATCH | `/api/auth/me` | ทุกคน | `{display_name}` → `{user}` — profile ของตัวเอง (ตัด control char + trim, ยาว ≤ 64 ตัวอักษร ไม่งั้น 400 `invalid_display_name`); ว่างได้ = กลับไปใช้ username/email |
| PUT | `/api/auth/me/avatar` | ทุกคน | `multipart/form-data` part `avatar` → `{user}` — รูปโปรไฟล์ของตัวเอง (≤ 512KB ไม่งั้น 413 `avatar_too_large`; ชนิดตัดสินจาก content sniffing ของ bytes จริง ต้องเป็น PNG/JPEG/GIF/WebP ไม่งั้น 400 `invalid_image_type`) |
| DELETE | `/api/auth/me/avatar` | ทุกคน | → `{user}` — ลบรูปโปรไฟล์ (`avatar_url` กลับเป็น `null`) |
| GET | `/api/users/{id}/avatar` | login แล้ว | → bytes ของรูป (`Content-Type` ตามชนิดจริง, `ETag` + `Cache-Control: private`); 404 `not_found` เมื่อ user นั้นไม่มีรูป |

**profile endpoint ทั้งสามเส้นไม่ผูก capability** — ยึด user id จาก session เสมอจึงแตะได้แค่บัญชี
ตัวเอง (แนวเดียวกับ `POST /api/auth/change-password`); การแก้ข้อมูล user คนอื่นยังต้องผ่าน
`users.edit` ที่ `PATCH /api/users/{id}` เหมือนเดิม. `GET /api/users/{id}/avatar` เปิดให้ทุกคนที่
login แล้ว (เหมือน `/users/directory`) เพราะรูปโผล่คู่กับชื่อในลิสต์สมาชิก/access อยู่แล้ว

`user` object: `{id, email, username, display_name, avatar_url, is_admin, is_active, must_change_password, capabilities, created_at}`
(`username` = string หรือ `null` — optional login identifier;
`display_name` = ชื่อที่เจ้าของบัญชีตั้งเอง อาจเป็น `""`;
`avatar_url` = `/api/users/{id}/avatar?v={unix}` หรือ `null` เมื่อยังไม่ตั้งรูป — `?v=` เป็น cache-buster)
password policy: ยาว ≥ 10 ตัวอักษร (เช็คทั้ง web และ server)

### Capabilities (global RBAC — แยกจาก server_permissions)

`capabilities` = array ของ key ที่ admin ตั้งให้ user (is_admin ครอบทุก capability โดยปริยาย)
เมนู/หน้า/ปุ่มฝั่ง web แสดงตาม **effective capability** = `is_admin ? ทั้งหมด : capabilities`
backend บังคับทุก endpoint: ผ่านเมื่อ `is_admin` **หรือ** มี capability ที่กำหนด

key เป็นรูป `{feature}.{action}` เสมอ — catalog (source of truth) อยู่ที่
`apps/control-plane/internal/httpapi/capabilities.go`, map endpoint → capability อยู่ในตาราง
route เดียวใน `internal/httpapi/api.go`

| key | endpoint ที่คุม |
|---|---|
| `users.view` | `GET /api/users`, `GET /api/users/{id}` (หน้า/เมนู Users) |
| `users.create` | `POST /api/users` |
| `users.edit` | `PATCH /api/users/{id}` (บทบาท/สิทธิ์/สถานะ) |
| `users.delete` | `DELETE /api/users/{id}` |
| `users.reset_password` | `POST /api/users/{id}/reset-password` |
| `nodes.view` | `GET /api/nodes` (หน้า/เมนู Nodes) + รับ event `node_stats` |
| `nodes.create` | `POST /api/nodes` |
| `nodes.delete` | `DELETE /api/nodes/{id}` |
| `servers.view_all` | เห็น server ทุกตัว (เหมือน admin) ไม่จำกัดเฉพาะที่มี server_permission |
| `servers.create` | `POST /api/servers`, `POST /api/servers/import` |
| `servers.edit` | `PATCH /api/servers/{id}` |
| `servers.delete` | `DELETE /api/servers/{id}` |
| `servers.power` | `POST /api/servers/{id}/actions` (start/stop/restart/kill) |
| `console.view` | `GET /api/servers/{id}/console/history`, `WS /ws/servers/{id}/console` |
| `console.write` | `{"type":"input"}` บน console WS |
| `files.view` | `GET /api/servers/{id}/files`, `.../files/content` |
| `files.write` | `PUT .../files/content`, `POST .../files/dir`, `POST .../files/rename` |
| `files.delete` | `DELETE /api/servers/{id}/files` |
| `players.view` | `GET /api/servers/{id}/players` |
| `players.manage` | `POST /api/servers/{id}/players`, `DELETE .../players/{uuid}` |
| `players.moderate` | `POST /api/servers/{id}/players/action` (op/deop/kick/ban/pardon) |
| `settings.view` | `GET /api/servers/{id}/properties` |
| `settings.edit` | `PUT /api/servers/{id}/properties` |
| `access.view` | `GET /api/servers/{id}/permissions` |
| `access.manage` | `POST /api/servers/{id}/permissions`, `DELETE .../permissions/{user_id}` |

⚠️ capability ที่คุม endpoint ระดับ server (`console.*`, `files.*`, `players.*`, `settings.*`,
`access.*`, `servers.edit/delete/power`) เป็นชั้น **เพิ่มเติม** จาก `server_permissions` —
ต้องผ่าน **ทั้งสองชั้น** (capability AND สิทธิ์ต่อ server นั้น) is_admin ข้ามทั้งคู่

| Method | Path | ใคร | Response |
|---|---|---|---|
| GET | `/api/meta/capabilities` | login แล้ว | → `{capabilities: [{key, group, action, label, description}]}` — catalog สำหรับหน้า permission (`group`/`action` ให้ web จัดกลุ่ม + แปลเอง, `label`/`description` เป็น fallback อังกฤษ) |
| GET | `/api/users/directory` | login แล้ว | → `{users: [{id, email, username, display_name, avatar_url}]}` — user ที่ active + ยังไม่ถูกลบทั้งหมด (field เบา, ไม่ leak สิทธิ์/สถานะ); **ไม่ต้องมี** `users.view` — owner ใช้เลือก collaborator ตอน grant permission (`email` อาจเป็น `""`, `username` อาจเป็น null); เรียงตาม username/email |

## Users (ต้องมี capability `users.*` ตามตารางด้านบน หรือ is_admin)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/api/users/{id}` | → `{user}` — โหลด user คนเดียว (หน้า `/admin/users/{id}/permissions` เปิดตรงจาก URL ได้); 404 `user_not_found` |
| GET | `/api/users` | `?search=&role=&status=` → `{users: [user]}` — filter: `search` (email/username substring), `role` (`admin`\|`user`), `status` (`active`\|`inactive`); user ที่ถูกลบ (soft delete) ไม่แสดง |
| POST | `/api/users` | `{email?, username?, is_admin, capabilities?}` → 201 `{user, initial_password}` — password สุ่ม 20 ตัว แสดงครั้งเดียว; ต้องมี `email` หรือ `username` อย่างน้อยหนึ่งอย่าง ไม่งั้น 400 `identifier_required` (username-only account ได้ — `user.email` จะเป็น `""`); `email` (optional) ถ้าส่งต้องมี `@` และยาว ≤255 ไม่งั้น 400 `invalid_email`; `username` (optional) ต้อง match `^[a-zA-Z0-9_.-]{3,64}$` ไม่งั้น 400 `invalid_request`; 409 `email_exists` / `username_exists` เมื่อซ้ำ |
| PATCH | `/api/users/{id}` | `{is_admin?, is_active?, capabilities?}` → `{user}` — ห้ามถอด admin/ปิด active ตัวเอง; `capabilities` ต้องเป็น key ใน catalog เท่านั้น |
| DELETE | `/api/users/{id}` | → 204 — soft delete (mark `deleted_at` + ปิด active + bump token_version → session เก่าตาย + ล้าง server_permissions); email/username ที่ถูกลบใช้ซ้ำได้; 400 `cannot_delete_self`, 404 `not_found` |
| POST | `/api/users/{id}/reset-password` | → `{initial_password}` — สุ่มใหม่ + must_change_password + bump token_version |

## Nodes (ต้องมี capability `nodes.*` ตามตารางด้านบน หรือ is_admin)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/api/nodes` | → `{nodes: [node]}` — node รวม stats ล่าสุดจาก heartbeat |
| POST | `/api/nodes` | `{name}` → 201 `{node, token}` — token format `<node_id>.<secret>` แสดงครั้งเดียว |
| DELETE | `/api/nodes/{id}` | → 204 — ลบได้เฉพาะ node ที่ไม่มี server |

`node`: `{id, name, status, agent_version, os, arch, cpu_percent, memory_used_mb, memory_total_mb, disk_used_mb, disk_total_mb, net_rx_bps, net_tx_bps, last_heartbeat_at, created_at}`
(`net_rx_bps`/`net_tx_bps` = network rate ของ node หน่วย bytes/sec, เก็บใน nodes row เหมือน cpu/mem/disk)

## Servers

สิทธิ์: `is_admin` เห็น/ทำได้ทุกอย่าง | นอกนั้นดูตาม `server_permissions.role`
- `owner` — ทุกอย่างของ server ตัวเอง รวมจัดการ permission
- `operator` — start/stop/restart/kill, ดู console, พิมพ์ console ถ้า `can_console_write`
- `viewer` — ดูอย่างเดียว (status + console read)

user ที่มี capability `servers.create` (หรือ is_admin) สร้าง server ได้ — คนสร้างได้ role `owner` อัตโนมัติ

| Method | Path | ใคร | Body → Response |
|---|---|---|---|
| GET | `/api/servers` | login แล้ว | → `{servers: [server]}` เฉพาะที่มีสิทธิ์เห็น |
| POST | `/api/servers` | login แล้ว | `{name, node_id, server_type, mc_version, memory_mb, host_port?, accept_eula}` → 201 `{server, job}` — ต้อง accept_eula=true ยกเว้น velocity; 400 `insufficient_memory` เมื่อ RAM เกิน (ดู admission control ล่าง) |
| POST | `/api/servers/import` | `servers.create` | `multipart/form-data` (ดูด้านล่าง) → 201 `{server, job}` — import server เดิมจาก .zip; 400 `insufficient_memory` เหมือน create |
| GET | `/api/servers/{id}` | viewer+ | → `{server, permissions: [permission+user_email]}` |
| PATCH | `/api/servers/{id}` | owner/admin | `{name?, memory_mb?, host_port?}` → `{server}` (มีผลตอน start ครั้งถัดไป); `memory_mb`/`host_port` แก้ได้เฉพาะตอน stopped/errored (409 `invalid_state`); ขยาย `memory_mb` เกิน RAM node → 400 `insufficient_memory` |
| DELETE | `/api/servers/{id}` | owner/admin | → `{job}` — สั่งงาน delete (ลบ container + ข้อมูลทั้งหมด) |
| POST | `/api/servers/{id}/actions` | operator+ | `{action: "start"\|"stop"\|"restart"\|"kill"}` → `{job}` |
| GET | `/api/servers/{id}/jobs` | viewer+ | `?limit=20` → `{jobs: [job]}` |
| GET | `/api/servers/{id}/console/history` | viewer+ | → `{lines: [string]}` (ring buffer 500 บรรทัดล่าสุด) |
| GET | `/api/servers/{id}/permissions` | owner/admin | → `{permissions: [{user_id, email, username, display_name, avatar_url, role, can_console_write, can_manage_files}]}` |
| POST | `/api/servers/{id}/permissions` | owner/admin | `{user_id?, email?, role, can_console_write, can_manage_files}` → upsert; resolve target ด้วย `user_id` ก่อน (จาก `/users/directory`) ไม่งั้น fallback `email` (404 `user_not_found` ถ้าไม่เจอ/ถูกลบ) |
| DELETE | `/api/servers/{id}/permissions/{user_id}` | owner/admin | → 204 — ห้ามลบ owner คนสุดท้าย |

### ความหมายของ `memory_mb`

`memory_mb` = **hard limit ของทั้ง container** (cgroup `Memory`/`MemorySwap`) ไม่ใช่ JVM heap —
ตั้ง 2048 แปลว่า instance นั้นใช้ RAM ได้ 2048MB รวมทุกอย่าง และ `stats.memory_limit_mb`
จะคืน 2048 ตรงกัน. agent คำนวณ `-Xmx` ให้เองด้วย `runner.HeapMB()` โดยกันส่วนที่ JVM กินนอก heap
(metaspace, code cache, thread stacks, direct buffers, GC) ไว้ ~1/3 ของ limit — floor 256MB,
cap 2048MB, และไม่เกินครึ่งของ limit. เช่น 2048 → heap 1366, 3072 → heap 2048, 8192 → heap 6144

### RAM admission control (create / import / grow)

ตอน create, import และตอนขยาย `memory_mb` (PATCH) control-plane กัน RAM overcommit: ผลรวม
`memory_mb` ของทุก server บน node นั้น (`SumServerMemoryMBOnNode`) + memory ที่ขอ ต้องไม่เกิน
`node.memory_total_mb`. ถ้าเกิน → `400 insufficient_memory` โดย body มี field พิเศษเพิ่มจาก
`{code, message}` ปกติ: `{"code":"insufficient_memory","message":"...","used_mb":N,"total_mb":M,"available_mb":K}`
(`used_mb` = ที่จองอยู่แล้วบน node ไม่รวม instance ที่กำลังเพิ่ม/ขยาย). ตอน PATCH เช็คเฉพาะเมื่อ
ค่าใหม่ > ค่าเดิม และไม่นับ memory เดิมของ server ตัวเอง. ถ้า node ยังไม่รายงาน `memory_total_mb`
(=0/ไม่รู้) → ข้ามเช็ค ไม่บล็อก

### Import server (`POST /api/servers/import`)

รับ server instance เดิม (โลกเดิม/config/plugin) เป็น `.zip` แล้วสร้าง server ใหม่โดย**ไม่โหลด jar**
— body เป็น `multipart/form-data` streaming (control-plane ไม่ buffer ทั้ง zip):

- text parts: `name`, `node_id`, `server_type`, `mc_version`, `memory_mb`, `host_port?`, `accept_eula`
  (validate เหมือน `POST /api/servers` ทุกกฎ: memory_mb ≥256, host_port ว่าง=ไม่ expose ไม่งั้น 1024-65535,
  accept_eula ต้อง true ยกเว้น velocity)
- file part **ชื่อ `archive`** (ต้องเป็น part สุดท้าย): ไฟล์ `.zip` ของ server เดิม (เพดาน 2 GiB)

Flow: control-plane อ่าน zip ทีละก้อน stream เข้า agent เป็น chunked file-write (`FileWriteChunk`)
ไปไว้ที่ `.mcpanel/import.zip` ใน jail ของ server → dispatch NATS job `import_server` ให้ agent
แตก zip (ผ่าน `SafeJoin`/กัน zip-slip) เขียน eula/launch script แล้ว success → status `stopped`
(semantics เดียวกับ `create_server`). staged bytes เป็น file I/O ปกติ, lifecycle command เป็น NATS job.
agent อาจ detect เวอร์ชันจริงจากไฟล์ที่ import แล้วรายงานกลับใน `JobResult.Detail` เป็น JSON
`{"mc_version":"..."}` — เมื่อ import สำเร็จ control-plane จะ update `mc_version` ของ server ตามนั้น
(ถ้าเวอร์ชันผ่านการ validate) ดังนั้น `mc_version` ของ server row อาจเปลี่ยนหลัง import (fetch ใหม่จะเห็น)

- 201 → `{server, job}` (shape เดียวกับ create)
- errors: `invalid_name`/`invalid_server_type`/`invalid_mc_version`/`invalid_memory`/`invalid_host_port`
  (400), `eula_required` (400), `empty_archive` (400, zip ว่าง), `archive_too_large` (413, เกิน 2 GiB),
  `node_not_found` (404), `host_port_taken` (409), `node_offline` (503, agent ไม่ออนไลน์),
  `agent_timeout` (504), `import_failed` (502, agent ปัด chunk), `dispatch_failed` (502)
- audit action: `server_import`

## File manager (ต้อง `can_manage_files` หรือ owner/admin)

interactive file ops วิ่งผ่าน gRPC stream (control-plane → agent, correlate ด้วย request_id) —
ไม่ผ่าน NATS (ไม่ใช่ lifecycle job) และ agent ไม่เปิด port. ทุก path ผ่าน `SafeJoin` (jail = dir ของ server)
`path` เป็น relative ต่อ root ของ server instance (`""`/`/` = root); ไฟล์อ่าน/เขียนจำกัดขนาด (เกินตอบ `file_too_large`)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/api/servers/{id}/files?path=` | → `{path, entries: [{name, is_dir, size, mod_time}]}` (เรียง dir ก่อน) |
| GET | `/api/servers/{id}/files/content?path=` | → `{path, content, truncated}` — content เป็น text (utf-8); 413 `file_too_large` |
| PUT | `/api/servers/{id}/files/content` | `{path, content}` → 204 (เขียน/สร้างไฟล์) |
| POST | `/api/servers/{id}/files/dir` | `{path}` → 201 (สร้างโฟลเดอร์) |
| POST | `/api/servers/{id}/files/rename` | `{from, to}` → 204 |
| DELETE | `/api/servers/{id}/files?path=` | → 204 (ลบไฟล์/โฟลเดอร์) |

error codes: `forbidden` (ไม่มี can_manage_files), `file_not_found`, `file_too_large`, `invalid_path`
(path traversal/นอก jail), `node_offline` (agent ไม่ออนไลน์), `agent_timeout`. mutation ลง audit
(`file_write`, `file_mkdir`, `file_rename`, `file_delete`)

## server.properties (สิทธิ์เท่า file manager: `can_manage_files` หรือ owner/admin)

อ่าน/เขียน `server.properties` ที่ root ของ server instance ผ่าน gRPC stream เดียวกับ file manager
(ไฟล์ไม่มีอยู่ = ถือเป็นว่างเปล่า ไม่ 404). curated catalog ของ key ที่แก้ผ่าน UI ได้ — key อื่นในไฟล์
เก็บไว้ verbatim ใน `extra` และไม่ถูกแตะตอนเขียน (merge รักษา comment/บรรทัดว่าง/ลำดับ key เดิม)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/api/servers/{id}/properties` | → `{fields, values, extra}` |
| PUT | `/api/servers/{id}/properties` | `{values: {<key>: <string>}}` → 204 — ต้อง stopped/errored เท่านั้น (409 `invalid_state`; MC เขียนทับ server.properties ตอน shutdown). GET/read ทำได้ทุกสถานะ |

- `fields`: `[{key, label, type, options, min, max}]` — `type` = `enum`\|`int`\|`bool`\|`string`;
  `options` = array (ว่าง `[]` เมื่อไม่ใช่ enum); `min`/`max` = int หรือ `null`
- `values`: `{<catalog-key>: <string>}` — ค่าที่ parse ได้จากไฟล์ ไม่งั้น default
- `extra`: `{<key>: <string>}` — key ในไฟล์ที่ไม่อยู่ใน catalog (preserve verbatim)
- PUT: ทุก key ใน `values` ต้องเป็น catalog key และ value ผ่าน validate ตาม type (enum ∈ options,
  int แปลงได้ + อยู่ใน [min,max], bool = `"true"`\|`"false"`) — ไม่งั้น 400 `invalid_property` (ระบุ key)
- error อื่น: `forbidden`, `node_offline`, `agent_timeout`. mutation ลง audit (`properties_update`)

## Whitelist / players (สิทธิ์เท่า file manager: `can_manage_files` หรือ owner/admin)

จัดการ whitelist ของ server: control-plane verify username กับ Mojang → เก็บใน DB (`server_players`)
เป็น source of truth → rebuild `whitelist.json` ที่ root ของ server แล้วเขียนผ่าน agent FileWrite (gRPC
stream เดียวกับ file manager, path ผ่าน `SafeJoin` ที่ agent). ถ้า server กำลัง `running` จะส่ง
`whitelist reload` เข้า console ให้ผลทันที (best-effort — ถ้าไม่ running/node offline ข้าม, ไฟล์ apply ตอน start ครั้งหน้า)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/api/servers/{id}/players` | → `{whitelist_enabled, players: [{uuid, username, whitelisted, seen, op, banned, online, playtime_seconds}]}` (unified, เรียงตาม username) |
| POST | `/api/servers/{id}/players` | `{username}` → 201 `{player: {uuid, username, added_at}}` |
| DELETE | `/api/servers/{id}/players/{uuid}` | → 204 |
| POST | `/api/servers/{id}/players/action` | `{action, username}` → `{ok: true}` — สั่งผ่าน console ของ server |

- `action` เป็น allow-list: `op` / `deop` / `kick` / `ban` / `pardon` (ไม่อยู่ในลิสต์ → 400 `invalid_action`).
  `username` ต้องตรง `^[A-Za-z0-9_.*-]{1,32}$` ไม่งั้น 400 `invalid_username` —
  ชื่อถูกต่อเข้าไปในคำสั่ง console ตรง ๆ whitespace/newline จึงเป็น command injection
- ต้อง `status=running` (สั่งผ่าน stdin) ไม่งั้น 409 `invalid_state`; node ติดต่อไม่ได้ → 503 `node_offline`
- `online` มาจาก stats cache (agent อ่านจาก console) ไม่ใช่ไฟล์ — `false` เสมอเมื่อ server ไม่ได้รัน
- `playtime_seconds` อ่านจาก `{level-name}/stats/{uuid}.json` ของ MC (`minecraft:play_time` หรือ
  `minecraft:play_one_minute` หน่วย tick ÷ 20) เฉพาะคนที่ `seen=true` และจำกัด 50 คนต่อ request
  (เกินนั้น/อ่านไม่ได้/ไม่เคยเล่น = `0` = ไม่รู้)

- GET: **unified list** merge จากหลาย source โดย key ด้วย uuid (normalize dash/case):
  DB `server_players` → `whitelisted=true`; `usercache.json` (เคย join) → `seen=true`;
  `ops.json` → `op=true`; `banned-players.json` → `banned=true`. `username` เลือกจากไฟล์ก่อน
  (สะท้อนชื่อปัจจุบัน) fallback ชื่อใน DB. ไฟล์ MC อ่านผ่าน agent FileRead (gRPC เดียวกับ file manager).
  `whitelist_enabled` = ค่า `white-list` ใน server.properties. **Graceful degradation**: ไฟล์ไม่มี
  (server ยังไม่เคย start) = ถือว่าว่าง ไม่ error; **node offline** = ไม่ hard-fail — คืนเฉพาะ DB whitelist
  (`whitelisted=true`, flag อื่น false) + `whitelist_enabled` best-effort (default false)
- POST: `username` trim แล้ว validate 3-16 ตัว `[A-Za-z0-9_]` (ไม่งั้น 400 `invalid_username`) →
  Mojang lookup (ไม่พบ → 404 `player_not_found`; upstream error/timeout → 502 `mojang_unavailable`) →
  ถ้าซ้ำ 409 `player_exists`. `uuid`/`username` ที่เก็บเป็นค่า canonical จาก Mojang
- DELETE: `{uuid}` ต้อง parse ได้ (ไม่งั้น 400 `invalid_request`); ไม่พบ → 404 `not_found`
- error อื่น: `forbidden`, `node_offline`, `agent_timeout`. mutation ลง audit (`player_add`, `player_remove`)
- ⚠️ ต้องตั้ง `white-list=true` ใน server.properties (ผ่าน PUT `/properties`) ถึงจะ enforce whitelist จริง —
  endpoint นี้แค่จัดการรายชื่อ. UUID จาก Mojang ใช้กับ `online-mode=true` เท่านั้น — offline-mode ใช้ UUID
  แบบ derived คนละชุด (whitelist ที่ verify กับ Mojang จะไม่ match)

`server`: `{id, node_id, owner_id, name, server_type, mc_version, memory_mb, host_port, status, created_at, updated_at, stats}`
`stats`: `{cpu_percent, memory_used_mb, memory_limit_mb, net_rx_bps, net_tx_bps, disk_read_bps, disk_write_bps, started_at, online_players, max_players, tps, updated_at}` หรือ `null` — resource monitoring แบบ realtime
ต่อ instance (agent วัดจาก container stats ทุก ~5 วิ, เก็บใน memory ของ control-plane ไม่ลง DB;
`null` เมื่อ server ไม่ได้รันหรือยังไม่มีข้อมูล) มากับทั้ง `GET /api/servers` และ `GET /api/servers/{id}`
(`net_rx_bps`/`net_tx_bps`/`disk_read_bps`/`disk_write_bps` = network/block-I/O rate ของ container หน่วย bytes/sec;
`started_at` = RFC3339 เวลาที่ container รอบปัจจุบันเริ่มรัน ใช้คำนวณ uptime — `null` เมื่อ agent ไม่รู้;
`online_players` = array ชื่อผู้เล่นที่ออนไลน์ (agent อ่านจาก console: คำสั่ง `list` ตอน attach/ทุก 30 วิ
+ บรรทัด joined/left the game ระหว่างนั้น — reply ของคำสั่งที่ agent ยิงเองถูกกรองออกจาก console stream),
`max_players` = 0 เมื่อยังไม่ได้ resync รอบแรก, `tps` = TPS จากคำสั่ง `tps` ของ Paper/Spigot —
`0` เมื่อ server type ไม่มีคำสั่งนี้ (vanilla/fabric/forge))
`job`: `{id, server_id, type, status, error, requested_by_email, requested_by_name, requested_by_username, created_at, started_at, completed_at}`
`requested_by_email`/`requested_by_name`/`requested_by_username`: identity ของ user ที่สั่งงาน
(name = display_name, ทั้งสาม null ถ้า user ถูกลบไปแล้ว — `requested_by` เป็น SET NULL) —
web ประกอบเป็นชื่อที่แสดงเองด้วย `userTitle` (display_name → username → email)

`host_port` ใน PATCH `/api/servers/{id}`: `0` = เลิก expose port ออก host (เข้าถึงได้ผ่าน velocity network เท่านั้น),
`null`/ไม่ส่ง field = ไม่เปลี่ยนค่าเดิม (มีผลตอน start ครั้งถัดไป)

| Method | Path | ใคร | Response |
|---|---|---|---|
| GET | `/api/jobs/{id}` | ผู้มีสิทธิ์เห็น server นั้น | → `{job}` — web ใช้ poll สถานะงาน |
| GET | `/api/meta/server-types` | login แล้ว | → `{types: [{id, label, needs_eula}]}` |
| GET | `/api/meta/versions?type=paper` | login แล้ว | → `{versions: [string]}` ใหม่→เก่า (proxy + cache 10 นาทีจาก Mojang/PaperMC/Fabric API; forge ใช้ promoted builds) |
| GET | `/api/meta/nodes` | login แล้ว | → `{nodes: [{id, name, status}]}` — ข้อมูลขั้นต่ำสำหรับ dropdown ตอนสร้าง server (ตัวเต็มดูได้เฉพาะ admin ที่ `/api/nodes`) |
| GET | `/api/meta/next-port?node_id={uuid}` | login แล้ว | → `{port}` — host_port ว่างต่ำสุดบน node (เริ่ม 25565) สำหรับ prefill ฟอร์มสร้าง server; suggestion เท่านั้นไม่ reserve (node ไม่พบ → 404 `node_not_found`) |

## WebSocket — console

`GET /ws/servers/{id}/console` (upgrade) — auth ด้วย cookie เดียวกัน, ตรวจ Origin, ตรวจสิทธิ์ viewer+
ข้อความเป็น JSON ทั้งสองทาง:

server → client:
```json
{"type": "lines", "lines": ["[12:00:01] [Server thread/INFO]: ..."]}
{"type": "status", "status": "running"}
{"type": "error", "code": "forbidden", "message": "..."}
```

client → server (ต้องมีสิทธิ์เขียน: owner หรือ can_console_write):
```json
{"type": "input", "command": "say hello"}
```

เปิดมาแล้ว server ส่ง history (ring buffer) เป็น `lines` ก้อนแรกก่อน แล้วตามด้วย realtime
ทุก `input` ถูกเขียนลง `audit_logs` (action=`console_command`)

## WebSocket — events (push realtime)

`GET /ws/events` (upgrade) — เส้นเดียวต่อ browser session สำหรับ push update ของ
server/node/stats/jobs เพื่อให้ web **เลิก poll REST** (ไม่มี `refetchInterval` สำหรับข้อมูลพวกนี้).
Handshake auth เหมือน console: ตรวจ Origin ทุก handshake → auth ด้วย cookie `mc_session`
(ตอบ JSON `unauthorized` / `password_change_required` ก่อน upgrade). **Read-only** — server
ไม่รับ input ใด ๆ จาก client (ดูดทิ้งเพื่อจับ ping/pong/close เท่านั้น).

Authorization scope (filter ต่อ event ตาม connection):
- **server event** (`server_status`, `server_stats`, `server_jobs`, `job_update`): admin หรือ `servers.view_all`
  เห็นทุก server; ที่เหลือเห็นเฉพาะ server ที่ตัวเองเข้าถึงได้ (owner หรือมี `server_permissions` row —
  set เดียวกับ `GET /api/servers`). set นี้ refresh ทุก ~15s อัตโนมัติ (server ที่เพิ่งถูก grant/สร้าง
  โผล่โดยไม่ต้อง reconnect)
- **node event** (`node_stats`): เฉพาะ admin หรือ `nodes.view` — คนอื่นไม่ได้รับ
- **server list event** (`server_added`, `server_removed`): broadcast **ไม่ filter** ไปทุก connection
  (payload มีแค่ `server_id` ไม่มีข้อมูล server จริง จึงไม่รั่ว) — client ใช้ signal ว่า list ของ server
  เปลี่ยน (create/import/delete) แล้ว refetch `GET /api/servers` (ซึ่งเช็คสิทธิ์เองอยู่แล้ว)

server → client (JSON, field `type` เป็นตัวแยกชนิด):
```json
{"type": "server_status", "server_id": "<uuid>", "status": "running"}
{"type": "server_stats", "server_id": "<uuid>", "stats": {"cpu_percent": 12.5, "memory_used_mb": 800, "memory_limit_mb": 2048, "net_rx_bps": 1024, "net_tx_bps": 512, "disk_read_bps": 0, "disk_write_bps": 4096, "started_at": "<rfc3339>", "online_players": ["Steve"], "max_players": 20, "tps": 19.98, "updated_at": "<rfc3339>"}}
{"type": "server_stats", "server_id": "<uuid>", "stats": null}
{"type": "server_jobs", "server_id": "<uuid>"}
{"type": "job_update", "server_id": "<uuid>", "job_id": "<uuid>", "job_type": "start_server", "status": "running", "error": "", "restart": false}
{"type": "server_added", "server_id": "<uuid>"}
{"type": "server_removed", "server_id": "<uuid>"}
{"type": "node_stats", "node": {"id": "<uuid>", "name": "...", "status": "online", "agent_version": "...", "os": "...", "arch": "...", "cpu_percent": 3.1, "memory_used_mb": 1024, "memory_total_mb": 16384, "disk_used_mb": 20000, "disk_total_mb": 500000, "net_rx_bps": 2048, "net_tx_bps": 1024, "last_heartbeat_at": "<rfc3339>", "created_at": "<rfc3339>"}}
```
- `server_status`: `status` เป็นค่าเดียวกับ field `status` ของ server (provisioning|stopped|starting|running|stopping|errored|deleting)
- `server_stats`: `stats` เป็น `null` เมื่อ server ไม่ได้ running / ยังไม่มี cache (semantics เดียวกับ field `stats` ของ `GET /api/servers`); เมื่อ running และมีค่าใหม่ push ตัวเลขล่าสุด
- `server_jobs`: signal ว่า job ของ server นี้เปลี่ยน (client refetch `GET /api/servers/{id}/jobs`)
- `job_update`: ความคืบหน้าของ lifecycle job ตัวหนึ่ง — ส่ง 2 จังหวะ: ตอน dispatch
  (`status` = `pending`/`running`, `error` ว่าง) และตอนจบ (`succeeded`/`failed` โดย `failed`
  มีเหตุผลจริงจาก agent ใน `error`; job ที่ค้างเกิน 30 นาทีถูก reaper ปิดก็ส่ง `failed` เหมือนกัน).
  `job_type`/`status` เป็นค่าเดียวกับ field ของ `GET /api/servers/{id}/jobs`.
  `restart: true` = job นี้เป็นขา **stop** ของ restart — สำเร็จแล้วยังไม่จบงาน ขา start
  ตามมาเป็น job ใหม่อีกตัว (client ไม่ควรรายงานว่า restart สำเร็จตรงนี้)
  ต่างจาก `server_jobs` ตรงที่ carry ผลลัพธ์มาด้วย ใช้แจ้ง user ได้เลยโดยไม่ต้อง refetch —
  จำเป็นเพราะ start/stop ที่ล้มบางเคสไม่มี `server_status` ตามมา (รอ heartbeat reconcile)
- `server_added` / `server_removed`: signal ว่า list ของ server เปลี่ยน — `server_added` ส่งตอน
  create/import (row เกิดแล้ว), `server_removed` ส่งตอน delete job สำเร็จ (row ถูกลบจริง);
  client refetch `GET /api/servers` (ส่งแบบ unfiltered ทุก connection — ดู scope ด้านบน)
- `node_stats`: `node` เป็น object รูปแบบเดียวกับ item ของ `GET /api/nodes` แบบ field-for-field

Pattern การใช้ฝั่ง web: โหลด state เริ่มต้นผ่าน REST ตามปกติ แล้วอัปเดตต่อจาก event เหล่านี้;
ตอน reconnect ให้ refetch REST อีกครั้งเพื่อ resync (event ที่หายช่วง disconnect ไม่ durable)

## Audit log actions

`login_success`, `login_failed`, `password_changed`, `profile_updated`, `avatar_updated`, `avatar_removed`,
`user_created`, `user_updated`, `user_delete`, `password_reset`,
`node_created`, `node_deleted`, `server_created`, `server_import`, `server_updated`, `server_deleted`,
`server_action` (detail: start/stop/restart/kill), `permission_updated`, `permission_removed`, `console_command`,
`file_write`, `file_mkdir`, `file_rename`, `file_delete`, `properties_update`, `player_add`, `player_remove`
