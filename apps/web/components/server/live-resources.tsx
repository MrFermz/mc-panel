"use client";

import * as React from "react";
import dynamic from "next/dynamic";
import { ActivityIcon } from "lucide-react";
import type { Server } from "@/lib/types";
import { useT } from "@/lib/i18n";
import {
  useStatsHistoryStore,
  type StatPoint,
} from "@/lib/settings/stats-history";
import { useSettingsStore } from "@/lib/settings/store";
import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";

// recharts ResponsiveContainer measure DOM — ปิด SSR กัน hydration mismatch
const StatsChart = dynamic(
  () => import("@/components/server/stats-chart").then((m) => m.StatsChart),
  { ssr: false, loading: () => <Skeleton className="h-[140px] w-full" /> },
);

const EMPTY_HISTORY: StatPoint[] = [];

// การ์ด live resources (CPU/RAM/net/disk) ของ server หนึ่งตัว — เก็บ history ฝั่ง client เอง
// ให้ใช้ซ้ำได้ทุกหน้า (ตอนนี้อยู่ล่างสุดของหน้า console); ซ่อนเมื่อ server ไม่ได้รัน
export function LiveResources({ server }: { server: Server }) {
  const t = useT();
  const id = server.id;
  const stats = server.stats;
  const isRunning = server.status === "running";

  const history = useStatsHistoryStore((s) => s.history[id] ?? EMPTY_HISTORY);
  const pushStats = useStatsHistoryStore((s) => s.push);
  const resetStats = useStatsHistoryStore((s) => s.reset);

  // การ์ดเปิด/ปิด — จำค่าไว้ข้าม visit (persist ใน settings store)
  const resourcesOpen = useSettingsStore((s) => s.detailResourcesOpen);
  const setResourcesOpen = useSettingsStore((s) => s.setDetailResourcesOpen);

  // เก็บ history ทุกครั้งที่ค่าใหม่เข้ามา (stats push ผ่าน WS)
  React.useEffect(() => {
    if (!isRunning || !stats) return;
    pushStats(id, {
      t: new Date(stats.updated_at).getTime() || Date.now(),
      cpu: stats.cpu_percent,
      memUsed: stats.memory_used_mb,
      memLimit: stats.memory_limit_mb,
      netRx: stats.net_rx_bps,
      netTx: stats.net_tx_bps,
      diskR: stats.disk_read_bps,
      diskW: stats.disk_write_bps,
    });
  }, [id, isRunning, stats, pushStats]);

  // reset history เมื่อ server ไม่ได้รันแล้ว (กราฟเริ่มใหม่รอบหน้า)
  React.useEffect(() => {
    if (!isRunning) resetStats(id);
  }, [id, isRunning, resetStats]);

  if (!isRunning || !stats) return null;

  return (
    <Card className="px-4 py-0">
      <Accordion
        type="single"
        collapsible
        value={resourcesOpen ? "resources" : ""}
        onValueChange={(v) => setResourcesOpen(v === "resources")}
      >
        <AccordionItem value="resources" className="border-b-0">
          <AccordionTrigger className="text-muted-foreground hover:text-foreground text-xs font-semibold tracking-wider uppercase">
            <span className="flex items-center gap-1.5">
              <ActivityIcon className="size-3.5" />
              {t("stats.title")}
            </span>
          </AccordionTrigger>
          <AccordionContent>
            <StatsChart stats={stats} history={history} height={140} />
          </AccordionContent>
        </AccordionItem>
      </Accordion>
    </Card>
  );
}
