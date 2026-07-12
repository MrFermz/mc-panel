# ============================================================
# mc-panel — Makefile
#   dev  = infra บน docker (postgres/redis/nats) + service รันนอก docker แบบ hot reload
#   full = ทุกอย่างบน docker (make up) — ใกล้เคียง production
# secret ทั้งหมดอยู่ใน .env (สร้างด้วย make env) ห้าม hardcode ในไฟล์นี้
# ============================================================

-include .env
export

DEV_COMPOSE  := docker compose --env-file .env -f infra/docker-compose.dev.yml
FULL_COMPOSE := docker compose --env-file .env -f infra/docker-compose.yml
DB_URL        = postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/$(POSTGRES_DB)?sslmode=disable
BUF          ?= go run github.com/bufbuild/buf/cmd/buf@v1.47.2
GOOSE        ?= go run github.com/pressly/goose/v3/cmd/goose@v3.24.1

.PHONY: help
help: ## แสดงคำสั่งทั้งหมดพร้อมคำอธิบาย
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ---------- setup ----------

.PHONY: env
env: ## สร้าง .env พร้อม secret สุ่มทั้งหมด (ทำครั้งแรกครั้งเดียว)
	./scripts/gen-env.sh
	@echo "(Windows/WSL2) แนะนำรัน 'make doctor' เช็ค environment ก่อนเริ่ม"

.PHONY: doctor
doctor: ## เช็ค environment (docker, compose v2, WSL2 drvfs) — แนะนำสำหรับ Windows/Docker Desktop
	./scripts/preflight.sh

.PHONY: runtime-images
runtime-images: ## build mcpanel/mc-runtime:8/17/21/25 (base image ของ MC instance)
	docker build -t mcpanel/mc-runtime:8  --build-arg JAVA_VERSION=8  infra/mc-runtime
	docker build -t mcpanel/mc-runtime:17 --build-arg JAVA_VERSION=17 infra/mc-runtime
	docker build -t mcpanel/mc-runtime:21 --build-arg JAVA_VERSION=21 infra/mc-runtime
	docker build -t mcpanel/mc-runtime:25 --build-arg JAVA_VERSION=25 infra/mc-runtime

.PHONY: agent-image
agent-image: ## build image node-agent อย่างเป็นทางการ (mcpanel/node-agent:local) สำหรับ install-agent.sh
	# context = repo root เพราะ Dockerfile ต้องเห็น packages/proto (ดู apps/node-agent/Dockerfile)
	# label project=mc-panel ให้ purge เก็บกวาดได้ (docker image prune --filter label=project=mc-panel)
	docker build -t mcpanel/node-agent:local \
		--label project=mc-panel \
		-f apps/node-agent/Dockerfile .

# ---------- full stack (ทุกอย่างบน docker) ----------

.PHONY: up
up: ## เปิด full stack ทั้งหมดบน docker (build ให้อัตโนมัติ)
	$(FULL_COMPOSE) up -d --build
	@echo ""
	@echo "เปิดแล้ว: http://localhost:$${PANEL_HTTP_PORT:-8000}"
	@echo "ดู password ของ admin ครั้งแรก: make admin-password"
	@echo "(Windows/WSL2) ถ้า bind mount แปลก ๆ ลองรัน 'make doctor'"

.PHONY: down
down: ## ปิด full stack (data ยังอยู่)
	$(FULL_COMPOSE) down

.PHONY: logs
logs: ## ดู log ของ full stack แบบ real-time
	$(FULL_COMPOSE) logs -f

.PHONY: admin-password
admin-password: ## ดู initial admin credentials จาก log ของ control-plane
	@$(FULL_COMPOSE) logs control-plane 2>/dev/null | grep -A4 "INITIAL ADMIN" || \
		echo "ไม่เจอ — admin อาจถูกสร้างไปแล้ว (ถ้าลืม password ให้ใช้ make admin-reset-password)"

.PHONY: admin-reset-password
admin-reset-password: ## รีเซ็ต password ของ admin (ADMIN_EMAIL) กรณีลืม — สุ่มใหม่ + พิมพ์ครั้งเดียว
	$(FULL_COMPOSE) exec control-plane /control-plane -reset-admin-password

# ---------- dev infra (postgres/redis/nats) ----------

.PHONY: infra-up
infra-up: ## เปิด postgres/redis/nats สำหรับ dev
	$(DEV_COMPOSE) up -d
	@echo "รอ postgres พร้อม..."
	@until $(DEV_COMPOSE) exec -T postgres pg_isready -U $(POSTGRES_USER) > /dev/null 2>&1; do sleep 1; done
	@echo "infra พร้อมแล้ว"

.PHONY: infra-down
infra-down: ## ปิด dev infra (data ยังอยู่)
	$(DEV_COMPOSE) down

.PHONY: infra-logs
infra-logs: ## ดู log ของ dev infra ทั้งหมดแบบ real-time
	$(DEV_COMPOSE) logs -f

# ---------- database ----------

.PHONY: migrate-up
migrate-up: ## รัน migration ทั้งหมดที่ยังไม่ apply (dev — full stack รัน auto ตอน boot)
	$(GOOSE) -dir apps/control-plane/migrations postgres "$(DB_URL)" up

