# ============================================================
# game-manager — Makefile
#   dev  = infra บน docker (postgres/redis/nats) + service รันนอก docker แบบ hot reload
#   full = ทุกอย่างบน docker (make up) — ใกล้เคียง production
# secret ทั้งหมดอยู่ใน .env (สร้างด้วย make env) ห้าม hardcode ในไฟล์นี้
# ============================================================

-include .env
export

DEV_COMPOSE  := docker compose --env-file .env -f infra/docker-compose.dev.yml
DEV_APP_COMPOSE := docker compose --env-file .env -f infra/docker-compose.dev.yml -f infra/docker-compose.dev.app.yml
FULL_COMPOSE := docker compose --env-file .env -f infra/docker-compose.yml
DB_URL        = postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/$(POSTGRES_DB)?sslmode=disable
BUF          ?= go run github.com/bufbuild/buf/cmd/buf@v1.47.2

.PHONY: help
help: ## Show all commands with descriptions
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ---------- setup ----------

.PHONY: env
env: ## Create .env with all-random secrets (first-time, once)
	./scripts/gen-env.sh
	@echo "(Windows/WSL2) recommended: run 'make doctor' to check the environment first"

.PHONY: doctor
doctor: ## Check environment (docker, compose v2, WSL2 drvfs) — recommended for Windows/Docker Desktop
	./scripts/preflight.sh

.PHONY: runtime-images
runtime-images: ## Build runtime images ล่วงหน้า (ออปชัน — agent เตรียมเองได้ตอนใช้ครั้งแรก)
	docker build -t game-manager/runtime-java:8  --build-arg JAVA_VERSION=8  infra/runtime-java
	docker build -t game-manager/runtime-java:17 --build-arg JAVA_VERSION=17 infra/runtime-java
	docker build -t game-manager/runtime-java:21 --build-arg JAVA_VERSION=21 infra/runtime-java
	docker build -t game-manager/runtime-java:25 --build-arg JAVA_VERSION=25 infra/runtime-java
	# runtime-steam: Dockerfile อยู่ใน game definition ของเกมที่ใช้ SteamCMD (agent embed
	# ไฟล์เดียวกันนี้แล้ว build เองถ้ายังไม่มีบน node) — tag ต้องตรงกับ zomboid/runtime.go
	docker build -t game-manager/runtime-steam:1 \
		-f apps/node-agent/internal/games/zomboid/runtime.Dockerfile \
		apps/node-agent/internal/games/zomboid

.PHONY: agent-image
agent-image: ## Build the official node-agent image (game-manager/node-agent:local) for install-agent.sh
	# context = repo root เพราะ Dockerfile ต้องเห็น packages/proto (ดู apps/node-agent/Dockerfile)
	# label project=game-manager ให้ purge เก็บกวาดได้ (docker image prune --filter label=project=game-manager)
	docker build -t game-manager/node-agent:local \
		--label project=game-manager \
		-f apps/node-agent/Dockerfile .

# ---------- full stack (ทุกอย่างบน docker) ----------

.PHONY: up
up: ## Start the full stack on docker (auto build)
	$(FULL_COMPOSE) up -d --build
	@echo ""
	@echo "Started: http://localhost:$${PANEL_HTTP_PORT:-8000}"
	@echo "See the initial admin password: make admin-password"
	@echo "(Windows/WSL2) if bind mounts look off, try 'make doctor'"

.PHONY: down
down: game-stop ## Stop the full stack + all game instances (data kept)
	$(FULL_COMPOSE) down

# container ของ instance ไม่ได้ถูกจัดการโดย compose (agent สร้างเอง) — compose down จึงไม่แตะ
# ต้องหยุดด้วย label filter ชุดเดียวกับที่ agent ใช้ (ดู runner.LabelManagedBy)
# หยุดอย่างเดียวไม่ลบ: world อยู่ใน bind mount อยู่แล้ว แต่เก็บ container ไว้ให้ start กลับได้เร็ว
# -t 40 = ให้ JVM save world ทัน (agent เองรอ graceful 30 วิ ก่อน SIGTERM)
.PHONY: game-stop
game-stop: ## Stop all game instances on this host (agent-managed containers)
	@ids=$$(docker ps -q --filter "label=gamemanager.managed_by=game-manager-agent"); \
	if [ -n "$$ids" ]; then \
		echo "Stopping game instances: $$(echo $$ids | wc -w | tr -d ' ')"; \
		docker stop -t 40 $$ids > /dev/null; \
	else \
		echo "No running game instances"; \
	fi

.PHONY: logs
logs: ## Tail full stack logs in real-time
	$(FULL_COMPOSE) logs -f

.PHONY: admin-password
admin-password: ## Show initial admin credentials from control-plane logs
	@$(FULL_COMPOSE) logs control-plane 2>/dev/null | grep -A4 "INITIAL ADMIN" || \
		echo "Not found — admin may already have been created (if you forgot the password, use make admin-reset-password)"

.PHONY: admin-reset-password
admin-reset-password: ## Reset the admin (ADMIN_USERNAME) password if forgotten — regenerate + print once
	$(FULL_COMPOSE) exec control-plane /control-plane -reset-admin-password

# ---------- dev infra (postgres/redis/nats) ----------

.PHONY: infra-up
infra-up: ## Start postgres/redis/nats for dev
	$(DEV_COMPOSE) up -d
	@echo "Waiting for postgres to be ready..."
	@until $(DEV_COMPOSE) exec -T postgres pg_isready -U $(POSTGRES_USER) > /dev/null 2>&1; do sleep 1; done
	@echo "infra is ready"

.PHONY: infra-down
infra-down: ## Stop dev infra (data kept)
	$(DEV_COMPOSE) down

