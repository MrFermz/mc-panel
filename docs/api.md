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

`user` object: `{id, email, username, display_name, is_admin, is_active, must_change_password, capabilities, created_at}`
(`username` = string หรือ `null` — optional login identifier)
password policy: ยาว ≥ 10 ตัวอักษร (เช็คทั้ง web และ server)

### Capabilities (global RBAC — แยกจาก server_permissions)

`capabilities` = array ของ key ที่ admin ตั้งให้ user (is_admin ครอบทุก capability โดยปริยาย)
เมนู/หน้า/ปุ่มฝั่ง web แสดงตาม **effective capability** = `is_admin ? ทั้งหมด : capabilities`
backend บังคับทุก endpoint: ผ่านเมื่อ `is_admin` **หรือ** มี capability ที่กำหนด

| key | เข้าถึงอะไร |
|---|---|
| `users.manage` | หน้า/เมนู Users + CRUD user (`/api/users*`) |
| `nodes.manage` | หน้า/เมนู Nodes + CRUD node (`/api/nodes*`) |
| `servers.create` | สร้าง server ใหม่ (`POST /api/servers`) |
| `servers.view_all` | เห็น server ทุกตัว (เหมือน admin) ไม่จำกัดเฉพาะที่มี server_permission |

| Method | Path | ใคร | Response |
|---|---|---|---|
| GET | `/api/meta/capabilities` | login แล้ว | → `{capabilities: [{key, label, description}]}` — catalog สำหรับหน้า admin |

## Users (ต้อง `users.manage` หรือ is_admin)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/api/users` | `?search=&role=&status=` → `{users: [user]}` — filter: `search` (email/username/display_name substring), `role` (`admin`\|`user`), `status` (`active`\|`inactive`); user ที่ถูกลบ (soft delete) ไม่แสดง |
| POST | `/api/users` | `{email, username?, display_name, is_admin, capabilities?}` → 201 `{user, initial_password}` — password สุ่ม 20 ตัว แสดงครั้งเดียว; `username` (optional) ต้อง match `^[a-zA-Z0-9_.-]{3,64}$` ไม่งั้น 400 `invalid_request`; 409 `email_exists` / `username_exists` เมื่อซ้ำ |
| PATCH | `/api/users/{id}` | `{display_name?, is_admin?, is_active?, capabilities?}` → `{user}` — ห้ามถอด admin/ปิด active ตัวเอง; `capabilities` ต้องเป็น key ใน catalog เท่านั้น |
| DELETE | `/api/users/{id}` | → 204 — soft delete (mark `deleted_at` + ปิด active + bump token_version → session เก่าตาย + ล้าง server_permissions); email/username ที่ถูกลบใช้ซ้ำได้; 400 `cannot_delete_self`, 404 `not_found` |
| POST | `/api/users/{id}/reset-password` | → `{initial_password}` — สุ่มใหม่ + must_change_password + bump token_version |

## Nodes (ต้อง `nodes.manage` หรือ is_admin)

| Method | Path | Body → Response |
|---|---|---|
| GET | `/api/nodes` | → `{nodes: [node]}` — node รวม stats ล่าสุดจาก heartbeat |
| POST | `/api/nodes` | `{name}` → 201 `{node, token}` — token format `<node_id>.<secret>` แสดงครั้งเดียว |
| DELETE | `/api/nodes/{id}` | → 204 — ลบได้เฉพาะ node ที่ไม่มี server |

`node`: `{id, name, status, agent_version, os, arch, cpu_percent, memory_used_mb, memory_total_mb, disk_used_mb, disk_total_mb, last_heartbeat_at, created_at}`

## Servers

สิทธิ์: `is_admin` เห็น/ทำได้ทุกอย่าง | นอกนั้นดูตาม `server_permissions.role`
- `owner` — ทุกอย่างของ server ตัวเอง รวมจัดการ permission
- `operator` — start/stop/restart/kill, ดู console, พิมพ์ console ถ้า `can_console_write`
- `viewer` — ดูอย่างเดียว (status + console read)

user ที่มี capability `servers.create` (หรือ is_admin) สร้าง server ได้ — คนสร้างได้ role `owner` อัตโนมัติ

| Method | Path | ใคร | Body → Response |
|---|---|---|---|
| GET | `/api/servers` | login แล้ว | → `{servers: [server]}` เฉพาะที่มีสิทธิ์เห็น |
| POST | `/api/servers` | login แล้ว | `{name, node_id, server_type, mc_version, memory_mb, host_port?, accept_eula}` → 201 `{server, job}` — ต้อง accept_eula=true ยกเว้น velocity |
| GET | `/api/servers/{id}` | viewer+ | → `{server, permissions: [permission+user_email]}` |
| PATCH | `/api/servers/{id}` | owner/admin | `{name?, memory_mb?, host_port?}` → `{server}` (มีผลตอน start ครั้งถัดไป) |
| DELETE | `/api/servers/{id}` | owner/admin | → `{job}` — สั่งงาน delete (ลบ container + ข้อมูลทั้งหมด) |
| POST | `/api/servers/{id}/actions` | operator+ | `{action: "start"\|"stop"\|"restart"\|"kill"}` → `{job}` |
| GET | `/api/servers/{id}/jobs` | viewer+ | `?limit=20` → `{jobs: [job]}` |
| GET | `/api/servers/{id}/console/history` | viewer+ | → `{lines: [string]}` (ring buffer 500 บรรทัดล่าสุด) |
| GET | `/api/servers/{id}/permissions` | owner/admin | → `{permissions: [{user_id, email, display_name, role, can_console_write, can_manage_files}]}` |
| POST | `/api/servers/{id}/permissions` | owner/admin | `{email, role, can_console_write, can_manage_files}` → upsert โดย resolve email→user (404 `user_not_found` ถ้าไม่มี) |
| DELETE | `/api/servers/{id}/permissions/{user_id}` | owner/admin | → 204 — ห้ามลบ owner คนสุดท้าย |

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
| PUT | `/api/servers/{id}/properties` | `{values: {<key>: <string>}}` → 204 |

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
| GET | `/api/servers/{id}/players` | → `{players: [{uuid, username, added_at}]}` (เรียงตาม added_at) |
| POST | `/api/servers/{id}/players` | `{username}` → 201 `{player: {uuid, username, added_at}}` |
| DELETE | `/api/servers/{id}/players/{uuid}` | → 204 |