.PHONY: migrate-down
migrate-down: ## ถอย migration ล่าสุด 1 ก้าว
	$(GOOSE) -dir apps/control-plane/migrations postgres "$(DB_URL)" down

# ---------- proto ----------

.PHONY: proto-gen
proto-gen: ## generate Go + TS จาก .proto (generated code ต้อง commit เสมอ)
	cd packages/proto && $(BUF) generate

.PHONY: proto-lint
proto-lint: ## lint proto files
	cd packages/proto && $(BUF) lint

# ---------- build ----------

.PHONY: build-control-plane
build-control-plane: ## build control-plane binary
	go build -o bin/control-plane ./apps/control-plane/cmd/server

.PHONY: build-agent
build-agent: ## build node-agent binary (linux amd64)
	GOOS=linux GOARCH=amd64 go build -o bin/node-agent ./apps/node-agent/cmd/agent

.PHONY: build
build: build-control-plane build-agent ## build ทุกตัว

# ---------- run (dev นอก docker) ----------

.PHONY: run-control-plane
# TRUSTED_PROXY_COUNT=0 ในรีซีปด้านล่าง: dev รันตรงไม่มี Caddy อยู่หน้า จึงเชื่อ
# RemoteAddr ตรง ๆ (default=1 สำหรับ production หลัง Caddy). ประสานกับ agent INFRA —
# บรรทัด env นี้เป็นของ control-plane อย่าลบ/ย้าย
run-control-plane: ## รัน control-plane (dev — ต่อ dev infra บน localhost)
	HTTP_ADDR=:8080 GRPC_ADDR=:9090 \
	DATABASE_URL="$(DB_URL)" \
	REDIS_URL="redis://:$(REDIS_PASSWORD)@localhost:6379/0" \
	NATS_URL="nats://$(NATS_CONTROL_USER):$(NATS_CONTROL_PASSWORD)@localhost:4222" \
	COOKIE_SECURE=false \
	ALLOWED_ORIGINS=http://localhost:3000 \
	TRUSTED_PROXY_COUNT=0 \
	go run ./apps/control-plane/cmd/server

.PHONY: run-agent
run-agent: ## รัน node-agent (dev — ต้องมี docker บนเครื่อง)
	AGENT_TOKEN="$(NODE_TOKEN)" \
	CONTROL_PLANE_GRPC=localhost:9090 \
	NATS_URL="nats://$(NATS_AGENT_USER):$(NATS_AGENT_PASSWORD)@localhost:4222" \
	MC_DATA_DIR="$(MC_DATA_DIR)" \
	MC_NETWORK=mcpanel-servers \
	go run ./apps/node-agent/cmd/agent

.PHONY: run-web
run-web: ## รัน frontend dev server (proxy /api,/ws -> localhost:8080)
	cd apps/web && pnpm dev

# ---------- test & lint ----------

.PHONY: test
test: ## รัน test ทุก service
	go test ./apps/control-plane/... ./apps/node-agent/...
	cd apps/web && pnpm test run 2>/dev/null || true

.PHONY: lint
lint: ## รัน lint ทุก service
	go vet ./apps/control-plane/... ./apps/node-agent/...
	cd apps/web && pnpm lint

# ---------- reset / purge ----------

.PHONY: reset
reset: ## ล้าง DB + infra data ทั้งหมดแล้วตั้งใหม่ (dev)
	@echo "กำลัง reset dev environment..."
	$(DEV_COMPOSE) down -v
	$(DEV_COMPOSE) up -d
	@until $(DEV_COMPOSE) exec -T postgres pg_isready -U $(POSTGRES_USER) > /dev/null 2>&1; do sleep 1; done
	$(MAKE) migrate-up
	@echo "reset เสร็จแล้ว พร้อม dev ต่อ"

.PHONY: purge
purge: ## ล้างทั้งหมด (data, volume, binary, node_modules) ต้องพิมพ์ยืนยัน
	@echo "คำสั่งนี้จะลบ:"
	@echo "  - docker volume ทั้งหมดของ dev + full stack (ข้อมูล DB/NATS หายหมด)"
	@echo "  - โฟลเดอร์ bin/ และ node_modules/.next ของ web"
	@echo "  - หมายเหตุ: ไม่ลบ generated proto (ต้อง commit) และไม่ลบ data/servers"
	@read -p "พิมพ์ 'yes' เพื่อยืนยัน: " confirm; \
	if [ "$$confirm" != "yes" ]; then \
		echo "ยกเลิก"; exit 1; \
	fi
	$(DEV_COMPOSE) down -v --remove-orphans || true
	$(FULL_COMPOSE) down -v --remove-orphans || true
	rm -rf bin/
	rm -rf apps/web/node_modules apps/web/.next
	docker image prune -f --filter "label=project=mc-panel"
	@echo "purge เสร็จแล้ว — รัน 'make bootstrap' เพื่อเริ่ม dev ใหม่ หรือ 'make up' สำหรับ full stack"

# ---------- bootstrap dev ----------

.PHONY: bootstrap
bootstrap: ## เซ็ตอัพ dev ครั้งแรก (env -> infra -> migrate -> web deps)
	@test -f .env || $(MAKE) env
	$(MAKE) infra-up
	$(MAKE) migrate-up
	cd apps/web && pnpm install
	@echo "bootstrap เสร็จแล้ว — เปิด 3 terminal: make run-control-plane / make run-agent / make run-web"
