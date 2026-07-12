-- +goose Up
CREATE TABLE nodes (
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
    cpu_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    memory_used_mb BIGINT NOT NULL DEFAULT 0,
    memory_total_mb BIGINT NOT NULL DEFAULT 0,
    disk_used_mb BIGINT NOT NULL DEFAULT 0,
    disk_total_mb BIGINT NOT NULL DEFAULT 0,
    last_heartbeat_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE RESTRICT,
    owner_id UUID REFERENCES users(id) ON DELETE SET NULL,
    name VARCHAR(100) NOT NULL,
    server_type VARCHAR(30) NOT NULL
        CHECK (server_type IN ('vanilla', 'paper', 'fabric', 'forge', 'velocity')),
    mc_version VARCHAR(50) NOT NULL,
    memory_mb INT NOT NULL CHECK (memory_mb >= 256),
    -- NULL = ไม่ expose host port (เข้าถึงได้ผ่าน velocity ใน docker network mcpanel-servers)
    host_port INT CHECK (host_port BETWEEN 1024 AND 65535),
    status VARCHAR(20) NOT NULL DEFAULT 'provisioning'
        CHECK (status IN ('provisioning', 'stopped', 'starting', 'running',
                          'stopping', 'errored', 'deleting')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (node_id, host_port)
);

CREATE TABLE server_permissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('owner', 'operator', 'viewer')),
    can_console_write BOOLEAN NOT NULL DEFAULT FALSE,
    can_manage_files BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, server_id)
);

CREATE TABLE jobs (
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

CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    server_id UUID,  -- ไม่มี FK — เก็บประวัติไว้แม้ server ถูกลบ
    action VARCHAR(50) NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}',
    ip VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_servers_node ON servers(node_id);
CREATE INDEX idx_servers_owner ON servers(owner_id);
CREATE INDEX idx_perms_user_server ON server_permissions(user_id, server_id);
CREATE INDEX idx_perms_server ON server_permissions(server_id);
CREATE INDEX idx_jobs_server_status ON jobs(server_id, status);
CREATE INDEX idx_jobs_created ON jobs(created_at DESC);
CREATE INDEX idx_audit_server_time ON audit_logs(server_id, created_at DESC);
CREATE INDEX idx_audit_user_time ON audit_logs(user_id, created_at DESC);

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE jobs;
DROP TABLE server_permissions;
DROP TABLE servers;
DROP TABLE nodes;
