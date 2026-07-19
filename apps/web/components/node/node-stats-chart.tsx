"use client";

import * as React from "react";
import dynamic from "next/dynamic";
import type { Node, ServerStats } from "@/lib/types";
import {
  useStatsHistoryStore,
  type StatPoint,
} from "@/lib/settings/stats-history";
import { Skeleton } from "@/components/ui/skeleton";

// recharts ResponsiveContainer แตะ DOM ตอน measure — ปิด SSR กัน hydration mismatch (เหมือน server-stats-accordion)
const StatsChart = dynamic(
  () => import("@/components/server/stats-chart").then((m) => m.StatsChart),
  {
    ssr: false,
    // สี่กราฟ height=100 (CPU/RAM/Network/Disk)
    loading: () => <Skeleton className="h-[520px] w-full" />,
  },
);

const EMPTY_HISTORY: StatPoint[] = [];

// รวม node metrics ให้เข้ารูป ServerStats เพื่อ reuse StatsChart ตัวเดียวกับ instance
// (node ไม่มี "limit" ต่อ process — ใช้ total ของเครื่องเป็นเพดาน)
function nodeStats(node: Node): ServerStats {
  // net/disk baseline ปล่อย 0 — StatsChart อ่านค่าจริงจากจุดล่าสุดใน history (StatPoint ของ node)
  return {
    cpu_percent: node.cpu_percent,
    memory_used_mb: node.memory_used_mb,
    memory_limit_mb: node.memory_total_mb,
    net_rx_bps: 0,
    net_tx_bps: 0,
    disk_read_bps: 0,
    disk_write_bps: 0,
    // uptime เป็นของ container ต่อ instance — node ไม่มีค่านี้
    started_at: null,
    // ผู้เล่น/TPS เป็นของ instance (อ่านจาก console ของ server) — node ไม่มีค่านี้
    online_players: [],
    max_players: 0,
    tps: 0,
    updated_at: node.last_heartbeat_at ?? "",
  };
}

export function NodeStatsChart({ node }: { node: Node }) {
  const history = useStatsHistoryStore(
    (s) => s.history[node.id] ?? EMPTY_HISTORY,
  );
  return (
    <StatsChart
      stats={nodeStats(node)}
      history={history}
      height={100}
      variant="node"
    />
  );
}
