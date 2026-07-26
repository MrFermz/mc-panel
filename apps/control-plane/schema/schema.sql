-- Schema ปัจจุบันของทั้งระบบ — control-plane รันไฟล์นี้ตอน boot ทุกครั้ง
--
-- ยังไม่มีระบบ migration (ถอด goose ออกไปแล้ว จะกลับมาทำใหม่ทีหลัง) — ตอนนี้ไฟล์นี้คือ
-- source of truth เดียวของ schema จึงต้อง **idempotent เสมอ** (IF NOT EXISTS ทุกคำสั่ง)
-- boot ซ้ำต้องไม่พัง แต่ก็ **ไม่มีการ migrate ข้อมูลเดิม**: แก้คอลัมน์/constraint ของตาราง
-- ที่มีอยู่แล้วจะไม่มีผลกับ DB เก่า ต้อง `make reset` (dev) หรือลบ volume ทิ้งเอง

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Auth เป็น local อย่างเดียวใน scope ปัจจุบัน (OIDC ค่อยว่ากันทีหลัง)
-- ไม่มี email ทั้งระบบ: username เป็น login identifier เดียว (ไม่มีอะไรส่งเมล จึงไม่เก็บ PII ฟรี ๆ)
-- flow ผู้ใช้ใหม่ทุกคน: ระบบเจน password มั่ว ๆ ให้ -> must_change_password = TRUE
-- -> login ครั้งแรกโดนบังคับตั้ง password ใหม่ก่อนใช้งานอย่างอื่น
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- canonical เป็น lowercase ตั้งแต่ชั้น DB — โค้ดที่ลืม normalize จะโดน CHECK ตัวนี้เป็นด่านสุดท้าย
    username VARCHAR(64) NOT NULL CONSTRAINT users_username_lowercase CHECK (username = lower(username)),
    password_hash TEXT NOT NULL,
    -- ชื่อที่เจ้าของบัญชีตั้งเอง ว่าง = ตกไปใช้ username
    display_name VARCHAR(64) NOT NULL DEFAULT '',
    is_admin BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    must_change_password BOOLEAN NOT NULL DEFAULT TRUE,
    -- bump ทุกครั้งที่เปลี่ยน password / โดน reset -> JWT เก่าทุกใบใช้ไม่ได้ทันที
    token_version INT NOT NULL DEFAULT 1,
    -- Global capability ต่อ user (คนละชั้นกับ server_permissions ที่เป็นสิทธิ์ต่อ server)
    -- key รูป `{feature}.{action}` — catalog อยู่ใน internal/httpapi/capabilities.go
    -- is_admin = superuser ครอบทุก capability โดยปริยาย (ไม่ต้องใส่ในคอลัมน์นี้)
    capabilities TEXT[] NOT NULL DEFAULT '{}',
    -- เก็บรูป avatar เป็น bytes ในแถวเลย (ระบบยังไม่มี object storage และ handler จำกัดขนาดไว้แล้ว)
    -- คอลัมน์ avatar ไม่เคยอยู่ใน SELECT ปกติ อ่านเฉพาะตอน serve /api/users/{id}/avatar
    -- avatar_updated_at ใช้ทั้ง cache-buster ของ URL และ ETag
    avatar BYTEA,
    avatar_mime VARCHAR(64) NOT NULL DEFAULT '',
    avatar_updated_at TIMESTAMPTZ,
    last_login_at TIMESTAMPTZ,
    -- ลบ user = soft delete เสมอ: เก็บแถวไว้เพื่อรักษา FK/audit history + กู้คืนได้ทั้ง
    -- สิทธิ์ต่อ server (server_permissions ไม่ถูกแตะตอนลบ)
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- unique "ทั้งตาราง" (ไม่ใช่ partial WHERE deleted_at IS NULL) โดยตั้งใจ — ชื่อของบัญชีใน
-- ถังขยะถูกจองไว้ตลอด ลบแล้ว restore ได้เสมอ แลกกับที่ชื่อนั้นเอาไปสร้างใหม่ไม่ได้จนกว่าจะกู้คืน
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_lower ON users (lower(username));

