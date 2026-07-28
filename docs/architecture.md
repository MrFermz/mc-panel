# สถาปัตยกรรม game-manager

## Scope
- Linux-first, **full Docker**: ทั้งระบบหลักและทุก game instance รันบน Docker ทั้งหมด
- Native process runner อยู่นอก scope ปัจจุบัน (interface `Runner` ยังเผื่อไว้ แต่ implement เฉพาะ Docker)
- รองรับ Vanilla, Paper (plugin), Fabric/Forge (modded), Velocity (proxy) ผ่าน abstraction เดียวกัน
  — ทั้งหมดเป็น **variant** ของ game definition `minecraft` ซึ่งเป็นเกมเดียวที่ลงทะเบียนไว้ตอนนี้
  (ดูหัวข้อ Game definition)
- Auth เป็น local อย่างเดียว (username+password — ระบบไม่เก็บ email เลย) — OIDC เป็น backlog

## 3 ชั้นหลัก
1. **Control plane** — API server (Go), stateless, ถือ DB/Redis/NATS
2. **Node agent** — Go binary ใน container, connect ออกไปหา control plane เอง (ไม่เปิด port รับเข้า)
3. **Node layer** — docker daemon ของเครื่องที่รัน container ของแต่ละ instance

## เส้นทางข้อมูล (transport แต่ละแบบมีหน้าที่ตายตัว — ห้ามสลับ)

