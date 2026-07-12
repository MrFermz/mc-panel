# mc-panel

ระบบจัดการ Minecraft server หลาย instance ผ่านเว็บ เขียนขึ้นเองทั้งหมด ไม่ fork จากโปรเจกต์อื่น
รองรับ Vanilla, Paper (plugin), Fabric/Forge (modded), และ Velocity proxy

ทุกอย่างรันบน Docker: ระบบหลัก (web/control-plane/agent/db) และ MC ทุก instance
แยก docker network ระหว่างระบบหลักกับ MC servers ชัดเจน

- สถาปัตยกรรม + design decisions: [`docs/architecture.md`](docs/architecture.md)
- REST/WS contract: [`docs/api.md`](docs/api.md)
- Convention สำหรับ AI/คนที่มาทำต่อ: [`CLAUDE.md`](CLAUDE.md)

## โครงสร้าง repo

แบ่ง directory ตามขอบเขต service — เตรียมไว้แยกเป็น git submodules เมื่อมี repo จริง
(แต่ละ app มี Dockerfile + go.mod/package.json ของตัวเอง ไม่ import ข้าม app)

```
apps/
  control-plane/   API server (Go) — auth, DB, job dispatch, gRPC hub, WS console
  node-agent/      Agent (Go) — คุม docker, provision jar, console attach
  web/             Frontend (Next.js 15 + shadcn/ui)
packages/
  proto/           .proto + generated Go (source of truth ของ message format)
  shared-types/    TS types ที่ generate จาก proto
infra/             docker-compose (dev + full stack), caddy, nats config, mc-runtime image
scripts/           gen-env.sh และ script ประกอบ
docs/              architecture + api contract
```

## Deploy บนเครื่องใหม่ (full stack — ทุกอย่างบน docker)

นี่คือวิธีเอาระบบขึ้นเครื่องใหม่ (staging/production หรือ dev ที่ไม่ต้องแก้โค้ด)

**ต้องมีบนเครื่อง:** Docker Engine + Docker Compose v2, `make`, `git` — **ไม่ต้องมี Go/Node**
(compile Go และรัน migration ทำใน container ให้หมด)

```bash
git clone <repo-url> mc-panel && cd mc-panel
make env              # สร้าง .env — สุ่ม secret ทุกตัว (ทำครั้งเดียว, ห้าม commit .env)
make runtime-images   # build base image ของ MC (8/17/21/25) — ข้ามได้ agent auto-pull ให้ตอนใช้ครั้งแรก
make up               # build image + เปิดทั้งระบบ
make admin-password   # ดู email/password ของ admin คนแรก (พิมพ์ลง log ครั้งเดียว)
```

เปิด `http://localhost:8000` → login → ระบบบังคับตั้ง password ใหม่ → ใช้งานได้เลย
(ลืม password admin: `make admin-reset-password`)

User ใหม่ทุกคนใช้ flow เดียวกัน: admin สร้าง user → ได้ password สุ่มแสดงครั้งเดียว
→ user คนนั้น login แล้วถูกบังคับตั้ง password ของตัวเอง

### ข้อมูล / backup / ย้ายเครื่อง

- **Postgres** (users, servers, jobs, permissions) → docker named volume
- **ไฟล์ของแต่ละ MC instance** → `MC_DATA_DIR` = `<repo>/data/servers/<server_id>` (bind mount)
- **Backup** = dump Postgres + `tar` โฟลเดอร์ `data/servers` + เก็บ `.env` ไว้ด้วย
  (ถ้า `.env` หาย = secret เดิมหาย, decrypt/verify ของเก่าไม่ได้)

### Production ที่มี domain จริง + HTTPS

