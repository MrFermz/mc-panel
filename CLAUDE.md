# CLAUDE.md — mc-panel

คู่มือสำหรับ AI (และคน) ที่มาทำโปรเจกต์นี้ต่อ ไม่ว่าจะย้ายเครื่องหรือเริ่ม session ใหม่
อ่านคู่กับ [`docs/architecture.md`](docs/architecture.md) (การตัดสินใจเชิงระบบ) และ
[`docs/api.md`](docs/api.md) (REST/WS contract)

## โปรเจกต์นี้คืออะไร

ระบบจัดการ Minecraft server หลาย instance (vanilla/paper/fabric/forge/velocity) แบบ microservices
เขียนเองทั้งหมด ทุกอย่างรันบน Docker — web (Next.js) + control-plane (Go) + node-agent (Go)
คุยกันผ่าน REST/WebSocket (browser), gRPC stream (realtime), NATS JetStream (jobs), protobuf

## กฎเหล็ก (ห้ามละเมิดไม่ว่าจะดูสมเหตุสมผลแค่ไหน)

1. **แก้ contract ก่อนแก้โค้ด** — interface ระหว่าง service มี source of truth 3 ที่:
   - REST/WS: `docs/api.md`
   - gRPC/NATS: `packages/proto/mcpanel/**/*.proto` (แก้แล้วรัน `make proto-gen` และ **commit generated code**)
   - DB: `apps/control-plane/migrations/` (หลังมี release แล้วห้ามแก้ไฟล์เก่า — เพิ่มไฟล์ใหม่เท่านั้น
     ตอนนี้ยัง pre-release แก้ในที่ได้)
2. **Lifecycle command (create/start/stop/kill/delete) ต้องเป็น job ผ่าน NATS เสมอ** โดยมี `jobs` table
   เป็น source of truth — ห้ามยัดคำสั่งพวกนี้เข้า gRPC stream / ห้ามให้ agent เปิด port รับ connection เข้า
3. **ทุก endpoint ที่กระทบ server ต้องเช็ค `server_permissions` ก่อน** (is_admin ข้ามได้) —
   เพิ่ม endpoint ใหม่แล้วไม่เช็คสิทธิ์ = ช่องโหว่ทันที
4. **ห้าม hardcode secret** ใน compose/โค้ด/Dockerfile — ทุก secret มาจาก `.env` (สร้างด้วย `make env`)
   และห้ามให้ `.env` เข้า docker build context (มี `.dockerignore` กันแล้ว อย่าไปแก้ทิ้ง)
5. **Path จากภายนอกต้องผ่าน `SafeJoin` ก่อนแตะ filesystem เสมอ** โดยเฉพาะก่อน `RemoveAll`
6. **อย่า fork/พึ่ง panel หรือ runtime image สำเร็จรูป** (Pterodactyl, itzg ฯลฯ) — เขียนเอง 100%,
   jar โหลดจาก official source เท่านั้น (Mojang/PaperMC/FabricMC/Forge maven) + verify checksum เมื่อมี
7. **อย่าเปลี่ยน** bind mount → named volume, PostgreSQL → NoSQL, และอย่าเพิ่ม Windows/native runner
   (scope คือ full Docker บน Linux)
8. **EULA ห้าม default เป็น true** — user ต้องติ๊กเอง
9. Frontend **ห้ามเก็บ token เอง** (ไม่มี localStorage/sessionStorage สำหรับ auth) — cookie HttpOnly
   จาก control-plane เท่านั้น และห้ามใช้ next-auth

## โครงสร้าง repo + แผน submodules

```
apps/control-plane   Go — module github.com/mc-panel/control-plane
apps/node-agent      Go — module github.com/mc-panel/node-agent
apps/web             Next.js (self-contained ไม่ import ข้าม directory)
packages/proto       .proto + generated Go — module github.com/mc-panel/proto
packages/shared-types  TS generate จาก proto (ยังไม่ถูกใช้โดย web — เผื่ออนาคต)
infra                compose ทั้งสองชุด, caddy, nats config, mc-runtime image
```

หลักการ: **แต่ละ directory ต้องแยกเป็น git repo ได้โดยแก้น้อยที่สุด** —
- app ห้าม import โค้ดของ app อื่น ข้ามได้เฉพาะ `packages/proto` (ผ่าน go module + `replace ../../packages/proto`)
- ตอนแยกจริง: ลบ `replace` ใน go.mod → ใช้ version tag ของ repo proto, ตั้ง root repo ถือ submodules + Makefile + infra
- docker build context: control-plane/node-agent ใช้ **repo root** (ต้องเห็น packages/proto), web ใช้ `apps/web` เอง