.PHONY: infra-logs
infra-logs: ## Tail all dev infra logs in real-time
	$(DEV_COMPOSE) logs -f

# ---------- proto ----------

.PHONY: proto-gen
proto-gen: ## Generate Go + TS from .proto (generated code must always be committed)
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
build: build-control-plane build-agent ## Build everything

# ---------- run (dev นอก docker) ----------

.PHONY: run-control-plane
# TRUSTED_PROXY_COUNT=0 ในรีซีปด้านล่าง: dev รันตรงไม่มี Caddy อยู่หน้า จึงเชื่อ
# RemoteAddr ตรง ๆ (default=1 สำหรับ production หลัง Caddy). ประสานกับ agent INFRA —
# บรรทัด env นี้เป็นของ control-plane อย่าลบ/ย้าย
run-control-plane: ## Run control-plane (dev — connects to dev infra on localhost)
	HTTP_ADDR=:8080 GRPC_ADDR=:9090 \
	DATABASE_URL="$(DB_URL)" \
	REDIS_URL="redis://:$(REDIS_PASSWORD)@localhost:6379/0" \
	NATS_URL="nats://$(NATS_CONTROL_USER):$(NATS_CONTROL_PASSWORD)@localhost:4222" \
	COOKIE_SECURE=false \
	ALLOWED_ORIGINS=http://localhost:3000 \
	TRUSTED_PROXY_COUNT=0 \
	go run ./apps/control-plane/cmd/server

.PHONY: run-agent
run-agent: ## Run node-agent (dev — requires docker on the host)
	AGENT_TOKEN="$(NODE_TOKEN)" \
	CONTROL_PLANE_GRPC=localhost:9090 \
	NATS_URL="nats://$(NATS_AGENT_USER):$(NATS_AGENT_PASSWORD)@localhost:4222" \
	GM_DATA_DIR="$(GM_DATA_DIR)" \
	GM_NETWORK=game-manager-servers \
	go run ./apps/node-agent/cmd/agent

.PHONY: run-web
run-web: ## Run the frontend dev server (proxy /api,/ws -> localhost:8080)
	cd apps/web && pnpm dev

# ---------- dev แบบ container + hot reload (ทางเลือกของ make run-* — คำสั่งเดียวจบ) ----------
# ต่างจาก make run-*: ไม่ต้องเปิด 3 terminal + Go ก็ hot reload (air) ให้ในตัว
# topology เท่า make run-* (browser -> localhost:3000, control-plane :8080, ไม่มี Caddy)

.PHONY: dev
dev: ## Start the whole dev stack in containers with hot reload (control-plane/agent=air, web=next dev)
	@test -f .env || $(MAKE) env
	$(DEV_APP_COMPOSE) up -d --remove-orphans
	@echo ""
	@echo "dev ready (hot reload): http://localhost:3000"
	@echo "First run compiles/installs control-plane/web — wait 1-2 minutes (watch progress with make dev-logs)"
	@echo "Combined logs: make dev-logs   |   Stop: make dev-down"

.PHONY: dev-logs
dev-logs: ## Tail combined dev container logs (air rebuild / next dev in real-time)
	$(DEV_APP_COMPOSE) logs -f

.PHONY: dev-down
dev-down: game-stop ## Stop dev containers + all game instances (infra data kept)
	$(DEV_APP_COMPOSE) down

# ---------- test & lint ----------

.PHONY: test
test: ## Run tests for all services
	go test ./apps/control-plane/... ./apps/node-agent/...
	cd apps/web && pnpm test run 2>/dev/null || true

.PHONY: lint
lint: ## Run lint for all services
	go vet ./apps/control-plane/... ./apps/node-agent/...
	cd apps/web && pnpm lint

# ---------- reset / purge ----------

.PHONY: reset
reset: ## Wipe DB + all infra data and set up fresh (dev)
	@echo "Resetting dev environment..."
	$(DEV_COMPOSE) down -v
	$(DEV_COMPOSE) up -d
	@until $(DEV_COMPOSE) exec -T postgres pg_isready -U $(POSTGRES_USER) > /dev/null 2>&1; do sleep 1; done
	@echo "Reset complete — ready to continue dev (control-plane creates the schema on boot)"

.PHONY: purge
purge: ## Wipe everything (data, volumes, binaries, node_modules) — requires typed confirmation
	@echo "This command will delete:"
	@echo "  - all docker volumes for dev + full stack (DB/NATS data is lost)"
	@echo "  - the bin/ folder and web's node_modules/.next"
	@echo "  - all game containers on this host (data in data/servers is kept)"
	@echo "  - note: generated proto (must be committed) and data/servers are NOT deleted"
	@read -p "Type 'yes' to confirm: " confirm; \
	if [ "$$confirm" != "yes" ]; then \
		echo "Cancelled"; exit 1; \
	fi
	$(DEV_COMPOSE) down -v --remove-orphans || true
	$(FULL_COMPOSE) down -v --remove-orphans || true
	@ids=$$(docker ps -aq --filter "label=gamemanager.managed_by=game-manager-agent"); \
	if [ -n "$$ids" ]; then docker rm -f $$ids > /dev/null; echo "Removed leftover game containers"; fi
	rm -rf bin/
	rm -rf apps/web/node_modules apps/web/.next
	docker image prune -f --filter "label=project=game-manager"
	@echo "purge complete — run 'make bootstrap' to start dev again, or 'make up' for the full stack"

# ---------- bootstrap dev ----------

.PHONY: bootstrap
bootstrap: ## First-time dev setup (env -> infra -> web deps; schema is created by control-plane on boot)
	@test -f .env || $(MAKE) env
	$(MAKE) infra-up
	cd apps/web && pnpm install
	@echo "bootstrap complete — open 3 terminals: make run-control-plane / make run-agent / make run-web"