1. แก้ [`infra/caddy/Caddyfile`](infra/caddy/Caddyfile): เปลี่ยน `:80` เป็น `yourdomain.com` และลบ `auto_https off`
   (Caddy ขอ Let's Encrypt ให้อัตโนมัติ — ต้องเปิด port 80+443 และ DNS ชี้มาเครื่องนี้)
2. ใน [`infra/docker-compose.yml`](infra/docker-compose.yml) เปลี่ยน port ของ caddy จาก `8000:80` เป็น `80:80` + `443:443`
3. ตั้ง `COOKIE_SECURE=true` ใน `.env` (cookie จะส่งเฉพาะ HTTPS)
4. `ALLOWED_ORIGINS` ปล่อยว่างได้ (default = อนุญาต Origin ที่ host ตรงกับ request) หรือระบุ `https://yourdomain.com` ชัด ๆ

> แยก node-agent ไปอีกเครื่อง (multi-node): มี `make agent-image` + `scripts/install-agent.sh`
> แต่ TLS ของ gRPC/NATS ข้ามเครื่องยังเป็น backlog (ดู [`CLAUDE.md`](CLAUDE.md)) — เครื่องเดียวจบใช้ `make up` พอ

### Windows + Docker Desktop

container เป็น Linux ทั้งหมด Docker Desktop รันผ่าน VM Linux (WSL2) ให้อยู่แล้ว **ไม่ต้องแก้โค้ด** แต่ต้อง setup ให้ถูก:

- **รันทุกอย่างใน WSL2 (เช่น Ubuntu) ไม่ใช่ PowerShell** — ต้องใช้ `make`/`bash`. เปิด Docker Desktop → Settings → **WSL integration** ให้ distro นั้น (`docker`/`docker compose` + `/var/run/docker.sock` จะใช้ได้ใน WSL)
- **วาง repo + `MC_DATA_DIR` บน filesystem ของ WSL2 (ext4) เท่านั้น** เช่น `~/mc-panel` — **ห้ามวางบน `/mnt/c/...`**
  (agent ใน container สั่ง docker daemon bind-mount `MC_DATA_DIR/<id>` แบบ path ต้องตรงกันทั้งใน/นอก และ MC รันเป็น user 1000 ต้อง chown ได้ — บน `/mnt/c` chown เป็น no-op + ช้ามาก)
- **Line endings ต้องเป็น LF** — repo มี `.gitattributes` บังคับ `eol=lf` ให้ scripts/Go/SQL/proto อยู่แล้ว
  ถ้า clone ใหม่ไม่ต้องตั้ง git config เอง (`gen-env.sh`/entrypoint จะไม่โดน CRLF)
- **เช็ค environment ก่อนเริ่ม** — รัน `make doctor` (เรียก `scripts/preflight.sh`) ดัก misconfig ที่พบบ่อย:
  docker เข้าไม่ได้, docker compose v2 ไม่มี, หรือ repo/`MC_DATA_DIR` วางบน `/mnt/...` (drvfs)

## Dev บนเครื่องใหม่ (service รันนอก docker, hot reload)

สำหรับเขียนโค้ดต่อ — infra (postgres/redis/nats) อยู่ docker ส่วน 3 service รันตรงบนเครื่องเพื่อ hot reload

**ต้องมีเพิ่มจาก full stack:** Go 1.24+, Node.js + pnpm (buf/goose ไม่ต้องลง — Makefile เรียกผ่าน `go run`)

```bash
git clone <repo-url> mc-panel && cd mc-panel
make bootstrap           # .env + dev infra (postgres/redis/nats) + migrate + ลง web deps ให้
make run-control-plane   # terminal 1
make run-agent           # terminal 2 (ต้องมี docker บนเครื่อง)
make run-web             # terminal 3 → http://localhost:3000
make admin-password      # (ครั้งแรก) ดู credentials ของ admin — control-plane พิมพ์ลง log ตอน boot
```

แก้ `.proto` ต้อง `make proto-gen` แล้ว commit generated code; ก่อนถือว่าเสร็จรัน `make test` + `make lint`

## คำสั่งที่ใช้บ่อย

พิมพ์ `make help` ดูทั้งหมด

| คำสั่ง | ใช้ตอนไหน |
|---|---|
| `make logs` | ดู log full stack แบบ real-time |
| `make down` / `make infra-down` | ปิด (data ยังอยู่) |
| `make reset` | ล้าง DB dev แล้ว migrate ใหม่ |
| `make purge` | ล้างทุกอย่างจนหมด (ต้องพิมพ์ยืนยัน) |
| `make proto-gen` | แก้ .proto แล้ว generate ใหม่ (ต้อง commit generated code) |
| `make test` / `make lint` | ก่อนถือว่างานเสร็จ |

## เครื่องมือที่ต้องมี

- **Full stack (`make up`)**: Docker Engine + Compose v2, `make`, `git` — เท่านี้พอ
- **Dev (hot reload)**: เพิ่ม Go 1.24+, Node.js + pnpm — ส่วน buf/goose ไม่ต้องติดตั้ง (Makefile เรียกผ่าน `go run` ให้)
