"use client";

import * as React from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  CpuIcon,
  DownloadIcon,
  HardDriveIcon,
  LayoutGridIcon,
  ListIcon,
  MemoryStickIcon,
  PlusIcon,
} from "lucide-react";
import { apiGet, ApiError } from "@/lib/api";
import {
  metaNodesResponseSchema,
  nodesResponseSchema,
  serversResponseSchema,
  type Node,
} from "@/lib/types";
import { formatMb } from "@/lib/format";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { useSettingsStore, type ServerView } from "@/lib/settings/store";
import { useStatsHistoryStore } from "@/lib/settings/stats-history";
import { ServerCard } from "@/components/server/server-card";
import { ServerRow } from "@/components/server/server-row";
import { NodeStatsAccordion } from "@/components/node/node-stats-accordion";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

const PAGE_SIZE: Record<ServerView, number> = { grid: 12, list: 10 };

function NodeSummary({ node }: { node: Node }) {
  const t = useT();
  const online = node.status === "online";
  return (
    <Card className="gap-3 py-4">
      <CardHeader className="px-4">
        <CardTitle className="flex items-center justify-between gap-2 text-sm">
          <span className="truncate">{node.name}</span>
          <Badge
            variant="outline"
            className={cn(
              online
                ? "bg-green-500/15 text-green-400 border-green-500/30"
                : "bg-red-500/15 text-red-400 border-red-500/30",
            )}
          >
            <span
              className={cn(
                "size-1.5 rounded-full",
                online ? "bg-green-400" : "bg-red-400",
              )}
            />
            {t(online ? "nodeStatus.online" : "nodeStatus.offline")}
          </Badge>
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-3 px-4">
        <div className="text-muted-foreground grid gap-1 text-xs">
          <div className="flex items-center gap-1.5">
            <CpuIcon className="size-3.5" />
            {t("res.cpu")} {node.cpu_percent.toFixed(1)}%
          </div>
          <div className="flex items-center gap-1.5">
            <MemoryStickIcon className="size-3.5" />
            {t("res.ram")} {formatMb(node.memory_used_mb)} /{" "}
            {formatMb(node.memory_total_mb)}
          </div>
          <div className="flex items-center gap-1.5">
            <HardDriveIcon className="size-3.5" />
            {t("res.disk")} {formatMb(node.disk_used_mb)} /{" "}
            {formatMb(node.disk_total_mb)}
          </div>
        </div>
      </CardContent>
      <NodeStatsAccordion node={node} className="-mb-4 px-4" />
    </Card>
  );
}

function ViewToggle({
  view,
  onChange,
}: {
  view: ServerView;
  onChange: (v: ServerView) => void;
}) {
  const t = useT();
  const base =
    "flex size-7 items-center justify-center rounded-sm transition-colors [&_svg]:size-4";
  return (
    <div className="bg-muted flex items-center gap-0.5 rounded-md p-0.5">
      <button
        type="button"
        aria-label={t("view.grid")}
        aria-pressed={view === "grid"}
        onClick={() => onChange("grid")}
        className={cn(
          base,
          view === "grid"
            ? "bg-background text-foreground shadow-xs"
            : "text-muted-foreground hover:text-foreground",
        )}
      >
        <LayoutGridIcon />
      </button>
      <button
        type="button"
        aria-label={t("view.list")}
        aria-pressed={view === "list"}
        onClick={() => onChange("list")}
        className={cn(
          base,
          view === "list"
            ? "bg-background text-foreground shadow-xs"
            : "text-muted-foreground hover:text-foreground",
        )}
      >
        <ListIcon />
      </button>
    </div>
  );
}

