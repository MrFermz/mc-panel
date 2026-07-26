#!/usr/bin/env bash
# preflight.sh — เช็ค environment ก่อนรัน game-manager (โดยเฉพาะ WSL2/Docker Desktop บน Windows)
# ดัก misconfig ที่พบบ่อยตั้งแต่ต้น: docker เข้าไม่ได้, repo/GM_DATA_DIR อยู่บน drvfs (/mnt/*)
# dependency-free — ใช้แค่ bash + docker CLI. exit != 0 เฉพาะ hard failure (ไม่มี docker) เท่านั้น
set -euo pipefail

# โหลด GM_DATA_DIR จาก .env ถ้ามี (ไม่ต้องพึ่ง env ที่ export ไว้แล้ว)
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ -f "$ROOT/.env" ]]; then
  # อ่านเฉพาะ GM_DATA_DIR แบบ safe (ไม่ source ทั้งไฟล์ — กัน side effect จาก secret)
  # \042 = double quote, \047 = single quote — strip both ถ้า value ถูก quote ไว้
  GM_DATA_DIR="$(grep -E '^GM_DATA_DIR=' "$ROOT/.env" | tail -n1 | cut -d= -f2- | tr -d '\042\047' || true)"
fi
GM_DATA_DIR="${GM_DATA_DIR:-}"

pass() { printf 'PASS  %s\n' "$1"; }
warn() { printf 'WARN  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1"; }

fail_hard=0

echo "game-manager preflight"
echo "=================="

# 1. docker reachable — hard failure ถ้าไม่ได้
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  pass "docker daemon reachable"
else
  fail "docker daemon not reachable — start Docker Desktop and enable WSL integration for this distro (docker info must run)"
  fail_hard=1
fi

# 2. docker compose v2 available
if docker compose version >/dev/null 2>&1; then
  pass "docker compose v2 available ($(docker compose version --short 2>/dev/null || echo 'ok'))"
else
  warn "docker compose v2 not found — use the compose plugin (docker compose ...), not the old docker-compose"
fi

# 3. repo path + GM_DATA_DIR ไม่ควรอยู่ใต้ /mnt/ (Windows drvfs ผ่าน WSL)
under_mnt() { case "$1" in /mnt/*) return 0 ;; *) return 1 ;; esac; }
DRVFS_REASON="drvfs: chown is a no-op (game servers run as user 1000 and must be able to chown) + bind mounts are very slow — move to a WSL ext4 home, e.g. ~/game-manager"

if under_mnt "$ROOT"; then
  warn "repo is on $ROOT (Windows drive via /mnt) — $DRVFS_REASON"
else
  pass "repo path is on a Linux filesystem ($ROOT)"
fi

if [[ -n "$GM_DATA_DIR" ]]; then
  if under_mnt "$GM_DATA_DIR"; then
    warn "GM_DATA_DIR is on $GM_DATA_DIR (Windows drive via /mnt) — $DRVFS_REASON"
  else
    pass "GM_DATA_DIR is on a Linux filesystem ($GM_DATA_DIR)"
  fi
fi

echo "=================="
if [[ "$fail_hard" -ne 0 ]]; then
  echo "preflight: FAILED (fix the FAIL items above first)"
  exit 1
fi
echo "preflight: OK (see WARN items above, if any)"
