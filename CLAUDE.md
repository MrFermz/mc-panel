# CLAUDE.md — game-manager

คู่มือสำหรับ AI (และคน) ที่มาทำโปรเจกต์นี้ต่อ ไม่ว่าจะย้ายเครื่องหรือเริ่ม session ใหม่
อ่านคู่กับ [`docs/architecture.md`](docs/architecture.md) (การตัดสินใจเชิงระบบ) และ
[`docs/api.md`](docs/api.md) (REST/WS contract)

## โปรเจกต์นี้คืออะไร

ระบบจัดการ **game server** หลาย instance แบบ microservices — ตัวระบบหลัก **ไม่ผูกกับเกมใดเกมหนึ่ง**
ความรู้เฉพาะเกมอยู่ใน game definition แยกเป็น package (ตอนนี้ลงทะเบียนไว้ 2 เกม: `minecraft`
variant vanilla/paper/fabric/forge/velocity และ `zomboid` variant vanilla ที่ติดตั้งผ่าน SteamCMD)
เขียนเองทั้งหมด ทุกอย่างรันบน Docker — web (Next.js) + control-plane (Go) + node-agent (Go)
คุยกันผ่าน REST/WebSocket (browser), gRPC stream (realtime), NATS JetStream (jobs), protobuf

## กฎเหล็ก (ห้ามละเมิดไม่ว่าจะดูสมเหตุสมผลแค่ไหน)

0. **ความรู้เฉพาะเกมอยู่ใน game definition ที่เดียว** — server ทุกตัวผูกกับเกม (`servers.game`) และคอลัมน์ `variant` คือ **ชนิดของ server** ภายในเกมนั้น. ทุกอย่างที่
   "เป็นของเกม" (รายการ variant/license, กติกาเวอร์ชัน, runtime image, ชื่อ+catalog+ไวยากรณ์ของไฟล์
   config, กติกาผู้เล่น/allowlist/คำสั่ง moderation/playtime/avatar, ที่มาของ artifact, launch script,
   คำสั่ง stop, port ใน container, การอ่าน console) อยู่ใน `internal/games` ของแต่ละ app
   และ `lib/games` ฝั่ง web
   — **ห้ามเขียน `switch` ตามชื่อเกม/variant หรือ hardcode ชื่อไฟล์/คำสั่งของเกมในชั้นอื่น**
   (httpapi, jobs, store, provision, runner, gamestate, component ฝั่ง web) เด็ดขาด
   ดูเช็คลิสต์ในหัวข้อ "เพิ่มเกมใหม่ / แก้ของเกมเดิม" ด้านล่าง
   — **schema ก็ห้ามถือความรู้ของเกม**: คอลัมน์ `variant` ไม่มี CHECK รายชื่อ และ `memory_mb`
   เช็คแค่ `> 0` (floor จริงมาจาก `Definition.MinMemoryMB`) — อย่าเพิ่ม CHECK กลับเข้าไป
1. **แก้ contract ก่อนแก้โค้ด** — interface ระหว่าง service มี source of truth 3 ที่:
   - REST/WS: `docs/api.md`
   - gRPC/NATS: `packages/proto/gamemanager/**/*.proto` (แก้แล้วรัน `make proto-gen` และ **commit generated code**)
   - DB: `apps/control-plane/schema/schema.sql` — **ยังไม่มีระบบ migration** (ถอด goose ออกแล้ว
     จะกลับมาทำใหม่ทีหลัง). control-plane รันไฟล์นี้ทั้งก้อนตอน boot ทุกครั้ง จึงต้อง
     **idempotent เสมอ** (`IF NOT EXISTS` ทุกคำสั่ง) แต่ก็ **ไม่ migrate ข้อมูลเดิมให้**:
     แก้คอลัมน์/constraint ของตารางที่ถูกสร้างไปแล้วจะไม่มีผลกับ DB เก่า — ต้อง `make reset`
     (dev) หรือลบ volume ทิ้งเอง
2. **Lifecycle command (create/start/stop/kill/delete) ต้องเป็น job ผ่าน NATS เสมอ** โดยมี `jobs` table
   เป็น source of truth — ห้ามยัดคำสั่งพวกนี้เข้า gRPC stream / ห้ามให้ agent เปิด port รับ connection เข้า
3. **ทุก endpoint ต้องผูก permission ก่อน merge** (is_admin ข้ามได้ทุกด่าน):
   - endpoint ที่กระทบ server ต้องเช็ค **ทั้งสองชั้น** — global capability (`requireCap` ใน
     ตาราง route ของ `internal/httpapi/api.go`) **และ** `server_permissions` ต่อ server นั้น
   - **เพิ่ม feature ใหม่ = เพิ่ม/ลบ capability ด้วยเสมอ** ตามเช็คลิสต์ในหัวข้อ
     "เพิ่ม feature ใหม่ → ต้องแตะ permission" ด้านล่าง — endpoint ที่ไม่ผูก capability = ช่องโหว่ทันที
4. **ห้าม hardcode secret** ใน compose/โค้ด/Dockerfile — ทุก secret มาจาก `.env` (สร้างด้วย `make env`)
   และห้ามให้ `.env` เข้า docker build context (มี `.dockerignore` กันแล้ว อย่าไปแก้ทิ้ง)
5. **Path จากภายนอกต้องผ่าน `SafeJoin` ก่อนแตะ filesystem เสมอ** โดยเฉพาะก่อน `RemoveAll`
6. **อย่า fork/พึ่ง panel หรือ runtime image สำเร็จรูป** (Pterodactyl, itzg ฯลฯ) — เขียนเอง 100%,
   artifact โหลดจาก official source ของเกมเท่านั้น (ที่มาอยู่ใน game definition — minecraft:
   Mojang/PaperMC/FabricMC/Forge maven) + verify checksum เมื่อมี
7. **อย่าเปลี่ยน** bind mount → named volume, PostgreSQL → NoSQL, และอย่าเพิ่ม Windows/native runner
   (scope คือ full Docker บน Linux)
8. **`accept_license` ห้าม default เป็น true** — user ต้องติ๊กยอมรับ license ของเกมเอง
   (ชื่อที่แสดงมาจาก `Definition.LicenseName` เช่น "Minecraft EULA")
9. Frontend **ห้ามเก็บ token เอง** (ไม่มี localStorage/sessionStorage สำหรับ auth) — cookie HttpOnly
   จาก control-plane เท่านั้น และห้ามใช้ next-auth

## โครงสร้าง repo + แผน submodules

