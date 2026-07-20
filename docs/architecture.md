# สถาปัตยกรรม mc-panel

## Scope
- Linux-first, **full Docker**: ทั้งระบบหลักและทุก MC instance รันบน Docker ทั้งหมด
- Native process runner อยู่นอก scope ปัจจุบัน (interface `Runner` ยังเผื่อไว้ แต่ implement เฉพาะ Docker)
- รองรับ Vanilla, Paper (plugin), Fabric/Forge (modded), Velocity (proxy) ผ่าน abstraction เดียวกัน
- Auth เป็น local อย่างเดียว (username+password — ระบบไม่เก็บ email เลย) — OIDC เป็น backlog

## 3 ชั้นหลัก
1. **Control plane** — API server (Go), stateless, ถือ DB/Redis/NATS
2. **Node agent** — Go binary ใน container, connect ออกไปหา control plane เอง (ไม่เปิด port รับเข้า)
3. **Node layer** — docker daemon ของเครื่องที่รัน MC containers

## เส้นทางข้อมูล (transport แต่ละแบบมีหน้าที่ตายตัว — ห้ามสลับ)

```
Browser ── WebSocket /ws/servers/{id}/console (JSON) ──> Control plane   console I/O ต่อ server
Browser ── WebSocket /ws/events (JSON) <── Control plane                 push server/node/stats/jobs realtime
Browser ── REST /api (JSON) ──> Control plane          CRUD + โหลด state เริ่มต้น
Agent   ── gRPC stream (protobuf) ──> Control plane    heartbeat, console I/O, server status/stats (realtime)
Control plane ── NATS JetStream (protobuf) ──> Agent   jobs: create/start/stop/kill/delete (durable)
Agent   ── NATS mcpanel.results ──> Control plane      ผลลัพธ์ของ job
```

### Realtime push model (`/ws/events`)

Browser เปิด WebSocket **เส้นเดียว** `/ws/events` ต่อ session แล้ว control-plane **push**
update ให้เอง — web **ไม่ poll REST** สำหรับข้อมูลพวกนี้ (ไม่มี `refetchInterval`).
control-plane emit event จาก hook จุดเดียวกับที่ข้อมูลจริงเปลี่ยน:
- `server_status` — จาก agent gRPC (`agenthub.setServerStatus`) และ job result (`jobs.afterCommit`/reaper) ทุกครั้งที่สถานะ server เปลี่ยน (emit คู่กับ console `BroadcastStatus` เดิม)
- `server_stats` — จาก agent gRPC `ServerStats` (~ทุก 5s) หลังเขียน stats cache; `stats:null` เมื่อ server ไม่ได้ running (semantics เดียวกับ field `stats` ของ REST)
- `node_stats` — จาก agent heartbeat (1 DB read/heartbeat โหลด node row) และ transition เป็น offline (disconnect / stale checker)
- `server_jobs` — เมื่อ JobResult ของ server ถูก apply (client refetch jobs ของ server นั้น)

Authorization: filter ต่อ connection — accessible-server set (owner/`server_permissions`,
หรือเห็นหมดถ้า admin/`servers.view_all`) refresh ทุก ~15s ให้ server ที่เพิ่ง grant โผล่เอง;
`node_stats` เห็นเฉพาะ admin/`nodes.view`. State เริ่มต้นยังโหลดผ่าน REST ตามเดิมแล้วค่อยรับ
update ต่อจาก WS; event ไม่ durable (หายช่วง disconnect) จึง **resync ด้วย refetch REST ตอน reconnect**.
Fan-out เป็น `events.Hub` (non-blocking, drop เมื่อ buffer เต็ม เหมือน console hub).

- **DB (`jobs` table) เป็น source of truth ของสถานะงานเสมอ** — NATS เป็นแค่ transport
- คำสั่งที่เปลี่ยน lifecycle ของ server ต้องเป็น job ผ่าน NATS เสมอ (ทนต่อ restart/redeliver, งานเขียนแบบ idempotent)
- ของ realtime (console, heartbeat, status) วิ่ง gRPC stream — หายแล้วหายเลย ไม่ต้อง durable
- Agent รู้ identity ตัวเองจาก `Welcome{node_id}` ที่ control plane ตอบหลัง auth เท่านั้น
  (config ฝั่ง agent มีแค่ token — **ห้ามเชื่อ node_id ที่ agent อ้างเอง**)

## Docker networks (แยก 3 วง)

