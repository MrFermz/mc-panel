#!/usr/bin/env bash
# install-agent.sh — รัน node-agent บนเครื่อง node เพิ่มเติม (นอกเหนือจาก node "local"
# ที่มากับ full stack compose อยู่แล้ว)
#
# ขั้นตอนก่อนใช้:
#   1. สร้าง node ผ่านหน้า /admin/nodes (หรือ POST /api/nodes) → ได้ token มา
#   2. เครื่อง node ต้องมี Docker Engine + image mcpanel/mc-runtime:8/17/21 (make runtime-images)
#   3. เครื่อง node ต้องมี image node-agent อยู่ในเครื่องเอง — build ด้วย `make agent-image`
#      แล้ว load เข้าเครื่อง (docker save|load) หรือระบุ --image=<registry ส่วนตัวของคุณ> เอง
#      สคริปต์นี้จงใจไม่ pull จาก Docker Hub namespace สาธารณะที่โปรเจกต์ไม่ได้ควบคุม
#   4. เครื่อง node ต้องเข้าถึง control plane (gRPC :9090) และ NATS (:4222) ได้
#      คำเตือน: ข้ามเครื่องจริงควรมี TLS/VPN คั่น — ดู backlog ใน docs/architecture.md
set -euo pipefail

TOKEN=""
GRPC_ADDR=""
NATS_URL=""
DATA_DIR="/srv/mcpanel/servers"
# default ชี้ image local ที่ build เอง (make agent-image) — ไม่ default เป็น tag บน Docker Hub
# namespace สาธารณะ mcpanel/ โปรเจกต์ไม่ได้ควบคุม ถ้ามีคน squat = รัน image แปลกปลอมที่ถือ
# docker.sock = root ทั้งเครื่อง node. ถ้าจะใช้ registry ส่วนตัว ให้ระบุ --image= เอง
IMAGE="mcpanel/node-agent:local"

usage() {
  echo "ใช้: $0 --token=<node-token> --grpc=<host:9090> --nats=<nats://agent:pass@host:4222> [--data-dir=$DATA_DIR] [--image=$IMAGE]"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --token=*)    TOKEN="${1#*=}" ;;
    --grpc=*)     GRPC_ADDR="${1#*=}" ;;
    --nats=*)     NATS_URL="${1#*=}" ;;
    --data-dir=*) DATA_DIR="${1#*=}" ;;
    --image=*)    IMAGE="${1#*=}" ;;
    *) echo "unknown arg: $1"; usage ;;
  esac
  shift
done

[[ -z "$TOKEN" || -z "$GRPC_ADDR" || -z "$NATS_URL" ]] && usage

command -v docker >/dev/null || { echo "ต้องติดตั้ง Docker Engine ก่อน"; exit 1; }

# กัน docker ไป pull image local โดยบังเอิญจาก Docker Hub namespace สาธารณะ:
# ถ้าใช้ค่า default (build เอง) image ต้องมีอยู่ในเครื่องนี้แล้วเท่านั้น — ไม่งั้น error
# (ถ้าระบุ --image= เป็น registry ส่วนตัวเอง จะข้ามเช็คนี้และปล่อยให้ docker pull ตามปกติ)
if [[ "$IMAGE" == "mcpanel/node-agent:local" ]] && ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  echo "ไม่พบ image '$IMAGE' บนเครื่องนี้"
  echo "  build บน build host: make agent-image  แล้ว load เข้าเครื่อง node (docker save | docker load)"
  echo "  หรือระบุ --image=<registry ส่วนตัวของคุณ>/node-agent:<tag> เอง"
  echo "  (จงใจไม่ pull จาก Docker Hub namespace สาธารณะที่โปรเจกต์ไม่ได้ควบคุม)"
  exit 1
fi

mkdir -p "$DATA_DIR"
docker network inspect mcpanel-servers >/dev/null 2>&1 || \
  docker network create --attachable mcpanel-servers

docker rm -f mcpanel-node-agent >/dev/null 2>&1 || true
docker run -d \
  --name mcpanel-node-agent \
  --restart unless-stopped \
  -e AGENT_TOKEN="$TOKEN" \
  -e CONTROL_PLANE_GRPC="$GRPC_ADDR" \
  -e NATS_URL="$NATS_URL" \
  -e MC_DATA_DIR="$DATA_DIR" \
  -e MC_NETWORK=mcpanel-servers \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$DATA_DIR":"$DATA_DIR" \
  "$IMAGE"

echo "node-agent รันแล้ว — ดูสถานะ node ได้ที่หน้า /admin/nodes"
