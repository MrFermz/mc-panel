# node-agent — build ด้วย context = repo root (ดู infra/docker-compose.yml)
# เพราะต้องเห็น packages/proto ผ่าน replace directive ใน go.mod

FROM golang:1.24-alpine AS build

WORKDIR /src

# ใน image ไม่มี go.work ของ monorepo — ปิด workspace mode ให้ resolve
# ผ่าน replace ../../packages/proto ใน go.mod แทน
ENV GOWORK=off CGO_ENABLED=0

COPY packages/proto/ packages/proto/
COPY apps/node-agent/ apps/node-agent/

WORKDIR /src/apps/node-agent
RUN go mod download
RUN go build -trimpath -ldflags="-s -w" -o /out/node-agent ./cmd/agent

# distroless static ไม่มี shell/package manager — ลด attack surface ของ container
# ที่ถือ docker.sock ซึ่งเป็นจุดอันตรายที่สุดของระบบ
# รันเป็น root โดยจำเป็น: ต้องเขียน /var/run/docker.sock และ chown ไฟล์ให้ uid 1000
FROM gcr.io/distroless/static-debian12

COPY --from=build /out/node-agent /node-agent

ENTRYPOINT ["/node-agent"]