export default function DashboardPage() {
  const t = useT();
  const { data: meData } = useMe();
  const isAdmin = meData?.user.is_admin ?? false;
  const canCreateServer = hasCapability(meData?.user, CAPABILITY.serversCreate);

  const view = useSettingsStore((s) => s.serverView);
  const setView = useSettingsStore((s) => s.setServerView);

  const pushStats = useStatsHistoryStore((s) => s.push);
  const resetStats = useStatsHistoryStore((s) => s.reset);

  const [page, setPage] = React.useState(1);

  // ไม่ poll แล้ว — stats/status อัปเดตผ่าน WS /ws/events (useEvents ที่ layout)
  const servers = useQuery({
    queryKey: ["servers"],
    queryFn: () => apiGet("/api/servers", serversResponseSchema),
  });

  const metaNodes = useQuery({
    queryKey: ["meta", "nodes"],
    queryFn: () => apiGet("/api/meta/nodes", metaNodesResponseSchema),
    staleTime: 60_000,
  });

  const nodes = useQuery({
    queryKey: ["nodes"],
    queryFn: () => apiGet("/api/nodes", nodesResponseSchema),
    enabled: isAdmin,
  });

  const nodeNames = React.useMemo(() => {
    const map = new Map<string, string>();
    for (const n of metaNodes.data?.nodes ?? []) map.set(n.id, n.name);
    return map;
  }, [metaNodes.data]);

  const serverList = servers.data?.servers;

  // ป้อน history stats ของทุก server จากระดับ dashboard (refetch ทุก 5s)
  // popover จึงมีข้อมูลย้อนหลังเสมอโดยไม่ต้องเปิดค้าง
  React.useEffect(() => {
    if (!serverList) return;
    for (const s of serverList) {
      if (s.status === "running" && s.stats) {
        pushStats(s.id, {
          t: new Date(s.stats.updated_at).getTime() || Date.now(),
          cpu: s.stats.cpu_percent,
          memUsed: s.stats.memory_used_mb,
          memLimit: s.stats.memory_limit_mb,
          netRx: s.stats.net_rx_bps,
          netTx: s.stats.net_tx_bps,
          diskR: s.stats.disk_read_bps,
          diskW: s.stats.disk_write_bps,
        });
      } else {
        resetStats(s.id);
      }
    }
  }, [serverList, pushStats, resetStats]);

  const pageSize = PAGE_SIZE[view];
  const total = serverList?.length ?? 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = Math.min(page, pageCount);
  const pageItems = React.useMemo(
    () =>
      (serverList ?? []).slice(
        (currentPage - 1) * pageSize,
        currentPage * pageSize,
      ),
    [serverList, currentPage, pageSize],
  );

  // สลับ view หรือรายการหดสั้นลง → กลับไปหน้า 1
  const changeView = (v: ServerView) => {
    setView(v);
    setPage(1);
  };
  React.useEffect(() => {
    if (page > pageCount) setPage(pageCount);
  }, [page, pageCount]);

  return (
    <div className="grid gap-6">
      {isAdmin && nodes.data && nodes.data.nodes.length > 0 && (
        <section className="grid gap-3">
          <h2 className="text-muted-foreground text-sm font-semibold tracking-wider uppercase">
            {t("dashboard.nodes")}
          </h2>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            {nodes.data.nodes.map((node) => (
              <NodeSummary key={node.id} node={node} />
            ))}
          </div>
        </section>
      )}

      <section className="grid gap-3">
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-muted-foreground text-sm font-semibold tracking-wider uppercase">
            {t("dashboard.servers")}
          </h2>
          <div className="flex items-center gap-2">
            <ViewToggle view={view} onChange={changeView} />
            {canCreateServer && (
              <>
                <Button size="sm" variant="outline" asChild>
                  <Link href="/servers/new?mode=import">
                    <DownloadIcon />
                    {t("import.button")}
                  </Link>
                </Button>
                <Button size="sm" asChild>
                  <Link href="/servers/new">
                    <PlusIcon />
                    {t("nav.newServer")}
                  </Link>
                </Button>
              </>
            )}
          </div>
        </div>

        {servers.isPending ? (
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
            <Skeleton className="h-32" />
          </div>
        ) : servers.isError ? (
          <p className="text-destructive text-sm">
            {t("dashboard.failedServers")}
            {servers.error instanceof ApiError
              ? `: ${servers.error.message}`
              : "."}
          </p>
        ) : total === 0 ? (
          <Card className="py-10">
            <CardContent className="text-muted-foreground flex flex-col items-center gap-3 text-sm">
              <p>{t("dashboard.noServers")}</p>
              {canCreateServer && (
                <Button variant="outline" asChild>
                  <Link href="/servers/new">{t("dashboard.createFirst")}</Link>
                </Button>
              )}
            </CardContent>
          </Card>
        ) : (
          <>
            {view === "grid" ? (
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {pageItems.map((server) => (
                  <ServerCard
                    key={server.id}
                    server={server}
                    nodeName={nodeNames.get(server.node_id)}
                  />
                ))}
              </div>
            ) : (
              <div className="grid gap-2">
                {pageItems.map((server) => (
                  <ServerRow
                    key={server.id}
                    server={server}
                    nodeName={nodeNames.get(server.node_id)}
                  />
                ))}
              </div>
            )}

            {pageCount > 1 && (
              <div className="flex items-center justify-center gap-3 pt-1">
                <Button
                  size="sm"
                  variant="outline"
                  disabled={currentPage <= 1}
                  onClick={() => setPage((p) => Math.max(1, p - 1))}
                >
                  {t("page.prev")}
                </Button>
                <span className="text-muted-foreground text-sm">
                  {t("page.indicator", {
                    current: currentPage,
                    total: pageCount,
                  })}
                </span>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={currentPage >= pageCount}
                  onClick={() => setPage((p) => Math.min(pageCount, p + 1))}
                >
                  {t("page.next")}
                </Button>
              </div>
            )}
          </>
        )}
      </section>
    </div>
  );
}