## คำสั่งหลัก

```bash
make env              # generate .env ครั้งแรก (secret สุ่มทั้งหมด)
make up / down / logs # full stack บน docker (build ให้เอง)
make admin-password   # ดู initial admin credentials จาก log
make runtime-images   # build mcpanel/mc-runtime:8/17/21/25 (ต้องทำก่อน start MC server ครั้งแรก)
make bootstrap        # dev ครั้งแรก: infra + migrate + web deps
make run-control-plane / run-agent / run-web   # dev hot-loop 3 terminals
make test / lint      # ต้องผ่านก่อนถือว่างานเสร็จ
make proto-gen        # หลังแก้ .proto (generated code ต้อง commit)
```

Verify งาน Go: `go build ./apps/...` + `go vet` + `go test` จาก repo root (มี go.work)
และแต่ละ app ต้อง build ได้แบบ standalone ด้วย: `cd apps/<app> && GOWORK=off go mod tidy && GOWORK=off go build ./...`
(นี่คือสิ่งที่ docker build ทำจริง — ผ่านแค่ go.work ไม่พอ)
Verify web: `cd apps/web && pnpm build && pnpm lint`

## Conventions

- **Comment ภาษาไทย** เขียนเฉพาะจุดที่อธิบาย "ทำไม" หรือ constraint ที่โค้ดบอกเองไม่ได้ —
  ห้าม comment เล่าว่าบรรทัดถัดไปทำอะไร / **log message ภาษาอังกฤษ** (เพื่อ grep + log tooling)
- Identifier/ชื่อไฟล์ภาษาอังกฤษทั้งหมด
- HTTP error ตอบ `{"code": "snake_case_code", "message": "..."}` — code ที่ web ผูก logic ไว้:
  `unauthorized`, `password_change_required`, `forbidden`, `rate_limited` (ดูครบใน docs/api.md)
- Go: stdlib + chi/pgx/gorilla — SQL เขียนตรง ๆ ใน `internal/store` ไม่ใช้ ORM;
  ห้ามเพิ่ม dependency ใหญ่โดยไม่มีเหตุผลใน commit/PR message
- Web: Next.js App Router, shadcn/ui (component อยู่ `components/ui` — generate/vendor ตามสไตล์ shadcn),
  react-query สำหรับ server state, zod schema ใน `lib/types.ts` ต้อง sync กับ docs/api.md
- **Realtime push**: server/node/stats/jobs update วิ่งผ่าน events WS `/ws/events` (browser เปิดเส้นเดียว,
  control-plane push จาก hook ใน `internal/agenthub` + `internal/jobs` → `internal/events.Hub`) —
  **ห้ามเพิ่ม `refetchInterval` poll REST สำหรับข้อมูลพวกนี้** (โหลด state เริ่มต้นด้วย REST ครั้งเดียว
  แล้วรับ update ต่อจาก WS, resync ด้วย refetch ตอน reconnect). เพิ่ม push event ใหม่ = emit ที่ agent
  gRPC hook point (agenthub) หรือ job result (jobs) เข้า `events.Hub` แล้ว document ใน docs/api.md
  - **server list change**: `server_added` (emit ตอน create/import ใน httpapi) / `server_removed`
    (emit ตอน delete job สำเร็จใน jobs) broadcast แบบ unfiltered (payload มีแค่ server_id) →
    web invalidate `["servers"]` refetch (dashboard เพิ่ม/เอา instance ออกเองแบบ realtime)
- Proto: package `mcpanel.<x>.v1`, directory ต้องตรง package (buf lint STANDARD บังคับ),
  breaking change ต้องขึ้น v2 ไม่แก้ v1
- NATS subjects: `mcpanel.jobs.{node_id}` (JobEnvelope), `mcpanel.results` (JobResult) —
  stream `JOBS` (WorkQueue) / `RESULTS`, consumer สร้างโดย control-plane เท่านั้น
  (NATS user ของ agent ไม่มีสิทธิ์สร้าง — ดู `infra/nats/nats-server.conf`)
- Docker: MC container ชื่อ `mc-{server_id}`, label `mc.managed_by=mc-panel-agent`,
  ข้อมูลอยู่ `{MC_DATA_DIR}/{server_id}` bind mount เป็น `/mc`
- เวลาแก้ Makefile: จำไว้ว่า buf/goose เรียกผ่าน `go run` (ไม่ assume ว่าติดตั้งไว้)

## Flow สำคัญที่ต้องเข้าใจก่อนแก้

