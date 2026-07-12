-- +goose Up
-- network telemetry ต่อ node (เก็บใน nodes row เหมือน cpu/mem/disk) — agent ส่งมาใน heartbeat
ALTER TABLE nodes ADD COLUMN net_rx_bps DOUBLE PRECISION NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN net_tx_bps DOUBLE PRECISION NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE nodes DROP COLUMN net_rx_bps;
ALTER TABLE nodes DROP COLUMN net_tx_bps;
