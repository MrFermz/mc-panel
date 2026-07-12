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
    loading: () => <Skeleton className="h-[220px] w-full" />,
  },
);

const EMPTY_HISTORY: StatPoint[] = [];

// รวม node metrics ให้เข้ารูป ServerStats เพื่อ reuse StatsChart ตัวเดียวกับ instance
// (node ไม่มี "limit" ต่อ process — ใช้ total ของเครื่องเป็นเพดาน)
function nodeStats(node: Node): ServerStats {
  return {
    cpu_percent: node.cpu_percent,
    memory_used_mb: node.memory_used_mb,
    memory_limit_mb: node.memory_total_mb,
    updated_at: node.last_heartbeat_at ?? "",
  };
}

export function NodeStatsChart({ node }: { node: Node }) {
  const history = useStatsHistoryStore(
    (s) => s.history[node.id] ?? EMPTY_HISTORY,
  );
  return <StatsChart stats={nodeStats(node)} history={history} height={100} />;
}