```
apps/control-plane   Go — module github.com/game-manager/control-plane
apps/node-agent      Go — module github.com/game-manager/node-agent
apps/web             Next.js (self-contained ไม่ import ข้าม directory)
packages/proto       .proto + generated Go — module github.com/game-manager/proto
packages/shared-types  TS generate จาก proto (ยังไม่ถูกใช้โดย web — เผื่ออนาคต)
infra                compose ทั้งสองชุด, caddy, nats config, runtime-java image (ไม่รู้จักเกม)
                     (runtime image ของเกมที่ต้องมีเครื่องมือเฉพาะ เช่น SteamCMD อยู่ใน package
                      ของเกมนั้นแทน เพราะ agent ต้อง embed ไปด้วย)
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
make runtime-images   # build runtime image ล่วงหน้า (ออปชัน — ข้ามได้ agent เตรียมเองตอนใช้ครั้งแรก)
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

- **ชื่อที่แสดงให้ user เห็นคือ "Game manager"** มาจาก `APP_NAME` (`apps/web/lib/brand.ts`) ที่เดียว
  (header/sidebar/tab title/หน้า login + prefix ของ system line ใน console) — ไม่ใช่ข้อความที่แปลตามภาษา
  จึงไม่อยู่ใน i18n dictionary. `game-manager` ที่เหลือเป็น **identifier** (repo, go module,
  ชื่อ container/label/image/network) — ห้ามเปลี่ยนตามชื่อที่แสดง
- **Comment ภาษาไทย** เขียนเฉพาะจุดที่อธิบาย "ทำไม" หรือ constraint ที่โค้ดบอกเองไม่ได้ —
  ห้าม comment เล่าว่าบรรทัดถัดไปทำอะไร / **log message ภาษาอังกฤษ** (เพื่อ grep + log tooling)
- Identifier/ชื่อไฟล์ภาษาอังกฤษทั้งหมด
- HTTP error ตอบ `{"code": "snake_case_code", "message": "..."}` — code ที่ web ผูก logic ไว้:
  `unauthorized`, `password_change_required`, `forbidden`, `rate_limited` (ดูครบใน docs/api.md)
- Go: stdlib + chi/pgx/gorilla — SQL เขียนตรง ๆ ใน `internal/store` ไม่ใช้ ORM;
  ห้ามเพิ่ม dependency ใหญ่โดยไม่มีเหตุผลใน commit/PR message
- Web: Next.js App Router, shadcn/ui (component อยู่ `components/ui` — generate/vendor ตามสไตล์ shadcn),
  react-query สำหรับ server state, zod schema ใน `lib/types.ts` ต้อง sync กับ docs/api.md
- **loading มี 2 ชั้น ทำงานคู่กันอัตโนมัติ** — ชั้นปุ่ม + ชั้นคลุมทั้งจอ:
  1. **ปุ่ม**: `Button` (`components/ui/button.tsx`) มี prop `loading` ที่เติม spinner นำหน้า +
     `disabled` + `aria-busy` ให้เอง — เขียน `loading={m.isPending}` แล้วเหลือ `disabled` ไว้เฉพาะ
     เงื่อนไข validate (**อย่าใส่ `isPending` ซ้ำใน `disabled`**). `ConfirmDialog` ส่ง `pending`
     ลงไปให้แล้ว. ปุ่มที่มีไอคอนนำอยู่แล้วต้องซ่อนไอคอนตอน loading เอง (`{!busy && <Icon />}`)
  2. **ทั้งจอ**: `GlobalLoading` (`components/global-loading.tsx`) mount ที่ `app/providers.tsx`
     เกาะกับ `useIsMutating()` ของ react-query — **ทุก `useMutation` ได้ overlay อัตโนมัติ
     ไม่ต้องต่อสายที่ call site**. โผล่เมื่อเกิน `SHOW_AFTER_MS` (350ms) เท่านั้น งานที่จบเร็วกว่านั้น
     เห็นแค่ spinner ที่ปุ่ม — **อย่าเอา delay ออก** ไม่งั้นจอกะพริบเทาทุกครั้งที่กดปุ่ม
  - **ทุก action ที่ยิง API ต้องเป็น `useMutation`** (ไม่ใช่ `useState` + async เอง) ไม่งั้นหลุด
    จาก `useIsMutating` แล้วไม่มี overlay — `useState(false)` ที่เหลือในโค้ดเป็นแฟล็ก dialog ล้วน
  - mutation ที่มี overlay เฉพาะทางของตัวเอง (เช่น create ของ wizard ที่บอก phase) ต้องตั้ง
    `mutationKey: [LOCAL_OVERLAY_KEY, ...]` เพื่อกันตัวเองออกจาก GlobalLoading ไม่ให้ซ้อน 2 ชั้น
  - ปุ่ม start/stop/restart/kill **จงใจไม่มี spinner** — HTTP เป็นแค่ dispatch job
    ตัวบอกความคืบหน้าจริงคือ status badge ที่อัปเดตผ่าน WS
- `PageLoader` (`components/page-loader.tsx`) = สถานะ "หน้ายังไม่พร้อม" (ไม่ใช่ระหว่าง action)
  ใช้ที่ auth guard ของทั้งสอง layout + Suspense fallback
- **modal ทุกตัวต้อง sticky header/footer** — `DialogContent` (`components/ui/dialog.tsx`) เป็น
  flex column สูงไม่เกิน 85svh, `DialogHeader`/`DialogFooter` เป็น `shrink-0` มีเส้นคั่น+พื้นหลัง
  ส่วน **`DialogBody` คือ scroll container เดียว** ที่เหลือ. กติกาเวลาเขียน dialog ใหม่:
  1. เนื้อหาที่ไม่ใช่ header/footer **ต้องห่อด้วย `DialogBody` เสมอ** — `DialogContent` เป็น `p-0`
     (padding อยู่ที่แต่ละ section) ของที่หลุดออกมาจะไม่มี padding และเลื่อนไม่ได้
  2. dialog ที่มี `<form>` ครอบ body+footer ให้ form เป็น `className="contents"` —
     ไม่งั้น form กลายเป็น flex child ชั้นเดียว footer จะเลื่อนตามเนื้อหาไปด้วย
  3. `DialogDescription` ที่ยาวเกินหนึ่งบรรทัดให้ไว้ต้น `DialogBody` ไม่ใช่ใน header
     (header ตรึงอยู่ ถ้ายาวจะกินพื้นที่จนส่วนที่เลื่อนได้เหลือนิดเดียว)
- **หน้าใน nav > general ผูกกับ "active server"** (เลือกจากหน้า list ที่ `/` เก็บใน `dashboardServerId`)
  ไม่ผูก id ใน URL — `/console`, `/players`, `/files`, `/access`, `/logs`, `/settings` ใช้ `ServerPageShell`
  + `useActiveServer()` ร่วมกัน (จัดการ loading/error/ไม่มี server/ไม่มีสิทธิ์ ที่เดียว)
- **`/` = หน้า server list (landing ก่อนเข้า panel)** อยู่นอก route group `(panel)` จึงไม่มี sidebar/top bar
  (`app/page.tsx`) — **เห็นเฉพาะ server ที่ตัวเองมีชื่อใน access (`server_permissions` row)
  แม้เป็น is_admin/มี `servers.view_all` ก็ตาม** (`GET /api/servers` default `scope=mine`);
  การจัดการ server ทั้งระบบอยู่ที่ `/admin/servers` (`?scope=all` ต้องมี `servers.view_all`)
  — การ์ดต่อ server (สถานะ/ผู้เล่น/tick rate/uptime/port) กดแล้ว `setDashboardServerId(id)`
  + ไป `/dashboard`. หน้านี้ mount `EventsListener` เอง (อยู่นอก panel layout) การ์ดจึงอัปเดต realtime
  ตามกฎ "ห้าม poll REST". **ไม่มี server switcher ใน sidebar แล้ว** — สลับ server ทำที่ `/` ที่เดียว
  (sidebar เหลือลิงก์ `All servers` กลับไปหน้านั้น) และชื่อ/สถานะของ server ที่กำลังจัดการอยู่บน
  top bar หน้าชื่อหน้าเป็น trail `● {ชื่อ server} / {ชื่อหน้า}` — มาจาก `usePageServer()` ซึ่งหน้า
  ต้อง `useSetPageServer()` เอง (`ServerPageShell` ทำให้แล้ว) หน้าไหนไม่ผูก server ก็เหลือแค่ชื่อหน้า
- **ไม่มีหน้า detail ต่อ server แล้ว** — `/servers/[id]` ถูกลบทิ้ง (ทุกแท็บย้ายไปเป็นหน้าใน general หมด)
  ที่ไหนอยากพา user ไปดู server ตัวหนึ่ง ให้ `setDashboardServerId(id)` แล้วไป `/dashboard` แทนการ push path
  (ตัวอย่าง: ชื่อ server ในตาราง `/admin/servers`, ปุ่มจบ wizard). สร้าง server อยู่ที่ `/servers/new`
- **route group `(standalone)` = หน้าที่ไม่ผูก active server จึงไม่มี sidebar** — `/admin/*`,
  `/servers/new`, `/profile`, `/preferences` (`app/(standalone)/layout.tsx`: top bar + user menu
  + nav แนวนอนของ admin เมื่ออยู่ใต้ `/admin`, เนื้อหา `max-w-6xl`). sidebar เหลือเฉพาะ group
  `(panel)` (หน้าที่ทำงานกับ active server) — **อย่าเอาหน้าเหล่านี้กลับเข้า `(panel)`**.
  ทั้งสอง layout ใช้ `useAuthGuard()` (`lib/use-auth-guard.ts`) + `PageTitle`
  (`components/layout/page-title.tsx`) ร่วมกัน — เพิ่ม layout ใหม่ก็ต้องใช้ตัวเดียวกันนี้
- **ทางเข้า admin อยู่ใน user menu ที่เดียว** (`components/layout/user-menu.tsx` — ปุ่มเดียวชื่อ
  `Admin` เหนือ Profile/Preferences, พาไป `visibleFor(adminItems, user)[0]` = หน้าแรกที่มีสิทธิ์เห็น
  แล้วสลับต่อด้วย nav แนวนอนในนั้น; ไม่มีสิทธิ์สักหน้า = ไม่ขึ้นปุ่ม) เพราะ
  user menu โผล่ทุก layout (panel sidebar ล่างสุด, top bar mobile, standalone header, หน้า `/`)
  — **sidebar ไม่มี section Admin แล้ว** และ `adminItems` (`components/layout/sidebar-nav.tsx`)
  ยังเป็น source of truth เดียวของรายการ ใช้ร่วมกันทั้ง user menu + nav แนวนอนในหน้า admin
- **Realtime push**: server/node/stats/jobs update วิ่งผ่าน events WS `/ws/events` (browser เปิดเส้นเดียว,
  control-plane push จาก hook ใน `internal/agenthub` + `internal/jobs` → `internal/events.Hub`) —
  **ห้ามเพิ่ม `refetchInterval` poll REST สำหรับข้อมูลพวกนี้** (โหลด state เริ่มต้นด้วย REST ครั้งเดียว
  แล้วรับ update ต่อจาก WS, resync ด้วย refetch ตอน reconnect). เพิ่ม push event ใหม่ = emit ที่ agent
  gRPC hook point (agenthub) หรือ job result (jobs) เข้า `events.Hub` แล้ว document ใน docs/api.md
  - **job progress**: `job_update` (job_id/job_type/status/error/restart) emit ทั้งตอน dispatch
    (`internal/jobs.Dispatcher`) และตอนจบ/ถูก reap (`internal/jobs.ResultConsumer`) → web ขึ้น toast
    ผลจริงของ start/stop/restart/kill ให้ user (ปุ่มบอกได้แค่ "ส่งคำสั่งแล้ว"). จำเป็นเพราะ start/stop
    ที่ล้มบางเคสไม่มี `server_status` ตามมาเลย (planTransition ปล่อยให้ heartbeat reconcile) —
    ถ้าไม่มี event นี้ user จะไม่มีทางรู้ว่างานพังเพราะอะไร. `restart:true` = ขา stop ของ restart
    (สำเร็จแล้วยังไม่จบ ขา start ตามมาเป็น job ใหม่)
  - **server list change**: `server_added` (emit ตอน create ใน httpapi) / `server_removed`
    (emit ตอน delete job สำเร็จใน jobs) broadcast แบบ unfiltered (payload มีแค่ server_id) →
    web invalidate `["servers"]` refetch (dashboard เพิ่ม/เอา instance ออกเองแบบ realtime)
- Proto: package `gamemanager.<x>.v1` (ไม่มี hyphen — proto package เป็น identifier),
  directory ต้องตรง package (buf lint STANDARD บังคับ),
  breaking change ต้องขึ้น v2 ไม่แก้ v1
- NATS subjects: `gamemanager.jobs.{node_id}` (JobEnvelope), `gamemanager.results` (JobResult) —
  stream `JOBS` (WorkQueue) / `RESULTS`, consumer สร้างโดย control-plane เท่านั้น
  (NATS user ของ agent ไม่มีสิทธิ์สร้าง — ดู `infra/nats/nats-server.conf`)
- Docker: container ของ instance ชื่อ `game-manager-{server_id}`, label `gamemanager.managed_by=game-manager-agent`,
  ข้อมูลอยู่ `{GM_DATA_DIR}/{game}/{server_id}` bind mount เป็น `/data` (`games.ContainerDataDir`)
  — path ของ instance ประกอบที่ `filemanager.Layout` ที่เดียว (`Dir(game, id)` ตอน provision,
  `Find(id)` สำหรับ layer ที่รู้แค่ server id) **ห้าม `filepath.Join(dataDir, serverID)` เอง**;
  instance เก่าที่ยังอยู่ชั้นบนสุดถูกย้ายให้ตอน agent boot (`games.MigrateLegacyLayout`)
- เวลาแก้ Makefile: จำไว้ว่า buf/goose เรียกผ่าน `go run` (ไม่ assume ว่าติดตั้งไว้)

## Flow สำคัญที่ต้องเข้าใจก่อนแก้

**Initial user**: users ว่าง → control-plane boot สร้าง admin (`ADMIN_USERNAME` default `admin` เป็น
username ที่ใช้ login + password สุ่ม 20 ตัว พิมพ์ลง log ครั้งเดียว, `must_change_password=true`)
→ login แล้วทุก endpoint ตอบ `403 password_change_required` จน user เปลี่ยน password
(bump `token_version` → JWT เก่าตายหมด)
Admin สร้าง user ใหม่/reset password ก็ flow เดียวกัน — API ตอบ `initial_password` ครั้งเดียว

**ระบบไม่มี email เลย** — `username` เป็น login identifier เดียว (`users.username` NOT NULL +
unique บน `lower(username)` **ทั้งตาราง** (รวมแถวที่ถูก soft delete — ไม่ใช่ partial index:
ชื่อถูกจองไว้ตลอด กู้คืนแล้วชนชื่อไม่ได้แน่นอน
แลกกับที่ชื่อของบัญชีในถังขยะเอาไปสร้างใหม่ไม่ได้), match `^[a-z0-9_.-]{3,64}$`
(**ตัวพิมพ์เล็กล้วนเสมอ** — มี CHECK `users_username_lowercase`
`username = lower(username)` เป็นด่านสุดท้าย; ทุกทางเข้าที่รับ username จากภายนอกต้องผ่าน
`canonicalUsername()` ก่อน validate/เทียบ/บันทึก — create, check-username, login, grant
permission ด้วย username, `ADMIN_USERNAME` ตอน load config, flag `-username` ของ CLI.
ฝั่ง web **ช่องกรอกรับทุก case ไม่แปลงตัวอักษรใต้มือที่กำลังพิมพ์** (UX แปลก) — lower ตอน
submit/ตอนเช็คซ้ำแทน, ฟอร์มสร้าง user บอกไว้ด้วยว่าจะถูกบันทึกเป็นชื่ออะไร (`users.usernameFreeAs`).
⚠️ `server_players.username` เป็นชื่อผู้เล่น **Minecraft** คนละเรื่องกัน — ห้าม lower),
**และต้องไม่ใช่ชื่อที่ระบบสงวนไว้** (`internal/httpapi/reserved_usernames.go` — `admin`, `system`,
`support`, ชื่อ role, ชื่อ component ฯลฯ; เทียบบนรูป normalized = พิมพ์เล็ก+ตัด `._-` ทิ้ง
`a-d-m-i-n` จึงโดนบล็อกด้วย, แต่เป็น exact match ไม่ใช่ substring `nodeman` เลยผ่าน).
enforce ที่ **HTTP handler เท่านั้น** — seed ตอน boot กับ CLI `-reset-admin-password` เรียก store
ตรง ๆ จึงตั้งชื่อ `admin` ได้ตามเดิม (**อย่าย้าย check ลงไปที่ store จะพัง seed**).
ฟอร์มสร้าง user เช็คสดผ่าน `GET /api/users/check-username` (เกณฑ์ต้องตรงกับ handler เสมอ),
login เทียบแบบ case-insensitive). ห้ามเพิ่มคอลัมน์/field email กลับมาโดยไม่ปรึกษาก่อน —
ตอนนี้ไม่มีอะไรในระบบส่งเมล จึงเป็น PII ที่ต้องดูแลฟรี ๆ

**ลบ user = soft delete เสมอ** (`users.deleted_at`, capability
`users.restore`): `DELETE /api/users/{id}` (cap `users.delete`) mark `deleted_at`
+ `is_active=false` + bump `token_version` (session เก่าตายทันที) แต่ **ไม่แตะ `server_permissions`
เลย** — restore ต้องได้สิทธิ์ต่อ server กลับมาครบโดยไม่ต้อง assign ใหม่. กู้คืน =
`POST /api/users/{id}/restore` (cap `users.restore`) → `deleted_at=NULL` + `is_active=true`
(กู้คืนแล้วชนชื่อไม่ได้ — username ถูกจองไว้ตลอด; 409 `username_exists` เหลือไว้เป็น safety net).
ถังขยะอยู่ที่ `/admin/users` filter `status=deleted` (ที่เดียวที่ `ListUsers` โผล่แถวที่ถูกลบ)
⚠️ grant ที่ค้างไว้ต้องไม่รั่วออกมาทางไหน: `ListServerPermissions` + `CountServerOwners`
join `users` แล้ว filter `deleted_at IS NULL` — **query ใหม่ที่อ่าน `server_permissions` ต้องทำแบบเดียวกัน**

**assign server ให้ user จากฝั่ง user**: `/admin/users/{id}/servers` (แท็บคู่กับ
`/admin/users/{id}/permissions`, nav ร่วมที่ `components/user/user-detail-tabs.tsx`) —
`GET|POST /api/users/{id}/servers`, `DELETE /api/users/{id}/servers/{server_id}`
(`internal/httpapi/user_permissions_handlers.go`). เป็นข้อมูลชุดเดียวกับแท็บ Access ต่อ server
แค่มองกลับด้าน จึงต้องเช็คสิทธิ์เหมือนกันเป๊ะ: global cap (`access.view`/`access.manage`)
**และ owner ของ server ตัวนั้น** (`ownedServer()`) — ไม่งั้นใครมี `access.manage` จะ grant owner
ให้ตัวเองบน server ที่ไม่เกี่ยวข้องได้ + guard `last_owner` เหมือนฝั่ง server

**Profile ของตัวเอง** (`/profile`): user ทุกคนแก้ display name + รูป avatar ของตัวเองได้ และ
เปลี่ยนรหัสผ่านจากหน้านี้ (ฟอร์มเดียวกับ dialog — `components/user/change-password-form.tsx`;
`ChangePasswordDialog` เหลือไว้ใช้กับ forced flow `/change-password` เท่านั้น).
endpoint `PATCH /api/auth/me`, `PUT|DELETE /api/auth/me/avatar` **ไม่ผูก capability** —
ยึด user id จาก session จึงแตะได้แค่บัญชีตัวเอง (แนวเดียวกับ change-password) ส่วนการแก้ user
คนอื่นยังต้องมี `users.edit` เหมือนเดิม. รูปเก็บเป็น bytes ในคอลัมน์ `users.avatar` (ไม่มี object
storage — จำกัด 512KB, ชนิดตัดสินจาก content sniffing ไม่เชื่อ Content-Type ของ client, ไม่รับ SVG)
เสิร์ฟที่ `GET /api/users/{id}/avatar` (login แล้วอ่านได้ทุกคน เพราะรูปโผล่ในลิสต์ access อยู่แล้ว)
โดย `user.avatar_url` มี `?v=<unix>` เป็น cache-buster. ชื่อที่แสดงทุกที่ผ่าน `userTitle()`
(`lib/user-display.ts`) = display_name → username — อย่า inline fallback chain เอง

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
   # เจาะจง username อื่น: ... -reset-admin-password -username=someone
   ```
   มันสุ่ม password ใหม่ + `must_change_password=true` + bump `token_version` (session เก่าตายหมด)
   แล้วพิมพ์ password ครั้งเดียว (ถ้า username นั้นยังไม่มีในระบบจะสร้างใหม่เป็น admin ให้เลย)
   > กรณี dev (service รันนอก docker): `go run ./apps/control-plane/cmd/server -reset-admin-password`
   > โดย set `DATABASE_URL` ให้ชี้ dev postgres (ดู `make run-control-plane` เป็นตัวอย่าง env)

**ถามบ่อย: ต้องมี connection ของ DB ทุกตัวไหมถึงจะเข้าใช้/รีเซ็ตได้?** — ไม่ต้อง.
เข้าเว็บ/login/เปลี่ยน password ปกติ ใช้แค่ผ่าน control-plane (คนใช้ไม่แตะ DB ตรง ๆ อยู่แล้ว).
การรีเซ็ต password ยุ่งกับ **Postgres อย่างเดียว** — Redis เป็นแค่ login rate-limit (fail-open ถ้าล่ม),
NATS เป็นแค่ job transport ไม่เกี่ยวกับ auth. ทั้ง Postgres/Redis/NATS อยู่ network `core` (internal)
เข้าจากภายนอกไม่ได้ ต้อง `docker compose exec` เข้า container หรือใช้ CLI ข้างบนที่รันในวงเดียวกัน

**สร้าง server**: POST /api/servers (ต้องมี **ทั้ง** cap `servers.create` และ `games.{game}`
ของเกมนั้น ไม่งั้น 403 `forbidden` — ดูหัวข้อ "สิทธิ์สร้างต่อเกม" ด้านล่าง; รับ `game` เป็น optional — ว่าง = เกม default;
`game_version` ของเกมที่ค่านี้เดินทางไปเป็น argument ของเครื่องมือฝั่ง agent เช่น Steam branch
ของ zomboid ต้องผ่าน `Version.Valid` ซึ่งเป็น allow-list — และ agent เช็คซ้ำเองอีกชั้นเสมอ)
→ insert แถว (status=provisioning) + job `create_server` → agent
โหลด artifact + เขียน seed config/launch script → JobResult → status=stopped → user สั่ง start ต่อ

**`/servers/new` = หน้าเลือกเกมก่อน แล้วค่อยเข้า wizard ของเกมนั้นที่ `/servers/new/{game}`** —
เกมไม่ใช่ field ในฟอร์มอีกแล้ว (ไม่มี dropdown ใน step general) เพราะ **ลำดับ step ของแต่ละเกมไม่เท่ากัน**
เปลี่ยนเกม = เปลี่ยนหน้า. หน้าเลือกเกมเป็นการ์ดต่อเกม (`components/server/new-server/game-card.tsx`)
= ปก + ชื่อ + คำอธิบายสั้น โดยรายการมาจาก `GET /api/meta/games` (**ไม่ถูกกรองตามสิทธิ์** — เกมที่
`can_create=false` โชว์การ์ดล็อกไว้ กดไม่ได้ ดีกว่าซ่อนจนดูเหมือนระบบไม่รองรับ) ส่วนปก/คำอธิบายมาจาก
`GameProfile` ฝั่ง web (`coverSrc` ชี้ไฟล์ใน `public/games/` ที่ **วาดเอง — ห้ามใช้ logo/asset ของเกมจริง**,
`descriptionKey` เป็น i18n key)

**Wizard สร้าง instance ที่ step สุดท้ายเท่านั้น** — **1 step = 1 component**
ใน `components/server/new-server/` (`step-general` / `step-properties` / `step-access` / `step-players`,
`step-indicator`, `steps.ts` = catalog ของ step ที่ระบบรู้จัก) โดย state อยู่ในฮุก: `use-server-metadata`
(ฟอร์มพื้นฐาน — คืนค่า+setter ไม่คืน JSX; รับ `game` เป็น argument ไม่ใช่ state), `use-create-server`
(ลำดับการสร้างทั้งหมด) — `page.tsx` เหลือแค่ประกอบร่าง + ถือ draft state.
**ลำดับ step เป็นของเกม ไม่ใช่ของ wizard** — `GameProfile.wizardSteps` เป็นคนบอก (minecraft:
general/properties/access/players, zomboid: ไม่มี players เพราะ panel เขียนรายชื่อผู้เล่นไม่ได้)
เพิ่ม step ใหม่ = เพิ่มไฟล์ `step-*.tsx` + key ใน `WizardStepKey` + แถวใน `STEP_TITLE_KEYS`
+ ใส่ key ลงใน `wizardSteps` ของเกมที่ต้องใช้ + `case` ใน `stepContent()`:
step แรกต้องเป็น `general` เสมอ และทุก step เป็น **draft ในหน้าเว็บล้วน ยังไม่ยิง API
สร้างอะไรทั้งนั้น** จึงถอยกลับไปแก้ได้ทุก step, step หลัง general ข้ามได้ (general บังคับกรอกให้ครบ),
มี node เดียวเลือกให้อัตโนมัติ. ปุ่ม create อยู่ที่ step สุดท้ายที่เดียว แล้วรันตามลำดับ:
POST `/api/servers` → POST permission ตาม access draft → รอ job provisioning จบ →
PUT `/config` **เฉพาะ key ที่ต่างจาก default** (ไฟล์ยังไม่มีตอนนั้น merge จะ append ให้ ที่เหลือตัวเกม
เขียนเองตอน start แรก) → `POST /players` ทีละชื่อ → `setDashboardServerId` + ไป `/dashboard`.
ขั้นหลัง create ล้ม = toast บอกเป็นรายการแล้วไปต่อ (server ถูกสร้างแล้ว ห้าม rollback เงียบ ๆ) —
**ห้ามย้ายการสร้างกลับไปไว้ step แรก**. draft ของ properties ใช้ `GET /api/meta/config`
(catalog + default ที่ไม่ผูก server) ส่วน access/players ใช้โหมด draft ของ `ServerAccess`
(`draft`/`onDraftChange` — เลือก user จาก directory เท่านั้น) กับ `PlayersDraft` (คนละตัวกับ
`ServerPlayers` ที่อ่านไฟล์ ops/banned/usercache จริง)

**ลบ server = soft delete เสมอ** (`servers.deleted_at`):
`DELETE /api/servers/{id}` (cap `servers.delete`, ต้อง stopped/errored) **ไม่ dispatch job และไม่แตะ
ไฟล์เลย** — แค่ set `deleted_at` แล้ว emit `server_removed`. ทุก query filter `deleted_at IS NULL`
(`GetServerByID` ด้วย → endpoint ระดับ server ตอบ 404 ให้ของในถังขยะเอง) ยกเว้น `GetServerByIDAny`
+ `ListAllServersWithDeleted` ที่ใช้เฉพาะ flow นี้. กู้คืน = `POST .../restore` (cap `servers.restore`)
→ emit `server_added`. **ลบจริงคือ `POST .../purge`** (cap `servers.purge`, ทำได้เฉพาะของที่อยู่ใน
ถังขยะแล้ว) = flow delete เดิมทั้งดุ้น (job `delete_server` → agent `RemoveAll` → ลบ row + audit
`server_deleted`). UI ทั้งหมดอยู่ที่ `/admin/servers` (filter active/ถังขยะ/สถานะ/node ทำฝั่ง UI).
⚠️ ของในถังขยะ **ยังจอง `host_port` (UNIQUE ไม่ได้ filter deleted) และยังถูกนับใน RAM admission
control** โดยตั้งใจ — ไฟล์ยังกินที่จริงและ restore ต้องกลับมา start ได้เสมอ

**Start**: job `start_server` (control-plane เลือก image จาก game definition — minecraft:
`game-manager/runtime-java:{8|17|21|25}` ตาม game_version, zomboid: `game-manager/runtime-steam:1`) →
agent **ensure runtime image** ตาม `ImageSource` ของเกม (มีในเครื่องแล้ว = reuse ไม่ทำอะไรซ้ำ;
ไม่มี = pull image official แล้ว tag ซ้ำ **หรือ** build จาก Dockerfile ที่ definition ถือไว้
แล้ว cache ไว้ share ข้าม instance) → สร้าง container
(hardening ครบ: cap-drop ALL, no-new-privileges, user 1000, mem limit, แยก network)
→ docker events → agent ส่ง `ServerStatus RUNNING` ผ่าน gRPC → DB + broadcast WS
- ถ้า start ล้มหลังสร้าง container / container crash (die exit≠0 ที่ไม่ได้สั่ง stop) → agent **ลบ container
  ที่ค้างทิ้งทันที + push console line แจ้ง user** ว่ากำลังเอาออก (ไม่ปล่อยให้ค้างเป็น dead container)
- instance ที่ dir มีแต่ยังไม่มี `.gamemanager/meta.json` = job create ล้มกลางทาง → agent **ปฏิเสธ
  การ start ทันที** (`games.HasInstanceMeta`) แทนที่จะให้ `DefinitionFor` ตกไปใช้เกม default
  แล้วไปเตรียม image ของเกมผิดตัว
- runtime image cache: `game-manager/runtime-java:{8|17|21|25}` + `game-manager/runtime-steam:1`
  build เองด้วย `make runtime-images` ก็ได้ (hardened) หรือปล่อยให้ agent เตรียมเองครั้งแรก
  ที่ต้องใช้ — reuse ตัวที่มีเสมอ ไม่ทำซ้ำ

**Online players / tick rate ต่อ instance**: เกมส่วนใหญ่ไม่มี API ให้ถาม — agent อ่านจาก **console** เอง
(`internal/gamestate` = เครื่องจักรกลางที่ไม่รู้จักเกม + `games.ConsoleSpec` ของเกมนั้นเป็นคนบอกว่า
ยิงคำสั่งอะไรและแปลบรรทัดยังไง — parser จริงอยู่ที่ `internal/games/minecraft/console.go`):
เกาะกับ console session (attach = server รันอยู่ = เขียน stdin ได้), ยิงคำสั่ง
`RosterCommand` (minecraft = `list`) ตอน attach + ทุก 30 วิ เป็น source of truth (ได้ทั้งรายชื่อ +
max players ทุก variant) แล้วอัปเดตทันทีระหว่างรอบจากบรรทัด join/left ของเกม.
**metric (minecraft = TPS) มีเฉพาะบาง variant** — probe ด้วย `MetricCommand` ครั้งแรก
ถ้าเจอ `Unknown command` จำไว้แล้วเลิกถามตลอด session
(`tick_rate=0` = variant นี้ไม่รองรับ ไม่ใช่ "ค่าเป็นศูนย์"). **reply ของคำสั่งที่ agent ยิงเองถูกกรองออกจาก
console ที่ user เห็น** (console.Manager มี `Observer` hook คืน false = ทิ้งบรรทัด) — ระวังตอนแก้ parser:
กรองพลาด = user เห็นคำสั่งผีทุก 30 วิ / parse พลาด = ผู้เล่นหายจาก dashboard
ค่าพวกนี้เดินทางไปกับ `ServerStats` เส้นเดิม (field `online_players`/`max_players`/`tick_rate`)

**เกมที่ไม่มี allowlist/identity service**: `PlayerSpec` เว้นว่างได้ทั้งก้อน — `HasAllowlist()` /
`HasIdentity()` เป็นตัวตัดสิน แล้ว `POST/DELETE /players` ตอบ **409 `unsupported`**
ส่วน GET ยังใช้ได้ (คืน `allowlist_supported: false` + รายชื่อเท่าที่มี) — zomboid เป็นเคสนี้
เพราะ PZ เก็บบัญชีผู้เล่นไว้ใน DB ของตัวเกม ไม่ใช่ไฟล์ที่ panel เขียนแทนได้

**Player action (op/deop/kick/ban/pardon)**: `POST /api/servers/{id}/players/action` ส่งคำสั่งเข้า console
(ต้อง running ไม่งั้น 409 `invalid_state`) — `action` เป็น **allow-list** และ `username` ต้องผ่าน regex
`^[A-Za-z0-9_.*-]{1,32}$` **เสมอ** เพราะชื่อถูกต่อเข้าไปในคำสั่งตรง ๆ (มี `\n` = สั่งอะไรก็ได้บน server)

**Resource monitoring ต่อ instance**: agent วัด container stats ทุก ~5 วิ → gRPC `ServerStats`
→ control-plane เก็บ in-memory cache (ไม่ลง DB — ephemeral) → แนบใน field `stats` ของ server response
→ web แสดง CPU/RAM ต่อ instance (dashboard + หน้า detail). `stats` มี network/block-I/O rate ด้วย
(`net_rx_bps`/`net_tx_bps`/`disk_read_bps`/`disk_write_bps` bytes/sec) และ node stats มี `net_rx_bps`/`net_tx_bps`
(เก็บใน nodes row เหมือน cpu/mem/disk, มาจาก heartbeat) — ทั้งคู่ push ผ่าน `server_stats`/`node_stats` WS ด้วย

**Global capability (RBAC ระดับ panel)**: คนละชั้นกับ `server_permissions` (ต่อ server) —
`users.capabilities` เป็น key array รูป **`{feature}.{action}`** ที่ครอบ CRUD ของทุกฟีเจอร์:
`users.view/create/edit/delete/reset_password`, `nodes.view/create/delete`,
`servers.view_all/create/edit/delete/restore/purge/power`, `console.view/write`, `files.view/write/delete`,
`players.view/manage/moderate`, `settings.view/edit`, `access.view/manage`
**+ `games.{game_id}` หนึ่ง key ต่อเกม** (ดู "สิทธิ์สร้างต่อเกม" ข้างล่าง)
(ตารางเต็ม + endpoint ที่คุมอยู่ใน docs/api.md). is_admin ครอบทุก capability
- **source of truth ของ catalog** = `apps/control-plane/internal/httpapi/capabilities.go`,
  **map endpoint → capability** = ตาราง route เดียวใน `internal/httpapi/api.go` (`requireCap`)
  ยกเว้น console WS ที่เช็คใน `internal/console/ws.go` (นิยาม const ซ้ำกัน import cycle)
- endpoint ระดับ server ต้องผ่าน **ทั้ง capability และ `server_permissions`** (AND) —
  capability คือ "ทำฟีเจอร์นี้ได้ไหมในระดับ panel", server_permissions คือ "กับ server ตัวไหน".
  ชั้น server เก็บ grant เป็น **capability key ชุดเดียวกัน** (subset ที่เป็น server-scoped: `serverScopedCaps`
  ใน capabilities.go) — `effectiveServerCap()` = `is_admin OR (hasCap(cap) AND (owner OR grant มี cap))`.
  ใช้ helper `loadServerCap(cap)` ที่ handler ทุก endpoint ระดับ server (แทน loadServerAccess เดิม).
  `server_permissions.role` มีแค่ `owner` (ได้ทุก server-scoped cap + จัดการ access list, ≥1 เสมอ) กับ
  `member` (ถือ `capabilities[]`). `access.*` เป็นของ owner เท่านั้น (ไม่ grant ราย cap)
- web: เมนู/ปุ่มระดับ panel แสดงตาม `hasCapability`; ระดับ server ใช้ `useActiveServer().can(cap)`
  (= effectiveServerCap ฝั่ง web ใน `lib/capabilities.ts` ต้อง sync catalog + `SERVER_SCOPED_CAPABILITIES`)
- UI จัดการสิทธิ์: ระดับ panel ที่ **หน้า** `/admin/users/{id}/permissions` (role preset `lib/user-roles.ts`
  admin/operator/moderator/viewer) ระดับ server ที่ **แท็บ Access** ต่อ server
  (`components/server/server-access.tsx`, role preset `lib/server-roles.ts` owner/operator/moderator/viewer)
  backend ไม่รู้จัก "preset" เก็บแค่ role + capabilities[]
- **สิทธิ์มาจาก role preset เท่านั้น — ไม่มี role custom** (ทั้ง 2 ชั้น): `PermissionGroups`
  (`components/user/permission-fields.tsx`) เป็น **read-only ล้วน** = หน้าต่างส่องว่า preset ที่เลือกให้
  อะไรบ้าง ไม่ใช่ที่ติ๊กทีละข้อ — **อย่าเติม prop `onToggle`/`onToggleGroup` กลับเข้าไป**.
  key `custom` ใน `RoleKey`/`ServerRoleKey` เหลือไว้เป็น fallback ของ **ข้อมูลเก่า** ที่เคยติ๊กรายข้อ
  ไว้ตอน UI ยังให้แก้ได้ (จงใจไม่ล้างข้อมูลให้ — เขียนทับสิทธิ์ user เงียบ ๆ อันตรายกว่า)
  เลือก preset ทับเมื่อไหร่ก็หายไปเอง; เพิ่ม preset ใหม่ = แก้ที่ `ROLE_PRESETS`/`SERVER_ROLE_PRESETS`

**สิทธิ์สร้างต่อเกม (`games.{game_id}`)**: "สร้าง server ได้ไหม" กับ "เกมไหนได้บ้าง" เป็นคนละคำถาม —
`servers.create` ตอบข้อแรก, `games.{game_id}` ตอบข้อสอง และ `POST /api/servers` ต้องผ่าน **ทั้งคู่**
(ไม่ผ่าน = 403 `forbidden`). กติกาที่ต้องรักษาไว้:
- **catalog ของกลุ่มนี้ dynamic** — `API.capabilityCatalog()` = `baseCapabilityCatalog` + 1 แถวต่อเกม
  ใน registry (label/description มาจาก `Definition` ไม่มีชื่อเกมฝังใน httpapi) เพิ่มเกมใหม่จึงได้
  capability ใหม่มาเองโดยไม่ต้องแก้ catalog — **อย่าฮาร์ดโค้ด `games.minecraft` กลับเข้าไป**
- เป็น **global-only** เหมือน `servers.create` (ไม่อยู่ใน `serverScopedCaps` — grant ต่อ server ไม่ได้)
- ฝั่ง web เป็น **มิติที่ตั้งฉากกับ role preset**: ติ๊กเองได้ที่ `GameAccessFields` (ส่วนเดียวที่แก้ทีละข้อได้ —
  `PermissionGroups` ยัง read-only และซ่อนกลุ่ม `games` ทิ้ง), `matchPreset()` ไม่นับ `games.*` (ไม่งั้น
  แค่เพิ่มเกมให้ก็กลายเป็น role custom) และเลือก preset ทับต้อง **ไม่ล้างสิทธิ์เกมเดิม** (`RolePresetPicker`
  merge ให้แล้ว)
- `GET /api/meta/games` มี `can_create` ต่อแถว (= `servers.create` AND `games.{id}`) ให้หน้าเลือกเกม
  ล็อกการ์ดไว้แต่แรก — **ห้ามกรองรายการเกมตามสิทธิ์** (ซ่อนแล้วดูเหมือนระบบไม่รองรับเกมนั้น)

**Game definition (abstraction ของ "เกม") — อ่านก่อนแตะอะไรที่เป็นของเกมใดเกมหนึ่ง**

`servers.game` ผูก server กับเกมหนึ่งเกม, `servers.variant` = variant
ภายในเกมนั้น. ความรู้เฉพาะเกมอยู่ใน `Definition` ตัวเดียวต่อเกมต่อ app:

- `apps/control-plane/internal/games` — `Definition` + `Registry` (validation/metadata ฝั่ง API):
  `Variants` (id/label/RequiresLicense), `DefaultHostPort`, `HostPortSpan`, `MinMemoryMB`,
  `Version` (MaxLen / List จาก upstream / Valid / RuntimeImage), `Config` (FileName / Fields / Format / EditableWhileRunning),
  `Players` (IdentityService / ValidateUsername / ConsoleSafeUsername / Lookup / Avatar / Allowlist /
  StateFiles / Actions / Playtime), `LicenseName`
  → ค่าจริงของ Minecraft อยู่ใน `internal/games/minecraft`: `versions.go`, `runtime.go`,
  `properties.go`, `players.go`, **`identity.go`** (Mojang lookup — เดิมคือ `internal/mojang`)
  และ **`avatar.go`** (ดึง skin + crop หน้า — เดิมคือ `internal/playerface`)
  ⚠️ ชั้น cache ของรูปผู้เล่นเป็น**ของกลาง** อยู่ที่ `internal/avatarcache` (DB-backed TTL +
  เสิร์ฟรูปเก่าตอน upstream ล่ม) — รับ `Fetcher` จาก definition, **ห้ามยัดโค้ดของเกมลงไปในนั้น**
- `apps/node-agent/internal/games` — `Definition` + `Registry` ฝั่งรันจริง: `Variants`, `Ports`
  (เลข/protocol/HostOffset ต่อ port — เกมมีได้หลาย port และเป็น udp ได้),
  `StopCommand`, `LaunchScript`, `LaunchEnv`, `SeedFiles`, `Provision`, `RuntimeImage`, `ImageSource`,
  `Console` (RosterCommand/MetricCommand/Parse)
  + `InstanceLookup` ที่อ่าน `.gamemanager/meta.json` เพื่อรู้ว่า instance บน disk เป็นเกมอะไร
  → ค่าจริงของ Minecraft อยู่ใน `internal/games/minecraft`
- **`ImageSource` = วิธีเตรียม runtime image เมื่อ node ยังไม่มี** (เป็นความรู้ของเกม ไม่ใช่ของ runner):
  `PullFrom` = pull image official แล้ว tag ซ้ำ (minecraft → `eclipse-temurin:{ver}-jre`) หรือ
  `Dockerfile` = ให้ agent `docker build` เอง (zomboid → `runtime.Dockerfile` ที่ `go:embed` ไว้
  ใน package ของเกม เพราะไม่มี image ที่มี SteamCMD จาก upstream ให้ pull และเราไม่ใช้ของ third-party)
  — ทั้งสองทางตั้ง tag เดียวกันเสมอเพื่อ share cache; namespace มาจาก `GM_RUNTIME_IMAGE_NAMESPACE`
  (default `game-manager`) **ตั้งแล้วต้องตรงกับที่ control-plane เลือกใน definition**
  - `ImageSource.Platform` = image นี้ต้องเป็น arch ไหน ("" = ตาม node) — zomboid pin
    `linux/amd64` เพราะ SteamCMD ของ Valve เป็น binary **32-bit x86** ตัวเดียว
    ⚠️ **agent cross-build ไม่ได้**: Engine API เรียกได้แค่ classic builder ซึ่ง **ไม่สนใจ platform**
    (BuildKit อยู่ใน buildx ที่เป็น CLI plugin) — `EnsureRuntimeImage` จึงเช็ค arch ของ node
    ก่อน build แล้ว error บอกให้ไปสร้างบน node ที่ arch ตรง แทนที่จะ build ผิด arch ทิ้งไว้
    (image ที่ cache ไว้ก็ถูกเช็ค arch ซ้ำก่อน reuse) → **เกมที่ติดตั้งผ่าน SteamCMD ใช้ได้เฉพาะ
    node x86_64** (ARM รันไม่ได้แม้จะ build image ข้าม arch มาให้ — steamcmd segfault ใต้ emulation)
  - ⚠️ `ImageSource(imageRef)` ของแต่ละเกม **ต้องรับเฉพาะ ref ของ runtime image ตัวเอง** —
    ref ที่ไม่ใช่ของตัวเองให้คืน zero value (เคยมีบั๊ก: minecraft แปลง tag ของ `runtime-steam:1`
    เป็น `eclipse-temurin:1-jre` แล้ว error ที่ user เห็นไม่เกี่ยวกับสาเหตุจริงเลย)
- **สอง app ไม่ import ข้ามกัน** (คนละ module) — สิ่งที่ต้องตรงกันคือ **id ของเกมและ variant**
  ซึ่งเดินทางผ่าน job payload (`CreateServer.game` ใน proto) และ `.gamemanager/meta.json`
  ⚠️ การ map เวอร์ชัน → Java image มีสองที่ (control-plane เลือก image ให้ job, agent เลือกให้
  forge installer) — **แก้ที่หนึ่งต้องแก้อีกที่เสมอ** มี test คุมทั้งสองฝั่ง
- `apps/web/lib/games` — `GameProfile` + registry ฝั่ง web (`gameProfile(server.game)` /
  `knownGameProfile(id)` ที่คืน undefined เมื่อ web ยังไม่รู้จักเกมนั้น):
  `isValidPlayerName`, `allowlistEnabledKey`, `highlightConsoleMessage`, `label`/`licenseName`/`licenseUrl`/`metricLabel`,
  **`wizardSteps`** (ลำดับ step ของฟอร์มสร้าง server), **`coverSrc`/`descriptionKey`** (การ์ดในหน้าเลือกเกม)
  → ค่าจริงของ Minecraft อยู่ใน `lib/games/minecraft.ts`, ของ PZ อยู่ใน `lib/games/zomboid.ts`
  **component ห้าม import `lib/games/minecraft` ตรง ๆ** — เรียกผ่าน `gameProfile()` เสมอ
- ชั้นอื่นทั้งหมดทำงานผ่าน definition: httpapi ใช้ `a.gameOf(w, srv)` / `a.gameFromQuery(w, r)`,
  jobs ใช้ `d.runtimeImage(srv)`, provision/runner/gamestate ใช้ registry + `InstanceLookup`
- ศัพท์ใน contract เป็นคำกลางทั้งหมดแล้ว (เปลี่ยนจากของเดิมตอนที่ระบบยังผูกกับ Minecraft):
  `mc_version`→`game_version`, `server_type`→`variant`, `whitelist*`→`allowlist*`, `tps`→`tick_rate`,
  `accept_eula`→`accept_license`, `/properties`→`/config`, `/meta/server-types`→`/meta/variants`,
  `/players/{uuid}/face`→`/avatar`, `mojang_unavailable`→`identity_unavailable`,
  ตาราง `player_faces`→`player_avatars`
  ศัพท์ของเกมโผล่ได้เฉพาะ **ค่า** ที่ definition ส่งมา (เช่น `LicenseName = "Minecraft EULA"`)
  — **อย่าเปลี่ยนชื่อ field พวกนี้โดยไม่แก้ web + docs/api.md พร้อมกัน**

**เพิ่มเกมใหม่ / แก้ของเกมเดิม (เช็คลิสต์บังคับ)**
1. แก้พฤติกรรมของ Minecraft = แก้ใน `internal/games/minecraft` (Go) หรือ `lib/games/minecraft.ts` (web)
   **ห้ามแก้ที่ handler/runner/component** ถ้าสิ่งนั้นเป็นความรู้ของเกม
2. เพิ่มเกมใหม่ = เพิ่ม package `internal/games/<เกม>` **ทั้งสองฝั่ง** (id ต้องตรงกัน) แล้ว
   ลงทะเบียนใน `games.NewRegistry(...)` ที่ `apps/control-plane/cmd/server/main.go` และ
   `apps/node-agent/cmd/agent/main.go`
3. web: เพิ่ม `lib/games/<เกม>.ts` (implement `GameProfile`) แล้วลงทะเบียนใน `PROFILES`
   ที่ `lib/games/index.ts` + วาดปกไว้ที่ `public/games/<เกม>.svg` (ห้ามใช้ asset ของเกมจริง)
   + เพิ่ม i18n `game.<เกม>.description` ทั้ง `en.ts`/`th.ts`. `DEFAULT_GAME_ID` เป็น **fallback
   ตอนไม่รู้ว่า instance เป็นเกมอะไรเท่านั้น** — การสร้าง server เลือกเกมจากหน้า `/servers/new`
   (รายการจริงจาก `GET /api/meta/games`) แล้วทุก query ของ wizard (variants/versions/config/next-port)
   ผูกกับเกมนั้นผ่าน route param
4. **ไม่ต้องแตะ schema** เพื่อรองรับ variant ใหม่ — schema ไม่มี CHECK รายชื่อ (ดูกฎเหล็กข้อ 0)
   และ **ไม่ต้องเพิ่ม capability เอง** — `games.{id}` ของเกมใหม่โผล่ใน catalog อัตโนมัติ
   (ต้องไปติ๊กให้ user ที่ `/admin/users/{id}/permissions` เอง ไม่มี backfill)
5. `docs/api.md` + `docs/architecture.md` (หัวข้อ Game definition)

**เพิ่ม feature ใหม่ → ต้องแตะ permission (เช็คลิสต์บังคับ)**
1. `internal/httpapi/capabilities.go` — เพิ่ม const + entry ใน `capabilityCatalog`
   (`{key, group, action, label, description}`, key ต้องเป็น `{group}.{action}`)
   ฟีเจอร์ที่ถูกลบ = **ลบ key ออกจาก catalog** (key ที่ค้างอยู่ใน `users.capabilities` ของ DB เดิม
   ไม่มีใครล้างให้ — ยังไม่มีระบบ migration; dev ล้างด้วย `make reset`)
2. `internal/httpapi/api.go` — ผูก `requireCap(...)` กับ route ใหม่ทุกเส้น (ทั้ง read และ write)
3. capability ใหม่ **ไม่ถูก backfill ให้ user เดิม** ด้วยเหตุผลเดียวกัน — ต้องไปติ๊ก preset ให้ใหม่
   ที่ `/admin/users/{id}/permissions` เอง (หรือ reset DB ตอน dev)
4. `docs/api.md` — เพิ่มแถวในตาราง Capabilities + คอลัมน์สิทธิ์ของ endpoint นั้น
5. web: `lib/capabilities.ts` (key), `lib/user-roles.ts` (ควรอยู่ใน preset ไหน),
   `lib/i18n/en.ts` + `th.ts` (`permGroup.<group>` ถ้าเป็นกลุ่มใหม่, `permAction.<action>`,
   `permDesc.<key>`) — ไม่ใส่ i18n = UI ตกไปใช้ label อังกฤษจาก API (ยังไม่พัง แต่ผิด convention)
6. gate ปุ่ม/เมนูฝั่ง web ด้วย `hasCapability` ให้ตรงกับที่ backend บังคับ

**Console**: agent attach stdout/stderr ของ container → batch ~100ms → gRPC `ConsoleOutput` →
control-plane เก็บ ring buffer 500 บรรทัด + broadcast WS → browser (xterm.js)
ขาเข้า: WS `{"type":"input"}` → เช็ค cap `console.write` + grant `console.write` ต่อ server (owner/admin ข้าม) ต่อ message
(โหลด user/permission ใหม่จาก DB ทุกครั้ง — สิทธิ์ที่ถูกถอดต้องมีผลทันที) → audit → gRPC `ConsoleInput` → stdin

**Agent identity**: agent มีแค่ token → gRPC auth (SHA-256 lookup) → control-plane ส่ง `Welcome{node_id}`
→ agent ค่อยเปิด NATS consumer `agent-{node_id}` ได้

## Security posture (จุดที่ต้องระวังเวลาแก้)

- docker.sock ใน node-agent = จุดเสี่ยงสุดของระบบ ทุก input จาก job ต้อง validate ก่อนถึง docker API
- Postgres/Redis/NATS อยู่ network `core` (internal, ไม่มี egress) — อย่า publish port / อย่าย้าย
  container ของ instance เข้า core เด็ดขาด (อยู่ `game-manager-servers` เท่านั้น)
- dev compose bind 127.0.0.1 ทุก port — อย่าเปลี่ยนเป็น 0.0.0.0
- WS ต้องเช็ค Origin ทุก handshake / cookie เป็น SameSite=Lax / GET ห้ามมี side effect (นี่คือแนวกัน CSRF)
- Backlog ด้าน security ที่รู้อยู่แล้ว (อย่าทำเงียบ ๆ ให้ปรึกษาก่อน): TLS ของ gRPC/NATS ข้ามเครื่อง,
  NKey/JWT ต่อ node, docker-socket-proxy, readonly rootfs ของ container ของ instance, 2FA

**File manager**: interactive file ops (list/read/write/mkdir/rename/delete) วิ่งผ่าน **gRPC stream**
(control-plane→agent `FileRequest`, agent→control-plane `FileResponse`, correlate ด้วย request_id) —
ไม่ผ่าน NATS (ไม่ใช่ lifecycle job) และ agent ไม่เปิด port. ทุก path ผ่าน `SafeJoin` (jail = dir ของ server)
ก่อนแตะ filesystem. ต้องมี cap `files.view/write/delete` ตาม action **ทั้ง 2 ชั้น** (global AND grant
ต่อ server; owner/admin ข้าม) — enforce ผ่าน `loadServerCap(cap)`. REST อยู่ `/api/servers/{id}/files*` (ดู docs/api.md)

**Allowlist/players**: control-plane verify username กับ identity service ของเกม (minecraft = Mojang,
`internal/games/minecraft/identity.go`, egress ผ่าน edge) → เก็บใน DB `server_players`
(source of truth) → rebuild ไฟล์รายชื่อที่ root ของ server (ชื่อไฟล์+รูปแบบจาก `Allowlist` ของ
definition; minecraft = `whitelist.json`) แล้วเขียนผ่าน agent FileWrite (stream เดียวกับ file manager,
SafeJoin ที่ agent) → ถ้า running ส่ง `Allowlist.ReloadCommand` เข้า console (best-effort). ต้องมี cap `players.view/manage` (+ `players.moderate` สำหรับ op/kick/ban)
คู่กับสิทธิ์ต่อ server เท่า file manager. REST `/api/servers/{id}/players*`. ⚠️ ต้อง `white-list=true`
ใน server.properties ถึงจะ enforce จริง; UUID จาก Mojang ใช้กับ `online-mode=true` เท่านั้น (offline-mode คนละ UUID)
- **GET players = unified list**: merge allowlist จาก DB + ไฟล์ state ของเกม (`StateFiles` ของ
  definition — minecraft: `usercache.json` seen / `ops.json` op / `banned-players.json` banned)
  โดย key ด้วย uuid (normalize dash/case) — อ่านไฟล์ผ่าน agent FileRead.
  ไฟล์ไม่มี = ว่าง (ไม่ error); **node offline = degrade** เหลือ allowlist จาก DB (แท็บยังใช้ได้ตอน server หยุด)
- **Access picker**: `GET /api/users/directory` (authed ทุกคน ไม่ใช่ `users.view`) คืน user ที่ active
  แบบ field เบา ให้ owner เลือก collaborator; POST permission รับ `user_id` (จาก directory) หรือ `username`

**Properties แก้ได้เฉพาะ stopped/errored**: PUT `/config` ตอบ 409 `invalid_state` ถ้า server ไม่หยุด
(เกมเขียนทับไฟล์ config ตอน shutdown — แก้ตอนรันจะหาย); GET/read ทำได้ทุกสถานะ

**`memory_mb` = container limit ไม่ใช่ heap**: ค่าที่ user ตั้ง = hard limit ของทั้ง container
(cgroup memory + memorySwap) → `stats.memory_limit_mb` คืนค่าเดียวกับที่ตั้ง. agent คำนวณ `-Xmx`
เองด้วย `minecraft.HeapMB()` (`internal/games/minecraft/launch.go` ของ agent — เป็นของ game
definition เพราะเป็นเรื่องของ JVM ที่เกมนี้ใช้; runner แค่เรียก `def.LaunchEnv(memoryMB)`) โดยกัน non-heap ของ JVM ไว้ ~1/3 (floor 256MB,
cap 2048MB, ไม่เกินครึ่งของ limit) แล้วส่งเข้า container เป็น env `GM_MEMORY_MB` ให้ launch.sh ใช้ —
**อย่ากลับไปตีความ `memory_mb` เป็น heap** เพราะจะทำให้ admission control ข้างล่างนับต่ำกว่าจริง 1.5x

**RAM admission control**: create/ขยาย `memory_mb` เช็คผลรวม `memory_mb` ทุก server บน node
(`SumServerMemoryMBOnNode`) + ตัวใหม่ ต้องไม่เกิน `node.memory_total_mb` → ไม่งั้น 400 `insufficient_memory`
(body มี `used_mb/total_mb/available_mb`). node total=0/ไม่รู้ = ข้ามเช็ค. PATCH เช็คเฉพาะตอนขยาย (ไม่นับ memory เดิมตัวเอง)

**Default host port**: `GET /api/meta/next-port?node_id=&game=` คืน host_port ว่างต่ำสุดบน node
(จุดเริ่มมาจาก `Definition.DefaultHostPort` — minecraft 25565, zomboid 16261) ให้ web prefill
ฟอร์มสร้าง server — suggestion เท่านั้น ไม่ reserve (create ยัง enforce UNIQUE (node_id, host_port))
⚠️ เกมที่กิน host port ติดกันหลายตัว (`Definition.HostPortSpan`, zomboid = 2 เพราะ PZ ต้องเปิด
16261+16262/udp) — DB เก็บแค่ port หลัก การเว้นช่วงจึงอยู่ที่ `nextFreeHostPort` ใน httpapi
ที่ขยาย port ที่ถูกจองตาม span ของเกมนั้น ๆ **ไม่ใช่ UNIQUE constraint** (ตั้ง port เองทับกันได้อยู่ —
จะไปพังตอน start แล้ว agent รายงานว่า port ถูกใช้อยู่)

## สิ่งที่ยังไม่มี (อย่าเข้าใจผิดว่ามีแล้ว)

- **ผู้เล่นออนไลน์ของ zomboid**: ยังอ่านไม่ได้ (`online_players` = 0 เสมอ) — คำสั่ง `players` ของ PZ
  ตอบเป็นหลายบรรทัด แต่ `games.ConsoleSpec.Parse` ตีความทีละบรรทัดอิสระ (EventRoster = 1 บรรทัด
  = ทั้ง roster) ต้องรอรอบที่แก้ console model ให้ parser มี state ก่อน. ที่อ่านได้แล้วคือ event ready
- Playtime ของผู้เล่น: อ่านจาก `{level-name}/stats/{uuid}.json` ตอนเรียก GET players — จำกัด 50 คน/request
  (เกินนั้นคืน 0 = ไม่รู้) เพราะเป็นไฟล์ละคน = N round-trip ต่อการเปิดหน้า
- File manager: อัปโหลด/ดาวน์โหลดไฟล์ใหญ่แบบ binary (artifact/mod) — ตอนนี้รองรับแค่ browse/แก้ไฟล์ text/mkdir/rename/delete
  (ไม่มี binary upload ทางไหนเลย ตั้งแต่ถอดฟีเจอร์ import server ออก)
- **นำเข้า server เดิมจาก .zip (import server)** — ถูกถอดออกทั้งระบบแล้ว (endpoint, job
  `import_server`, `FileWriteChunk`, หน้าเว็บ). field number ที่เคยใช้ถูก
  `reserved` ไว้ใน proto (`JobEnvelope` 15, `FileRequest` 16) เผื่อกลับมาทำใหม่
- Backup/restore, scheduler, mod/plugin browser (online-player list + player action มีแล้ว ดูหัวข้อข้างบน)
- OIDC (Discord/Google), quota ต่อ user, multi-node TLS
- Metrics/alerting ระยะยาว (มี resource monitoring ต่อ instance แบบ realtime แล้ว แต่ไม่เก็บ history/ไม่มี alert)
