"use client";

import { CpuIcon, MemoryStickIcon } from "lucide-react";
import type { Server } from "@/lib/types";
import { formatCpuPercent, formatMb } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { StatusBadge } from "@/components/status-badge";
import { ServerControls } from "@/components/server/server-controls";
import { ServerStatsAccordion } from "@/components/server/server-stats-accordion";
import { useServerNav } from "@/components/server/use-server-nav";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function ServerCard({
  server,
  nodeName,
}: {
  server: Server;
  nodeName?: string;
}) {
  const t = useT();
  const nav = useServerNav(server.id);

  return (
    <Card
      {...nav}
      aria-label={server.name}
      className="focus-visible:border-ring focus-visible:ring-ring/50 cursor-pointer gap-3 py-4 transition-colors outline-none hover:border-ring/40 focus-visible:ring-[3px]"
    >
      <CardHeader className="px-4">
        <CardTitle className="flex items-center justify-between gap-2 text-base">
          <span className="truncate">{server.name}</span>
          <StatusBadge status={server.status} />
        </CardTitle>
      </CardHeader>
      <CardContent className="flex items-end justify-between gap-2 px-4">
        <div className="text-muted-foreground grid min-w-0 gap-0.5 text-xs">
          <span className="capitalize">
            {server.server_type} {server.mc_version}
          </span>
          {server.status === "running" && server.stats ? (
            <span className="flex items-center gap-2">
              <span className="flex items-center gap-1">
                <CpuIcon className="size-3.5" />
                {formatCpuPercent(server.stats.cpu_percent)}
              </span>
              <span className="flex items-center gap-1">
                <MemoryStickIcon className="size-3.5" />
                {formatMb(server.stats.memory_used_mb)} /{" "}
                {formatMb(server.stats.memory_limit_mb)}
              </span>
            </span>
          ) : (
            <span>{t("dashboard.ramShort", { mb: formatMb(server.memory_mb) })}</span>
          )}
          <span className="truncate">
            {nodeName ? t("dashboard.node", { name: nodeName }) : null}
            {server.host_port
              ? ` · ${t("dashboard.port", { port: server.host_port })}`
              : ""}
          </span>
        </div>
        <ServerControls server={server} />
      </CardContent>
      <ServerStatsAccordion server={server} className="-mb-4 px-4" />
    </Card>
  );
}
