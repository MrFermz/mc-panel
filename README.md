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

## รัน full stack (ทุกอย่างบน docker)

```bash
make env              # generate .env (secret สุ่มทั้งหมด) — ครั้งแรกครั้งเดียว
make runtime-images   # build mcpanel/mc-runtime:8/17/21
make up               # build + เปิดทั้งระบบ
make admin-password   # ดู email/password ของ admin คนแรก (แสดงใน log ครั้งเดียว)
```

เปิด `http://localhost:8000` → login → ระบบบังคับตั้ง password ใหม่ → ใช้งานได้เลย

User ใหม่ทุกคนใช้ flow เดียวกัน: admin สร้าง user → ได้ password สุ่มแสดงครั้งเดียว
→ user คนนั้น login แล้วถูกบังคับตั้ง password ของตัวเอง

## Dev (service รันนอก docker, hot reload)

```bash
make bootstrap           # .env + dev infra (postgres/redis/nats) + migrate + web deps
make run-control-plane   # terminal 1
make run-agent           # terminal 2 (ต้องมี docker บนเครื่อง)
make run-web             # terminal 3 → http://localhost:3000
```

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

- Docker Engine (full stack ต้องการแค่นี้ + make)
- Dev เพิ่ม: Go 1.24+, Node.js + pnpm — ส่วน buf/goose ไม่ต้องติดตั้ง (Makefile เรียกผ่าน `go run` ให้)