```
Browser ── WebSocket /ws/servers/{id}/console (JSON) ──> Control plane   console I/O ต่อ server
Browser ── WebSocket /ws/events (JSON) <── Control plane                 push server/node/stats/jobs realtime
Browser ── REST /api (JSON) ──> Control plane          CRUD + โหลด state เริ่มต้น
Agent   ── gRPC stream (protobuf) ──> Control plane    heartbeat, console I/O, server status/stats (realtime)
Control plane ── NATS JetStream (protobuf) ──> Agent   jobs: create/start/stop/kill/delete (durable)
Agent   ── NATS gamemanager.results ──> Control plane      ผลลัพธ์ของ job
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
| `game-manager-servers` | container ของ instance ทุกตัว (รวม proxy variant) | attachable, แยกขาดจาก core — มองไม่เห็น DB/NATS |

- Velocity คุยกับ backend servers ผ่าน DNS alias `game-manager-{server_id}` ในวง `game-manager-servers`
- Server ที่ไม่ตั้ง host_port จะเข้าถึงได้จาก velocity เท่านั้น

## Game definition (abstraction ของ "เกม")

server ทุกตัวผูกกับ **เกม** หนึ่งเกม (`servers.game`) และคอลัมน์ `variant` คือ **ชนิดของ server**
ภายในเกมนั้น ความรู้เฉพาะเกมทั้งหมดถูกรวบไว้ใน `Definition` ตัวเดียวต่อเกม ต่อ app:

| ที่อยู่ | ถืออะไร |
|---|---|
| `apps/control-plane/internal/games` | Registry + `Definition`: variant + license, กติกาเวอร์ชัน, runtime image, catalog/ไวยากรณ์ของไฟล์ config, กติกาผู้เล่น (identity service, allowlist, ไฟล์ state, action, playtime, avatar), port เริ่มต้น, memory ขั้นต่ำ |
| `apps/control-plane/internal/games/minecraft` | ค่าจริงของ Minecraft: variant + EULA, รายการเวอร์ชัน, Java image mapping, `server.properties`, `whitelist.json`, `op/kick/ban`, **Mojang identity (`identity.go`) และการ crop หน้าจาก skin (`avatar.go`)** |
| `apps/node-agent/internal/games` | Registry + `Definition` ฝั่งรันจริง: ที่มาของ artifact, launch script, env, คำสั่ง stop, seed config, port ใน container, การอ่านสถานะในเกมจาก console + `.gamemanager/meta.json` (instance บน disk บอกเองว่าเป็นเกมอะไร) |
| `apps/node-agent/internal/games/minecraft` | ค่าจริงของ Minecraft ฝั่ง agent (ที่มาของ jar, launch script/heap, forge installer, parser ของ console) |
| `apps/*/internal/games/zomboid` | ค่าจริงของ Project Zomboid: ติดตั้งผ่าน **SteamCMD** (app 380870, login anonymous), `game_version` = Steam branch, ไฟล์ ini ใน cache dir ของเกม, **Dockerfile ของ runtime image ที่มี SteamCMD** (agent embed ไว้แล้ว build เอง) — image pin `linux/amd64` และ **ต้องรันบน node x86_64 เท่านั้น** (SteamCMD เป็น binary 32-bit x86, agent cross-build ไม่ได้เพราะ Engine API มีแค่ classic builder) |
| `apps/web/lib/games` | Registry + `GameProfile` ฝั่ง web: เดา variant/version จาก archive, กติกาชื่อผู้เล่นฝั่ง client, การลงสีบรรทัด console, ชื่อเกม/license/metric ที่โผล่ในข้อความ, **ลำดับ step ของ wizard สร้าง server (`wizardSteps`) + ปก/คำอธิบายบนการ์ดหน้าเลือกเกม** |

- สอง app แยก module กันจึง **ไม่ import ข้ามกัน** — สิ่งที่ต้องตรงกันคือ **id ของเกมและ variant**
  ซึ่งเดินทางผ่าน job payload (`CreateServer.game`) และ `.gamemanager/meta.json`
- ชั้นอื่น ๆ (httpapi, jobs, store, provision, runner, gamestate, component ฝั่ง web) **ห้ามมี switch
  ตามชื่อเกม/variant** — ทุกอย่างถามผ่าน definition/profile. เพิ่มเกมใหม่ = เพิ่ม package `games/<เกม>`
  ทั้งสามที่ แล้วลงทะเบียนใน registry ที่ `cmd/server` / `cmd/agent` / `lib/games/index.ts`
- ของกลางที่เคยผูกกับ Minecraft ถูกแยกออกเป็น package ที่ไม่รู้จักเกมแล้ว:
  `internal/avatarcache` (cache รูปผู้เล่นลง Postgres + เสิร์ฟรูปเก่าตอน upstream ล่ม — รับ
  "วิธีได้รูปมา" จาก definition), `internal/gamestate` ฝั่ง agent (เดิมชื่อ `mcstate`)
- schema ก็ไม่ถือความรู้ของเกม: คอลัมน์ `variant` **ไม่มี CHECK รายชื่อ** และ `memory_mb` เช็คแค่ `> 0`
  (floor จริงมาจาก `Definition.MinMemoryMB`) — เพิ่มเกม/variant ใหม่จึงไม่ต้องแตะ schema
- field/route ที่เคยใช้ศัพท์ Minecraft ถูกเปลี่ยนเป็นคำกลางหมดแล้ว: `mc_version`→`game_version`,
  `server_type`→`variant`, `whitelist*`→`allowlist*`, `tps`→`tick_rate`, `accept_eula`→`accept_license`,
  `/properties`→`/config`, `/players/{uuid}/face`→`/avatar`, `mojang_unavailable`→`identity_unavailable`
  (ศัพท์ของเกมโผล่ได้เฉพาะ *ค่า* ที่ definition ส่งมา เช่น label `"Minecraft EULA"`)

## Design decisions สำคัญ

- **Storage**: bind mount เสมอ (ไม่ใช้ named volume) — 1 instance = 1 directory
  `{GM_DATA_DIR}/{game}/{server_id}` (ชั้นเกมทำให้ข้อมูลของแต่ละเกมไม่ปนกัน)
  - `GM_DATA_DIR` ต้องเป็น absolute path ที่ **เหมือนกันทั้งใน agent container และบน host**
    (agent เขียนไฟล์เองผ่าน mount และสั่ง bind mount ให้ sibling container ที่ docker daemon มองจาก host)
  - layer อื่นรู้แค่ `server_id` (job start/stop, file manager, console) — `filemanager.Layout`
    เป็นคนแปลง id เป็น path เดียวในระบบ: `Dir(game, id)` ตอน provision (รู้เกมแน่นอน) และ
    `Find(id)` ที่สแกนชั้นเกมให้ตอนอื่น ๆ. instance ที่สร้างไว้ก่อนมีชั้นเกมถูกย้ายให้อัตโนมัติ
    ตอน agent boot (`games.MigrateLegacyLayout` — อ่าน `.gamemanager/meta.json` เพื่อรู้ว่าเป็นเกมอะไร)
- **1 container ต่อ 1 instance** — ชื่อ `game-manager-{server_id}`, label `gamemanager.managed_by=game-manager-agent`
- **container ของ instance ทุกตัว**: `cap-drop=ALL`, `no-new-privileges`, user 1000:1000, memory limit,
  pids limit, ไม่มี restart policy (agent เป็นคนคุม lifecycle), stdin เปิดไว้สำหรับ console
- **Runtime image ของเราเอง** (`game-manager/runtime-java:8|17|21|25`, `game-manager/runtime-steam:1`)
  — ไม่รู้จักเกมใดเป็นพิเศษ, artifact โหลดจาก official source ของเกมเท่านั้น (ที่มาอยู่ใน game
  definition: Mojang/PaperMC/FabricMC/Forge maven สำหรับ minecraft, Steam CDN ผ่าน SteamCMD
  สำหรับ zomboid) พร้อม verify checksum เมื่อ upstream ให้มา
- **Runtime image caching/reuse**: agent ensure image ก่อน start เสมอ — ถ้ามีในเครื่องแล้วใช้เลย
  (reuse ไม่ pull ซ้ำ) ถ้าไม่มีให้เตรียมตามที่ definition บอกผ่าน `games.ImageSource` ซึ่งมีสองทาง:
  - **pull แล้ว tag ซ้ำ** — เกมที่มี base image official ให้ใช้ (minecraft:
    `eclipse-temurin:{ver}-jre` → tag เป็น `game-manager/runtime-java:{ver}`)
  - **build เองจาก Dockerfile ที่ definition ถือไว้** — เกมที่ไม่มี base สำเร็จรูป (zomboid ต้องมี
    SteamCMD ในตัว และเราไม่ใช้ image ของ third-party) — agent embed Dockerfile ไว้ในตัว binary
    แล้ว `docker build` เป็น `game-manager/runtime-steam:1` ครั้งแรกที่ต้องใช้
  ทั้งสองทางตั้งชื่อ tag เดียวกันเสมอ จึง share cache ข้าม instance บน node เดียวกัน
  (`make runtime-images` build ล่วงหน้าได้ทั้งสองแบบ ถ้ามีอยู่แล้ว agent reuse ตัวนั้น ไม่ทับ)
  - namespace ของชื่อ image ตั้งได้ที่ agent ผ่าน `GM_RUNTIME_IMAGE_NAMESPACE` (default
    `game-manager`) — ตั้งแล้วต้องตรงกับที่ control-plane เลือกไว้ใน definition ด้วย
- **Port ของ instance มาจาก definition**: เกมบอกเองว่าฟัง port อะไร protocol ไหน และกี่ตัว
  (minecraft = `25565/tcp`; zomboid = `16261/udp` + `16262/udp`) — host port ของ port รอง
  คือ `host_port + n` ซึ่งแปลว่า instance ของเกมนั้นจอง host port ติดกันหลายตัว
  (`Definition.HostPortSpan` ฝั่ง control-plane เป็นคนบอก `/api/meta/next-port` ให้เว้นช่วงพอ)
- **Container cleanup**: container ที่ start ล้มกลางทาง หรือ crash (die exit≠0 ที่ไม่ได้สั่ง stop) —
  agent ลบทิ้งทันที + push console line แจ้ง user (ไม่ทิ้ง dead container ค้าง; directory ข้อมูลคงไว้เสมอ)
- **Java mapping (ของ minecraft)**: calendar version (26.x+)/velocity/fallback→25, 1.20.5–1.21.x→21, 1.17–1.20.4→17,
  ≤1.16.5→8 (logic อยู่ใน game definition ของ minecraft เท่านั้น และต้องตรงกัน 2 ที่:
  control-plane `internal/games/minecraft/runtime.go` เลือก image ของ job, agent
  `internal/games/minecraft/runtime.go` เลือก image ตอนรัน forge installer). Java backward-compatible → เวอร์ชันที่ parse
  ไม่ได้/รุ่นใหม่ default เป็น Java ใหม่สุดเสมอ (jar เก่ารันบน JVM ใหม่ได้; เฉพาะรุ่นเก่ามาก ≤1.16 ที่พังบน Java ใหม่จึงตรึงไว้ 8)
- **File manager**: กัน path traversal ด้วย `SafeJoin` — resolve symlink จาก ancestor ที่ลึกที่สุดที่มีจริง
  ใช้ทุกครั้งก่อนแตะ filesystem จาก path ภายนอก รวมถึงก่อน `RemoveAll` ตอนลบ server
- **RBAC 2 ชั้น**:
  - ต่อ server (`server_permissions`: owner/operator/viewer + granular flags) — ทุก endpoint ที่กระทบ
    server ต้องเช็คก่อนเสมอ
  - ระดับ panel (`users.capabilities` TEXT[]) — admin ตั้งต่อ user ให้เข้าถึงหน้า/เมนู + CRUD:
    key รูป `{feature}.{action}` ครอบทุกฟีเจอร์ (`users.*`, `nodes.*`, `servers.*`, `console.*`,
    `files.*`, `players.*`, `settings.*`, `access.*` — catalog เต็มใน docs/api.md)
  - **สิทธิ์ต่อเกม** (`games.{game_id}`) เป็นกลุ่มที่ catalog สร้างจาก registry ตอน runtime:
    การสร้าง server ต้องผ่าน `servers.create` **และ** capability ของเกมนั้น
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
→ control-plane รัน `schema/schema.sql` (idempotent, ยังไม่มีระบบ migration) อัตโนมัติ,
สร้าง admin คนแรกด้วย **password สุ่มที่พิมพ์ลง log ครั้งเดียว**
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
