# control-plane — build context คือ repo root (ดู infra/docker-compose.yml)
# ต้อง COPY packages/proto ด้วยเพราะ go.mod มี replace ../../packages/proto

FROM golang:1.24-alpine AS build
WORKDIR /src

COPY packages/proto/ packages/proto/
COPY apps/control-plane/ apps/control-plane/

WORKDIR /src/apps/control-plane
# GOWORK=off เพราะ go.work ที่ root ไม่ได้ถูก copy เข้ามา (อ้าง module อื่นที่ไม่เกี่ยว)
ENV GOWORK=off CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /out/control-plane ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/control-plane /control-plane
ENTRYPOINT ["/control-plane"]
