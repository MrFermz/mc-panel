"use client";

import * as React from "react";
import dynamic from "next/dynamic";
import { ActivityIcon } from "lucide-react";
import type { Server } from "@/lib/types";
import {
  useStatsHistoryStore,
  type StatPoint,
} from "@/lib/settings/stats-history";
import { useT } from "@/lib/i18n";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

// recharts ResponsiveContainer แตะ DOM ตอน measure — ปิด SSR กัน hydration mismatch
const StatsChart = dynamic(
  () => import("@/components/server/stats-chart").then((m) => m.StatsChart),
  {
    ssr: false,
    // height ให้ใกล้ StatsChart จริง (สี่กราฟ height=80: CPU/RAM/Network/Disk) เพื่อไม่ให้กระตุกตอนโหลด
    loading: () => <Skeleton className="h-[440px] w-full" />,
  },
);

const EMPTY_HISTORY: StatPoint[] = [];

// กัน event ทะลุไป parent (การ์ด/แถวที่กดแล้ว navigate เข้า detail)
function stop(e: React.SyntheticEvent) {
  e.stopPropagation();
}

export function ServerStatsAccordion({
  server,
  className,
}: {
  server: Server;
  className?: string;
}) {
  const t = useT();
  const history = useStatsHistoryStore(
    (s) => s.history[server.id] ?? EMPTY_HISTORY,
  );

  // แสดง accordion เฉพาะ server ที่รันอยู่และมี stats
  if (server.status !== "running" || !server.stats) return null;

  return (
    <Accordion type="single" collapsible className={cn("border-t", className)}>
      <AccordionItem value="resources" className="border-b-0">
        <AccordionTrigger
          className="text-muted-foreground hover:text-foreground px-0"
          onClick={stop}
          onPointerDown={stop}
          onKeyDown={stop}
        >
          <span className="flex items-center gap-1.5">
            <ActivityIcon className="size-3.5" />
            {t("stats.resources")}
          </span>
        </AccordionTrigger>
        <AccordionContent onClick={stop} onPointerDown={stop}>
          <StatsChart stats={server.stats} history={history} height={80} />
        </AccordionContent>
      </AccordionItem>
    </Accordion>
  );
}