CREATE TABLE IF NOT EXISTS nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    -- เก็บ SHA-256 hex ของ token ทั้งเส้น (token เป็น opaque random string)
    -- auth: hash token ที่ agent ส่งมา แล้ว lookup แถวด้วย hash ตรง ๆ
    agent_token_hash TEXT NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'offline'
        CHECK (status IN ('offline', 'online')),
    agent_version VARCHAR(50) NOT NULL DEFAULT '',
    os VARCHAR(20) NOT NULL DEFAULT '',
    arch VARCHAR(20) NOT NULL DEFAULT '',
    -- telemetry จาก heartbeat
    cpu_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_used_mb BIGINT NOT NULL DEFAULT 0,
    memory_total_mb BIGINT NOT NULL DEFAULT 0,
    disk_used_mb BIGINT NOT NULL DEFAULT 0,
    disk_total_mb BIGINT NOT NULL DEFAULT 0,
    net_rx_bps DOUBLE PRECISION NOT NULL DEFAULT 0,
    net_tx_bps DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_heartbeat_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE RESTRICT,
    owner_id UUID REFERENCES users(id) ON DELETE SET NULL,
    name VARCHAR(100) NOT NULL,
    -- server ทุกตัวผูกกับ "เกม" หนึ่งเกม แล้ว variant คือชนิดของ server ภายในเกมนั้น
    -- **ไม่มี CHECK รายชื่อทั้งสองคอลัมน์โดยตั้งใจ** — รายการเกม/variant ที่ถูกต้องเป็นความรู้
    -- ของ game definition (internal/games) ไม่ใช่ของ schema
    game TEXT NOT NULL,
    variant VARCHAR(30) NOT NULL,
    game_version VARCHAR(50) NOT NULL,
    -- floor จริงของ memory เป็นของแต่ละเกม (Definition.MinMemoryMB) — schema กันแค่ค่าที่ไร้ความหมาย
    memory_mb INT NOT NULL CHECK (memory_mb > 0),
    -- NULL = ไม่ expose host port (เข้าถึงได้ผ่าน proxy instance ใน docker network game-manager-servers)
    host_port INT CHECK (host_port BETWEEN 1024 AND 65535),
    status VARCHAR(20) NOT NULL DEFAULT 'provisioning'
        CHECK (status IN ('provisioning', 'stopped', 'starting', 'running',
                          'stopping', 'errored', 'deleting')),
    -- Soft delete: DELETE /api/servers/{id} แค่ mark deleted_at (ไฟล์ใน GM_DATA_DIR ยังอยู่ครบ
    -- → restore ได้ทันที) ส่วนการลบจริงคือ POST /api/servers/{id}/purge ที่ยิง job delete_server
    --
    -- แถวที่ถูก soft delete ยังนับใน UNIQUE (node_id, host_port) และใน SumServerMemoryMBOnNode
    -- โดยตั้งใจ — ทรัพยากรยังถูกจองไว้จริงบน node จน purge เพื่อให้ restore สำเร็จเสมอ
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (node_id, host_port)
);

-- สิทธิ์ต่อ server: effective = global capability AND grant ในตารางนี้
--   owner  = ได้ทุก server-scoped cap โดยปริยาย + จัดการ access list (มี ≥1 เสมอ)
--   member = ถือ capabilities[] (subset ของ catalog ที่มีความหมายต่อ server ตัวหนึ่ง)
CREATE TABLE IF NOT EXISTS server_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL
        CONSTRAINT server_permissions_role_check CHECK (role IN ('owner', 'member')),
    capabilities TEXT[] NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, server_id)
);

CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID REFERENCES servers(id) ON DELETE SET NULL,
    node_id UUID REFERENCES nodes(id) ON DELETE SET NULL,
    type VARCHAR(30) NOT NULL
        CHECK (type IN ('create_server', 'start_server', 'stop_server',
                        'kill_server', 'delete_server')),
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    -- protojson ของ JobEnvelope เก็บไว้เพื่อ debug/replay
    payload JSONB NOT NULL DEFAULT '{}',
    error TEXT NOT NULL DEFAULT '',
    requested_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    server_id UUID,  -- ไม่มี FK — เก็บประวัติไว้แม้ server ถูกลบ
    action VARCHAR(50) NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}',
    ip VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- allowlist ต่อ server — mirror ของไฟล์รายชื่อที่ agent เขียนลง disk
-- (source of truth คือ DB, ไฟล์ rebuild จากตารางนี้ทุกครั้งที่ add/remove)
-- uuid มาจาก identity service ของเกม; username เก็บไว้แสดงผล + เขียนลงไฟล์ allowlist
-- (ชื่อไฟล์/รูปแบบเป็นของ game definition — Minecraft = whitelist.json)
CREATE TABLE IF NOT EXISTS server_players (
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    uuid UUID NOT NULL,
    username VARCHAR(16) NOT NULL,
    added_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (server_id, uuid)
);

-- cache รูปประจำตัวผู้เล่นต่อ uuid เก็บเป็น bytes ในแถวเลย เหมือน user avatar
-- ตารางนี้ไม่รู้จักเกม — "วิธีได้รูปมา" อยู่ใน game definition ส่วนชั้น cache เป็นของกลาง
-- ที่ internal/avatarcache. key ด้วย uuid ของผู้เล่นอย่างเดียว (รูปเป็น global ต่อ account
-- ไม่ผูก server) → แชร์ cache ข้ามทุก server ได้
--
-- เก็บลง storage แทน in-memory เพื่อให้ยังเสิร์ฟรูปเก่าได้ตอน upstream ของเกมติดต่อไม่ได้
-- (graceful degradation) และรอดข้าม restart
--
-- png = NULL คือ negative cache (uuid นี้ไม่มีรูป เช่น offline-mode / ไม่มี texture)
-- fetched_at ใช้ตัดสิน staleness (refresh เมื่อรูปเปลี่ยน) — อ่านค่าเมื่อ TTL หมด
CREATE TABLE IF NOT EXISTS player_avatars (
    uuid       UUID PRIMARY KEY,
    png        BYTEA,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_servers_node ON servers(node_id);
CREATE INDEX IF NOT EXISTS idx_servers_owner ON servers(owner_id);
-- query ปกติทั้งหมด filter deleted_at IS NULL — partial index ครอบ row ที่ยัง active เท่านั้น
CREATE INDEX IF NOT EXISTS idx_servers_active ON servers (created_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_perms_user_server ON server_permissions(user_id, server_id);
CREATE INDEX IF NOT EXISTS idx_perms_server ON server_permissions(server_id);
CREATE INDEX IF NOT EXISTS idx_jobs_server_status ON jobs(server_id, status);
CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_server_time ON audit_logs(server_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_user_time ON audit_logs(user_id, created_at DESC);