| network | ใครอยู่ | หมายเหตุ |
|---|---|---|
| `edge` | caddy, web, control-plane, node-agent | ทางออก internet ของ service ที่ต้อง egress |
| `core` (internal) | control-plane, node-agent, postgres, redis, nats | ไม่มีทางออก internet, ไม่ publish port |
| `mcpanel-servers` | MC containers ทุกตัว + velocity | attachable, แยกขาดจาก core — MC container มองไม่เห็น DB/NATS |

- Velocity คุยกับ backend servers ผ่าน DNS alias `mc-{server_id}` ในวง `mcpanel-servers`
- Server ที่ไม่ตั้ง host_port จะเข้าถึงได้จาก velocity เท่านั้น

## Design decisions สำคัญ

- **Storage**: bind mount เสมอ (ไม่ใช้ named volume) — 1 instance = 1 directory `{MC_DATA_DIR}/{server_id}`
  - `MC_DATA_DIR` ต้องเป็น absolute path ที่ **เหมือนกันทั้งใน agent container และบน host**
    (agent เขียนไฟล์เองผ่าน mount และสั่ง bind mount ให้ sibling container ที่ docker daemon มองจาก host)
- **1 container ต่อ 1 instance** — ชื่อ `mc-{server_id}`, label `mc.managed_by=mc-panel-agent`
- **MC container ทุกตัว**: `cap-drop=ALL`, `no-new-privileges`, user 1000:1000, memory limit (+25% เผื่อ JVM overhead),
  pids limit, ไม่มี restart policy (agent เป็นคนคุม lifecycle), stdin เปิดไว้สำหรับ console
- **Runtime image ของเราเอง** (`mcpanel/mc-runtime:8|17|21`) — jar โหลดจาก official source เท่านั้น
  (Mojang/PaperMC/FabricMC/Forge maven) พร้อม verify checksum เมื่อ upstream ให้มา
- **Runtime image caching/reuse**: agent ensure image ก่อน start เสมอ — ถ้ามีในเครื่องแล้วใช้เลย
  (reuse ไม่ pull ซ้ำ), ถ้าไม่มีให้ pull `eclipse-temurin:{ver}-jre` (official) แล้ว tag เป็น
  `mcpanel/mc-runtime:{ver}` เก็บ cache ไว้ share กับทุก instance ที่ใช้ java version เดียวกันในอนาคต
  (`make runtime-images` ยัง build ตัว hardened เต็มได้ ถ้ามีอยู่ agent จะ reuse ตัวนั้น ไม่ทับ)
- **Container cleanup**: container ที่ start ล้มกลางทาง หรือ crash (die exit≠0 ที่ไม่ได้สั่ง stop) —
  agent ลบทิ้งทันที + push console line แจ้ง user (ไม่ทิ้ง dead container ค้าง; directory ข้อมูลคงไว้เสมอ)
- **Java mapping**: calendar version (26.x+)/velocity/fallback→25, MC 1.20.5–1.21.x→21, 1.17–1.20.4→17,
  ≤1.16.5→8 (logic ต้องตรงกัน 2 ที่: control-plane `jobs/image.go` เลือก image ของ job, agent
  `provision/forge.go` เลือก image ตอนรัน forge installer). Java backward-compatible → เวอร์ชันที่ parse
  ไม่ได้/รุ่นใหม่ default เป็น Java ใหม่สุดเสมอ (jar เก่ารันบน JVM ใหม่ได้; เฉพาะ MC เก่ามาก ≤1.16 ที่พังบน Java ใหม่จึงตรึงไว้ 8)
- **File manager**: กัน path traversal ด้วย `SafeJoin` — resolve symlink จาก ancestor ที่ลึกที่สุดที่มีจริง
  ใช้ทุกครั้งก่อนแตะ filesystem จาก path ภายนอก รวมถึงก่อน `RemoveAll` ตอนลบ server
- **RBAC 2 ชั้น**:
  - ต่อ server (`server_permissions`: owner/operator/viewer + granular flags) — ทุก endpoint ที่กระทบ
    server ต้องเช็คก่อนเสมอ
  - ระดับ panel (`users.capabilities` TEXT[]) — admin ตั้งต่อ user ให้เข้าถึงหน้า/เมนู + CRUD:
    key รูป `{feature}.{action}` ครอบทุกฟีเจอร์ (`users.*`, `nodes.*`, `servers.*`, `console.*`,
    `files.*`, `players.*`, `settings.*`, `access.*` — catalog เต็มใน docs/api.md)
  - endpoint ระดับ server ต้องผ่าน **ทั้งสองชั้น** (capability AND สิทธิ์ต่อ server นั้น)
  - is_admin = superuser ข้ามได้ทุกด่านทั้งสองชั้น
- **EULA**: user ต้องติ๊กยอมรับเองตอนสร้าง ระบบห้าม default เป็น true

## Security