**Initial user**: users ว่าง → control-plane boot สร้าง admin (`ADMIN_EMAIL` + password สุ่ม 20 ตัว
พิมพ์ลง log ครั้งเดียว, `must_change_password=true`) → login แล้วทุก endpoint ตอบ
`403 password_change_required` จน user เปลี่ยน password (bump `token_version` → JWT เก่าตายหมด)
Admin สร้าง user ใหม่/reset password ก็ flow เดียวกัน — API ตอบ `initial_password` ครั้งเดียว
(ตอนสร้าง user: `email` เป็น optional ได้ถ้าตั้ง `username` — ต้องมีอย่างน้อยหนึ่งอย่าง, login รับ
email-or-username; `users.email` เป็น nullable, username-only account จะได้ `user.email = ""`)

**ตั้ง/เปลี่ยน/รีเซ็ต admin password** (สรุป: password เก็บใน **Postgres ที่เดียว** — คอลัมน์
`users.password_hash` เป็น bcrypt; Redis/NATS ไม่เกี่ยวกับ password เลย):

1. **รู้ password ปัจจุบัน + อยากเปลี่ยนเอง** → login แล้วไปหน้า `/change-password` (หรือ
   `POST /api/auth/change-password {current_password, new_password}`). **ไม่ต้องต่อ DB ใด ๆ**
2. **admin อยากรีเซ็ตให้ user คนอื่น (หรือรีเซ็ตตัวเองผ่าน UI)** → `/admin/users` → ปุ่ม Reset password
   (หรือ `POST /api/users/{id}/reset-password`) → ได้ password สุ่มแสดงครั้งเดียว → คนนั้น login แล้วถูกบังคับตั้งใหม่
   **ไม่ต้องต่อ DB** (ทำผ่าน API ที่ต้อง login เป็น admin อยู่แล้ว)
3. **ลืม password admin จน login ไม่ได้เลย** (ไม่มี admin คนอื่นช่วย reset) → ใช้ CLI ของ control-plane
   ที่ต่อ **แค่ Postgres** (ไม่ต้องรู้/ต่อ Redis, NATS): รันใน container ที่มี `DATABASE_URL` อยู่แล้ว
   ```bash
   make admin-reset-password                    # full stack: exec เข้า control-plane container
   # หรือรันตรง:
   docker compose -f infra/docker-compose.yml exec control-plane /control-plane -reset-admin-password
   # เจาะจง email อื่น: ... -reset-admin-password -email=someone@example.com
   ```
   มันสุ่ม password ใหม่ + `must_change_password=true` + bump `token_version` (session เก่าตายหมด)
   แล้วพิมพ์ password ครั้งเดียว (ถ้า email นั้นยังไม่มีในระบบจะสร้างใหม่เป็น admin ให้เลย)
   > กรณี dev (service รันนอก docker): `go run ./apps/control-plane/cmd/server -reset-admin-password`
   > โดย set `DATABASE_URL` ให้ชี้ dev postgres (ดู `make run-control-plane` เป็นตัวอย่าง env)

**ถามบ่อย: ต้องมี connection ของ DB ทุกตัวไหมถึงจะเข้าใช้/รีเซ็ตได้?** — ไม่ต้อง.
เข้าเว็บ/login/เปลี่ยน password ปกติ ใช้แค่ผ่าน control-plane (คนใช้ไม่แตะ DB ตรง ๆ อยู่แล้ว).
การรีเซ็ต password ยุ่งกับ **Postgres อย่างเดียว** — Redis เป็นแค่ login rate-limit (fail-open ถ้าล่ม),
NATS เป็นแค่ job transport ไม่เกี่ยวกับ auth. ทั้ง Postgres/Redis/NATS อยู่ network `core` (internal)
เข้าจากภายนอกไม่ได้ ต้อง `docker compose exec` เข้า container หรือใช้ CLI ข้างบนที่รันในวงเดียวกัน

**สร้าง server**: POST /api/servers → insert แถว (status=provisioning) + job `create_server` → agent
โหลด jar + เขียน eula/launch script → JobResult → status=stopped → user สั่ง start ต่อ
(import server: job `import_server` — agent อาจ detect เวอร์ชันจริงแล้วรายงานใน `JobResult.Detail` JSON
`{"mc_version":"..."}` → control-plane update `mc_version` ของ server ให้ ถ้าเวอร์ชันผ่าน validate)

