"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ActivityIcon,
  ClockIcon,
  MemoryStickIcon,
  UsersIcon,
} from "lucide-react";
import { apiGet, ApiError } from "@/lib/api";
import { serversResponseSchema, type Server } from "@/lib/types";
import { formatMb, formatUptime } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { useSettingsStore } from "@/lib/settings/store";
import { useActiveServer } from "@/lib/use-active-server";
import { useSetPageServer } from "@/components/layout/breadcrumb-context";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

// การ์ดสถิติ 1 ค่า — icon + label ด้านบน, ค่าตัวใหญ่, บรรทัดย่อยเสริมบริบท (หรือป้าย coming soon)
function StatCard({
  icon: Icon,
  label,
  value,
  children,
}: {
  icon: React.ComponentType<{ className?: string }>;
  label: string;
  value: React.ReactNode;
  children?: React.ReactNode;
}) {
  return (
    <Card className="gap-2 py-4">
      <CardHeader className="px-4">
        <CardTitle className="text-muted-foreground flex items-center gap-1.5 text-xs font-semibold tracking-wider uppercase">
          <Icon className="size-3.5" />
          {label}
        </CardTitle>
      </CardHeader>
      <CardContent className="grid gap-2 px-4">
        <div className="text-2xl font-semibold tracking-tight">{value}</div>
        {children}
      </CardContent>
    </Card>
  );
}

// uptime เดินเองทุกวินาที (stats push มาทุก ~5s แต่ตัวเลขควรเดินต่อเนื่อง)
function UptimeValue({ startedAt }: { startedAt: string | null }) {
  const [now, setNow] = React.useState(() => Date.now());
  React.useEffect(() => {
    if (!startedAt) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [startedAt]);

  if (!startedAt) return <>—</>;
  const startMs = new Date(startedAt).getTime();
  if (Number.isNaN(startMs)) return <>—</>;
  return <>{formatUptime((now - startMs) / 1000)}</>;
}

function ServerOverview({ server }: { server: Server }) {
  const t = useT();
  const running = server.status === "running";
  const stats = server.stats;

  const memUsed = running && stats ? stats.memory_used_mb : 0;
  const memTotal = stats?.memory_limit_mb ?? server.memory_mb;
  const memPct = memTotal > 0 ? Math.min(100, (memUsed / memTotal) * 100) : 0;
  const memBar =
    memPct >= 90 ? "bg-red-500" : memPct >= 70 ? "bg-amber-500" : "bg-primary";

  const online = running && stats ? stats.online_players : [];
  const maxPlayers = stats?.max_players ?? 0;
  const tps = stats?.tps ?? 0;

  return (
    <div className="grid gap-4">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {/* ผู้เล่นออนไลน์ — agent อ่านจาก console (`list` + join/left) ส่งมากับ stats */}
        <StatCard
          icon={UsersIcon}
          label={t("overview.playersOnline")}
          value={
            running && stats ? (
              <span>
                {online.length}
                {maxPlayers > 0 && (
                  <span className="text-muted-foreground text-base font-normal">
                    {" / "}
                    {maxPlayers}
                  </span>
                )}
              </span>
            ) : (
              "—"
            )
          }
        />
        {/* TPS มีเฉพาะ paper/spigot — server type อื่นไม่มีคำสั่ง `tps` (stats.tps = 0) */}
        <StatCard
          icon={ActivityIcon}
          label={t("overview.tps")}
          value={running && tps > 0 ? tps.toFixed(2) : "—"}
        >
          {running && tps === 0 && (
            <span className="text-muted-foreground text-xs">
              {t("overview.tpsUnsupported")}
            </span>
          )}
        </StatCard>
        {/* memory เป็นค่าจริงจาก container stats (null เมื่อไม่ได้รัน → 0 / limit) */}
        <StatCard
          icon={MemoryStickIcon}
          label={t("overview.memory")}
          value={
            <span>
              {running && stats ? formatMb(memUsed) : "—"}
              <span className="text-muted-foreground text-base font-normal">
                {" / "}
                {formatMb(memTotal)}
              </span>
            </span>
          }
        >
          <div className="bg-muted h-1.5 w-full overflow-hidden rounded-full">
            <div
              className={cn("h-full rounded-full transition-all", memBar)}
              style={{ width: `${memPct}%` }}
            />
          </div>
        </StatCard>
        {/* uptime จริงจาก started_at ของ container (agent ส่งมากับ stats) เดินทุกวินาที */}
        <StatCard
          icon={ClockIcon}
          label={t("overview.uptime")}
          value={
            running ? <UptimeValue startedAt={stats?.started_at ?? null} /> : "—"
          }
        >
          <span className="text-muted-foreground text-xs capitalize">
            {server.server_type} {server.mc_version}
          </span>
        </StatCard>
      </div>
    </div>
  );
}

export default function DashboardPage() {
  const t = useT();
  const dashboardServerId = useSettingsStore((s) => s.dashboardServerId);

  // ไม่ poll — stats/status อัปเดตผ่าน WS /ws/events (useEvents ที่ layout patch cache นี้)
  const servers = useQuery({
    queryKey: ["servers"],
    queryFn: () => apiGet("/api/servers", serversResponseSchema),
  });

  const serverList = servers.data?.servers ?? [];

  // server ที่เลือกดูภาพรวม: ค่าที่จำไว้ (ตั้งจาก sidebar switcher) ถ้ายังมีอยู่ ไม่งั้น fallback ตัวแรก
  const selectedId =
    dashboardServerId && serverList.some((s) => s.id === dashboardServerId)
      ? dashboardServerId
      : serverList[0]?.id ?? "";
  const selected = serverList.find((s) => s.id === selectedId);

  // ปุ่มสั่งงานย้ายไป top bar — ผูก server + สิทธิ์ operate เข้ากับ header ของหน้านี้
  // operate = cap servers.power ต่อ server ตัวนั้น (2 ชั้น) โหลด perm ผ่าน useActiveServer
  // ซึ่งเลือก active server ด้วย logic เดียวกับ selectedId ที่นี่
  const canOperate = useActiveServer().canOperate;
  useSetPageServer(selected, canOperate);

  return (
    <div className="grid gap-6">
      {servers.isPending ? (
        <div className="grid gap-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Skeleton className="h-28" />
            <Skeleton className="h-28" />
            <Skeleton className="h-28" />
            <Skeleton className="h-28" />
          </div>
        </div>
      ) : servers.isError ? (
        <p className="text-destructive text-sm">
          {t("dashboard.failedServers")}
          {servers.error instanceof ApiError
            ? `: ${servers.error.message}`
            : "."}
        </p>
      ) : serverList.length === 0 ? (
        <Card className="py-10">
          <CardContent className="text-muted-foreground flex justify-center text-sm">
            {t("dashboard.noServers")}
          </CardContent>
        </Card>
      ) : (
        selected && <ServerOverview server={selected} />
      )}
    </div>
  );
}
