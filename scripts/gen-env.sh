#!/usr/bin/env bash
# gen-env.sh — สร้างไฟล์ .env ที่ root ของ repo พร้อม secret แบบสุ่มทั้งหมด
# ใช้ครั้งแรกครั้งเดียวต่อเครื่อง (ถ้ามี .env อยู่แล้วจะไม่ทับ เว้นแต่ใส่ --force)
#
# ห้าม hardcode password ใน compose file เด็ดขาด — ทุก secret ต้องมาจาก .env เท่านั้น
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="$ROOT/.env"

if [[ -f "$ENV_FILE" && "${1:-}" != "--force" ]]; then
  echo ".env already exists at $ENV_FILE — to recreate it (all old secrets become invalid) use: $0 --force"
  exit 0
fi

rand() { openssl rand -hex "$1"; }

cat > "$ENV_FILE" <<EOF
# ============================================================
# mc-panel secrets — generate โดย scripts/gen-env.sh
# ห้าม commit ไฟล์นี้ (.gitignore กันไว้แล้ว)
# ============================================================

# PostgreSQL
POSTGRES_USER=mcpanel
POSTGRES_PASSWORD=$(rand 24)
POSTGRES_DB=mcpanel

# Redis
REDIS_PASSWORD=$(rand 24)

# NATS — แยก user ระหว่าง control-plane กับ agent (คนละ permission)
NATS_CONTROL_USER=control-plane
NATS_CONTROL_PASSWORD=$(rand 24)
NATS_AGENT_USER=agent
NATS_AGENT_PASSWORD=$(rand 24)

# JWT signing secret ของ control-plane (HS256)
JWT_SECRET=$(rand 32)

# Token ของ node ตัวแรก (all-in-one compose) — control-plane seed node "local" ให้อัตโนมัติ
# เป็น opaque token (เก็บเป็น SHA-256 ใน DB) — production หลาย node ให้สร้าง node ผ่าน API แทน
NODE_TOKEN=$(rand 32)

# admin คนแรก — login คือค่านี้ (username-only account; ใส่ email ที่มี "@" ก็ได้ถ้าต้องการ)
# password จะถูก generate ตอน control-plane boot ครั้งแรก แล้วพิมพ์ลง log ครั้งเดียว
# (docker compose logs control-plane | grep -A3 "INITIAL ADMIN")
ADMIN_USERNAME=admin

# path บน host สำหรับข้อมูล MC servers — ต้องเป็น absolute path
# สำคัญ: path นี้ต้องมองเห็นเหมือนกันทั้งจากใน agent container และจาก docker daemon
# (agent ทำ bind mount ให้ sibling container ด้วย path เดียวกันนี้)
MC_DATA_DIR=$ROOT/data/servers

# port ที่ Caddy เปิดรับหน้าเว็บ
PANEL_HTTP_PORT=8000
EOF

chmod 600 "$ENV_FILE"
mkdir -p "$ROOT/data/servers"
echo "Created $ENV_FILE (chmod 600)"
