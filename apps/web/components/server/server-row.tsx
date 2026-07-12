"use client";

import { CpuIcon, MemoryStickIcon } from "lucide-react";
import type { Server } from "@/lib/types";
import { formatCpuPercent, formatMb } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { StatusBadge } from "@/components/status-badge";
import { ServerControls } from "@/components/server/server-controls";
import { ServerStatsAccordion } from "@/components/server/server-stats-accordion";
import { useServerNav } from "@/components/server/use-server-nav";

export function ServerRow({
  server,
  nodeName,
}: {
  server: Server;
  nodeName?: string;
}) {
  const t = useT();
  const nav = useServerNav(server.id);

  return (
    <div
      {...nav}
      aria-label={server.name}
      className={cn(
        "bg-card cursor-pointer rounded-lg border px-4 outline-none transition-colors",
        "hover:border-ring/40 focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]",
      )}
    >
      <div className="flex items-center gap-3 py-3">
        <div className="grid min-w-0 flex-1 gap-0.5">
          <div className="flex items-center gap-2">
            <span className="truncate font-medium">{server.name}</span>
            <StatusBadge status={server.status} />
          </div>
          <span className="text-muted-foreground truncate text-xs capitalize">
            {server.server_type} {server.mc_version}
            {nodeName ? (
              <span className="normal-case">
                {" · "}
                {t("dashboard.node", { name: nodeName })}
              </span>
            ) : null}
            {server.host_port ? (
              <span className="normal-case">
                {" · "}
                {t("dashboard.port", { port: server.host_port })}
              </span>
            ) : null}
          </span>
        </div>

        {server.status === "running" && server.stats ? (
          <div className="text-muted-foreground hidden shrink-0 items-center gap-3 text-xs sm:flex">
            <span className="flex items-center gap-1">
              <CpuIcon className="size-3.5" />
              {formatCpuPercent(server.stats.cpu_percent)}
            </span>
            <span className="flex items-center gap-1">
              <MemoryStickIcon className="size-3.5" />
              {formatMb(server.stats.memory_used_mb)} /{" "}
              {formatMb(server.stats.memory_limit_mb)}
            </span>
          </div>
        ) : null}

        <ServerControls server={server} />
      </div>

      <ServerStatsAccordion server={server} />
    </div>
  );
}