- POST: `username` trim แล้ว validate 3-16 ตัว `[A-Za-z0-9_]` (ไม่งั้น 400 `invalid_username`) →
  Mojang lookup (ไม่พบ → 404 `player_not_found`; upstream error/timeout → 502 `mojang_unavailable`) →
  ถ้าซ้ำ 409 `player_exists`. `uuid`/`username` ที่เก็บเป็นค่า canonical จาก Mojang
- DELETE: `{uuid}` ต้อง parse ได้ (ไม่งั้น 400 `invalid_request`); ไม่พบ → 404 `not_found`
- error อื่น: `forbidden`, `node_offline`, `agent_timeout`. mutation ลง audit (`player_add`, `player_remove`)
- ⚠️ ต้องตั้ง `white-list=true` ใน server.properties (ผ่าน PUT `/properties`) ถึงจะ enforce whitelist จริง —
  endpoint นี้แค่จัดการรายชื่อ. UUID จาก Mojang ใช้กับ `online-mode=true` เท่านั้น — offline-mode ใช้ UUID
  แบบ derived คนละชุด (whitelist ที่ verify กับ Mojang จะไม่ match)

`server`: `{id, node_id, owner_id, name, server_type, mc_version, memory_mb, host_port, status, created_at, updated_at, stats}`
`stats`: `{cpu_percent, memory_used_mb, memory_limit_mb, updated_at}` หรือ `null` — resource monitoring แบบ realtime
ต่อ instance (agent วัดจาก container stats ทุก ~5 วิ, เก็บใน memory ของ control-plane ไม่ลง DB;
`null` เมื่อ server ไม่ได้รันหรือยังไม่มีข้อมูล) มากับทั้ง `GET /api/servers` และ `GET /api/servers/{id}`
`job`: `{id, server_id, type, status, error, requested_by_email, created_at, started_at, completed_at}`
`requested_by_email`: อีเมลของ user ที่สั่งงาน (null ถ้า user ถูกลบไปแล้ว — `requested_by` เป็น SET NULL)

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
- **server event** (`server_status`, `server_stats`, `server_jobs`): admin หรือ `servers.view_all`
  เห็นทุก server; ที่เหลือเห็นเฉพาะ server ที่ตัวเองเข้าถึงได้ (owner หรือมี `server_permissions` row —
  set เดียวกับ `GET /api/servers`). set นี้ refresh ทุก ~15s อัตโนมัติ (server ที่เพิ่งถูก grant/สร้าง
  โผล่โดยไม่ต้อง reconnect)
- **node event** (`node_stats`): เฉพาะ admin หรือ `nodes.manage` — คนอื่นไม่ได้รับ

server → client (JSON, field `type` เป็นตัวแยกชนิด):
```json
{"type": "server_status", "server_id": "<uuid>", "status": "running"}
{"type": "server_stats", "server_id": "<uuid>", "stats": {"cpu_percent": 12.5, "memory_used_mb": 800, "memory_limit_mb": 2048, "updated_at": "<rfc3339>"}}
{"type": "server_stats", "server_id": "<uuid>", "stats": null}
{"type": "server_jobs", "server_id": "<uuid>"}
{"type": "node_stats", "node": {"id": "<uuid>", "name": "...", "status": "online", "agent_version": "...", "os": "...", "arch": "...", "cpu_percent": 3.1, "memory_used_mb": 1024, "memory_total_mb": 16384, "disk_used_mb": 20000, "disk_total_mb": 500000, "last_heartbeat_at": "<rfc3339>", "created_at": "<rfc3339>"}}
```
- `server_status`: `status` เป็นค่าเดียวกับ field `status` ของ server (provisioning|stopped|starting|running|stopping|errored|deleting)
- `server_stats`: `stats` เป็น `null` เมื่อ server ไม่ได้ running / ยังไม่มี cache (semantics เดียวกับ field `stats` ของ `GET /api/servers`); เมื่อ running และมีค่าใหม่ push ตัวเลขล่าสุด
- `server_jobs`: signal ว่า job ของ server นี้เปลี่ยน (client refetch `GET /api/servers/{id}/jobs`)
- `node_stats`: `node` เป็น object รูปแบบเดียวกับ item ของ `GET /api/nodes` แบบ field-for-field

Pattern การใช้ฝั่ง web: โหลด state เริ่มต้นผ่าน REST ตามปกติ แล้วอัปเดตต่อจาก event เหล่านี้;
ตอน reconnect ให้ refetch REST อีกครั้งเพื่อ resync (event ที่หายช่วง disconnect ไม่ durable)

## Audit log actions

`login_success`, `login_failed`, `password_changed`, `user_created`, `user_updated`, `user_delete`, `password_reset`,
`node_created`, `node_deleted`, `server_created`, `server_updated`, `server_deleted`,
`server_action` (detail: start/stop/restart/kill), `permission_updated`, `permission_removed`, `console_command`,
`file_write`, `file_mkdir`, `file_rename`, `file_delete`, `properties_update`, `player_add`, `player_remove`