**Start**: job `start_server` (control-plane เลือก `mcpanel/mc-runtime:{8|17|21|25}` จาก mc_version) →
agent **ensure runtime image** (มีในเครื่องแล้ว = reuse ไม่โหลดซ้ำ; ไม่มี = pull `eclipse-temurin:{ver}-jre`
จาก official แล้ว tag เป็น `mcpanel/mc-runtime:{ver}` cache ไว้ share ข้าม instance) → สร้าง container
(hardening ครบ: cap-drop ALL, no-new-privileges, user 1000, mem limit, แยก network)
→ docker events → agent ส่ง `ServerStatus RUNNING` ผ่าน gRPC → DB + broadcast WS
- ถ้า start ล้มหลังสร้าง container / container crash (die exit≠0 ที่ไม่ได้สั่ง stop) → agent **ลบ container
  ที่ค้างทิ้งทันที + push console line แจ้ง user** ว่ากำลังเอาออก (ไม่ปล่อยให้ค้างเป็น dead container)
- runtime image cache: `mcpanel/mc-runtime:{8|17|21|25}` build เองด้วย `make runtime-images` ก็ได้ (hardened)
  หรือปล่อยให้ agent auto-pull ครั้งแรกที่ต้องใช้ — reuse ตัวที่มีเสมอ ไม่โหลดซ้ำ