- Secret ทุกตัวอยู่ใน `.env` (generate ด้วย `make env`, chmod 600) — ห้าม hardcode ใน compose/โค้ด
- Postgres/Redis/NATS อยู่ใน network internal, dev compose bind 127.0.0.1 เท่านั้น
- NATS บังคับ auth + แยก user: control-plane (จัดการ stream ได้) กับ agent (pull job + ส่งผลเท่านั้น
  — สร้าง consumer เองไม่ได้) ; ต่อ node หลายเครื่องจริงต้องยกระดับเป็น NKey/JWT ต่อ node
- Node token: opaque random, DB เก็บ SHA-256 — เทียบด้วย hash lookup
- Session: JWT ใน cookie HttpOnly SameSite=Lax + `token_version` ใน users (เปลี่ยน/reset password แล้ว
  token เก่าตายทันที) ; login มี rate limit ผ่าน redis ; WS ตรวจ Origin ทุก handshake
- CSRF: อาศัย SameSite=Lax + GET ห้ามมี side effect
- Client IP: control-plane อ่านจาก `X-Forwarded-For` โดยเชื่อ `TRUSTED_PROXY_COUNT` hop นับจากขวาสุด
  (default 1 = หลัง Caddy) — **ห้ามเชื่อค่าซ้ายสุดของ XFF** เพราะ client ปลอมได้ (ใช้ IP นี้กับ rate limit + audit)
  (implement ที่ control-plane — ตรงนี้บันทึกไว้เป็น design/coordination)
- **จุดเสี่ยงที่สุดของระบบ = docker.sock ใน node-agent** — agent ต้องเป็น service เดียวที่แตะ,
  โค้ดฝั่ง agent ต้อง validate ทุก path/ชื่อ container ที่มาจาก job ก่อนใช้
  (backlog: docker-socket-proxy หรือ rootless docker)
- ทุก action สำคัญลง `audit_logs` (รวม console command)

## Bootstrap

Full stack: `make env` → `make runtime-images` → `make up`
→ control-plane รัน migration อัตโนมัติ, สร้าง admin คนแรกด้วย **password สุ่มที่พิมพ์ลง log ครั้งเดียว**
(`make admin-password`), สร้าง node "local" จาก `NODE_TOKEN` ใน .env
→ login ครั้งแรกถูกบังคับตั้ง password ใหม่ก่อนใช้งาน (must_change_password)
→ user ใหม่ทุกคนก็ flow เดียวกัน: admin สร้าง → ได้ password สุ่มแสดงครั้งเดียว → คนนั้น login แล้วตั้งใหม่

เพิ่ม node เครื่องอื่น: สร้าง node ผ่าน `/api/nodes` ได้ token → รัน agent container บนเครื่องนั้น
พร้อม `AGENT_TOKEN` (ต้องเปิดทาง gRPC + NATS ให้ถึง control plane — TLS ระหว่างเครื่องเป็น backlog)

## Tech stack
Control plane: Go (chi, pgx, gorilla/websocket, nats.go, go-redis, goose, golang-jwt)
Node agent: Go (docker SDK, nats.go, gopsutil)
Frontend: Next.js 15 + shadcn/ui + Tailwind v4 + @xterm/xterm + react-query
DB: PostgreSQL 16 / Cache+rate limit: Redis 7 / Broker: NATS JetStream 2.10
Proto: protobuf + buf (gen Go+TS จากไฟล์เดียว, generated code commit เข้า repo)
Edge: Caddy (same-origin ให้ web+api, TLS เมื่อมี domain)

## กฎเวลาช่วยแก้โค้ดโปรเจกต์นี้ (สำหรับ Claude instance อื่น)

- อย่าเสนอให้ fork Pterodactyl/Crafty Controller หรือใช้ image panel สำเร็จรูป — ตัดสินใจแล้วว่าเขียนเอง 100%
- อย่าเสนอ Windows native / native process runner — scope ปัจจุบันคือ Docker ทั้งหมด
- อย่าเปลี่ยน storage จาก bind mount เป็น named volume
- อย่าเสนอ MongoDB/NoSQL มาเก็บ log — PostgreSQL + Redis (+ Loki ทีหลังถ้าจำเป็น)
- Job/command ต้องผ่าน NATS เสมอ ห้ามยัด lifecycle command เข้า gRPC stream และห้ามให้ agent เปิด port รับเข้า
- ทุก endpoint ที่กระทบ server ต้องเช็ค `server_permissions` ก่อนเสมอ
- แก้ interface ต้องแก้ contract ก่อน: docs/api.md (REST/WS) หรือ packages/proto (gRPC/NATS) แล้วค่อย implement