**Import server**: POST /api/servers/import (`multipart/form-data`, ต้องมี cap `servers.create`) →
control-plane อ่าน `.zip` แบบ streaming (ไม่ buffer ทั้งก้อน — อ่านทีละ ~768KiB look-ahead หนึ่งก้อน)
แล้ว stream เข้า agent เป็น chunked `FileWriteChunk` ไปไว้ `.mcpanel/import.zip` ใน jail ของ server
(bytes-over-gRPC = file I/O ปกติ เหมือน file manager ไม่ใช่ lifecycle) → insert แถว (status=provisioning)
+ dispatch NATS job `import_server` (lifecycle command เป็น job ตามกฎ #2) → agent แตก zip ด้วย
`SafeJoin`/กัน zip-slip **โดยไม่โหลด jar** → JobResult success → status=stopped (semantics เดียวกับ
`create_server`, มี handling ใน `planTransition`/`reapPlan`). staging ล้ม = ลบ row ที่เพิ่งสร้างทิ้ง,
dispatch ล้ม = mark errored. audit `server_import`

**Resource monitoring ต่อ instance**: agent วัด container stats ทุก ~5 วิ → gRPC `ServerStats`
→ control-plane เก็บ in-memory cache (ไม่ลง DB — ephemeral) → แนบใน field `stats` ของ server response
→ web แสดง CPU/RAM ต่อ instance (dashboard + หน้า detail). `stats` มี network/block-I/O rate ด้วย
(`net_rx_bps`/`net_tx_bps`/`disk_read_bps`/`disk_write_bps` bytes/sec) และ node stats มี `net_rx_bps`/`net_tx_bps`
(เก็บใน nodes row เหมือน cpu/mem/disk, มาจาก heartbeat) — ทั้งคู่ push ผ่าน `server_stats`/`node_stats` WS ด้วย

**Global capability (RBAC ระดับ panel)**: คนละชั้นกับ `server_permissions` (ต่อ server) — `users.capabilities`
เป็น key array ที่ admin ตั้งต่อ user ให้เข้าถึงหน้า/เมนู + CRUD ได้ (`users.manage`, `nodes.manage`,
`servers.create`, `servers.view_all`). is_admin ครอบทุก capability. web แสดงเมนู/ปุ่มตาม effective cap,
backend บังคับทุก endpoint (is_admin OR capability). catalog อยู่ที่ `GET /api/meta/capabilities`

**Console**: agent attach stdout/stderr ของ container → batch ~100ms → gRPC `ConsoleOutput` →
control-plane เก็บ ring buffer 500 บรรทัด + broadcast WS → browser (xterm.js)
ขาเข้า: WS `{"type":"input"}` → เช็ค `can_console_write` ต่อ message → audit → gRPC `ConsoleInput` → stdin

**Agent identity**: agent มีแค่ token → gRPC auth (SHA-256 lookup) → control-plane ส่ง `Welcome{node_id}`
→ agent ค่อยเปิด NATS consumer `agent-{node_id}` ได้

## Security posture (จุดที่ต้องระวังเวลาแก้)

- docker.sock ใน node-agent = จุดเสี่ยงสุดของระบบ ทุก input จาก job ต้อง validate ก่อนถึง docker API
- Postgres/Redis/NATS อยู่ network `core` (internal, ไม่มี egress) — อย่า publish port / อย่าย้าย MC
  container เข้า core เด็ดขาด (MC container อยู่ `mcpanel-servers` เท่านั้น)
- dev compose bind 127.0.0.1 ทุก port — อย่าเปลี่ยนเป็น 0.0.0.0
- WS ต้องเช็ค Origin ทุก handshake / cookie เป็น SameSite=Lax / GET ห้ามมี side effect (นี่คือแนวกัน CSRF)
- Backlog ด้าน security ที่รู้อยู่แล้ว (อย่าทำเงียบ ๆ ให้ปรึกษาก่อน): TLS ของ gRPC/NATS ข้ามเครื่อง,
  NKey/JWT ต่อ node, docker-socket-proxy, readonly rootfs ของ MC container, 2FA

**File manager**: interactive file ops (list/read/write/mkdir/rename/delete) วิ่งผ่าน **gRPC stream**
(control-plane→agent `FileRequest`, agent→control-plane `FileResponse`, correlate ด้วย request_id) —
ไม่ผ่าน NATS (ไม่ใช่ lifecycle job) และ agent ไม่เปิด port. ทุก path ผ่าน `SafeJoin` (jail = dir ของ server)
ก่อนแตะ filesystem. ต้องมี `can_manage_files` (หรือ owner/admin). REST อยู่ `/api/servers/{id}/files*` (ดู docs/api.md)

**Whitelist/players**: control-plane verify username กับ Mojang (`internal/mojang`, egress ผ่าน edge) →
เก็บใน DB `server_players` (source of truth) → rebuild `whitelist.json` ที่ root ของ server แล้วเขียนผ่าน
agent FileWrite (stream เดียวกับ file manager, SafeJoin ที่ agent) → ถ้า running ส่ง `whitelist reload`
เข้า console (best-effort). สิทธิ์เท่า file manager. REST `/api/servers/{id}/players*`. ⚠️ ต้อง `white-list=true`
ใน server.properties ถึงจะ enforce จริง; UUID จาก Mojang ใช้กับ `online-mode=true` เท่านั้น (offline-mode คนละ UUID)
- **GET players = unified list**: merge DB whitelist + `usercache.json` (seen) + `ops.json` (op) +
  `banned-players.json` (banned) โดย key ด้วย uuid (normalize dash/case) — อ่านไฟล์ผ่าน agent FileRead.
  ไฟล์ไม่มี = ว่าง (ไม่ error); **node offline = degrade** เหลือ DB whitelist (แท็บยังใช้ได้ตอน server หยุด)
- **Access picker**: `GET /api/users/directory` (authed ทุกคน ไม่ใช่ `users.manage`) คืน user ที่ active
  แบบ field เบา ให้ owner เลือก collaborator; POST permission รับ `user_id` (จาก directory) หรือ `email`

**Properties แก้ได้เฉพาะ stopped/errored**: PUT `/properties` ตอบ 409 `invalid_state` ถ้า server ไม่หยุด
(MC เขียนทับ server.properties ตอน shutdown — แก้ตอนรันจะหาย); GET/read ทำได้ทุกสถานะ

**RAM admission control**: create/import/ขยาย `memory_mb` เช็คผลรวม `memory_mb` ทุก server บน node
(`SumServerMemoryMBOnNode`) + ตัวใหม่ ต้องไม่เกิน `node.memory_total_mb` → ไม่งั้น 400 `insufficient_memory`
(body มี `used_mb/total_mb/available_mb`). node total=0/ไม่รู้ = ข้ามเช็ค. PATCH เช็คเฉพาะตอนขยาย (ไม่นับ memory เดิมตัวเอง)

**Default host port**: `GET /api/meta/next-port?node_id=` คืน host_port ว่างต่ำสุดบน node (เริ่ม 25565)
ให้ web prefill ฟอร์มสร้าง server — suggestion เท่านั้น ไม่ reserve (create ยัง enforce UNIQUE (node_id, host_port))

## สิ่งที่ยังไม่มี (อย่าเข้าใจผิดว่ามีแล้ว)

- File manager: อัปโหลด/ดาวน์โหลดไฟล์ใหญ่แบบ binary (jar/mod) ใน UI ทั่วไป — ตอนนี้รองรับ browse/แก้ไฟล์ text/mkdir/rename/delete
  (binary upload มีเฉพาะ flow import server: zip ผ่าน chunked `FileWriteChunk` เข้า `.mcpanel/import.zip` เท่านั้น)
- Backup/restore, scheduler, online-player list/query (มี whitelist management แล้ว), mod/plugin browser
- OIDC (Discord/Google), quota ต่อ user, multi-node TLS
- Metrics/alerting ระยะยาว (มี resource monitoring ต่อ instance แบบ realtime แล้ว แต่ไม่เก็บ history/ไม่มี alert)
