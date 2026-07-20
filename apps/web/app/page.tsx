"use client";

import * as React from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ArrowRightIcon, BoxIcon, PlusIcon } from "lucide-react";
import { apiGet, ApiError } from "@/lib/api";
import { serversResponseSchema, type Server } from "@/lib/types";
import { formatUptime } from "@/lib/format";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useT } from "@/lib/i18n";
import { useAuthGuard } from "@/lib/use-auth-guard";
import { useSettingsStore } from "@/lib/settings/store";
import { EventsListener } from "@/components/layout/events-listener";
import { UserMenu } from "@/components/layout/user-menu";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

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

function Stat({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="grid gap-1">
      <span className="text-muted-foreground text-[0.625rem] font-semibold tracking-wider uppercase">
        {label}
      </span>
      <div className="text-lg leading-none font-semibold tracking-tight">
        {children}
      </div>
    </div>
  );
}

function ServerCard({ server }: { server: Server }) {
  const t = useT();
  const router = useRouter();
  const setDashboardServerId = useSettingsStore((s) => s.setDashboardServerId);
  const running = server.status === "running";
  const stats = server.stats;
  const online = running && stats ? stats.online_players.length : 0;
  const maxPlayers = stats?.max_players ?? 0;
  const tps = stats?.tps ?? 0;

  // เข้า server = ตั้ง active server แล้วไป /dashboard (ไม่มี id ใน URL ตาม convention ของ panel)
  const enter = () => {
    setDashboardServerId(server.id);
    router.push("/dashboard");
  };

  return (
    <Card
      role="link"
      tabIndex={0}
      onClick={enter}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          enter();
        }
      }}
      className="hover:border-primary/40 focus-visible:ring-ring cursor-pointer gap-4 transition-colors focus-visible:ring-2 focus-visible:outline-none"
    >
      <CardContent className="grid gap-4">
        <div className="flex items-start justify-between gap-3">
          <div className="grid min-w-0 gap-0.5">
            <h2 className="truncate text-lg font-semibold">{server.name}</h2>
            <p className="text-muted-foreground truncate font-mono text-xs capitalize">
              {server.server_type} {server.mc_version}
            </p>
          </div>
          <StatusBadge status={server.status} className="shrink-0" />
        </div>

        <div className="grid grid-cols-3 gap-3">
          <Stat label={t("overview.playersOnline")}>
            {running ? (
              <span>
                {online}
                {maxPlayers > 0 && (
                  <span className="text-muted-foreground text-sm font-normal">
                    /{maxPlayers}
                  </span>
                )}
              </span>
            ) : (
              "—"
            )}
          </Stat>
          {/* TPS มีเฉพาะ paper/spigot — type อื่น stats.tps = 0 (ไม่ใช่ "TPS เป็นศูนย์") */}
          <Stat label={t("overview.tps")}>
            {running && tps > 0 ? tps.toFixed(1) : "—"}
          </Stat>
          <Stat label={t("overview.uptime")}>
            {running ? <UptimeValue startedAt={stats?.started_at ?? null} /> : "—"}
          </Stat>
        </div>

        <div className="flex items-center justify-between gap-3 border-t pt-3">
          <span className="text-muted-foreground font-mono text-sm">
            {server.host_port ? `:${server.host_port}` : "—"}
          </span>
          <span className="text-primary flex items-center gap-1 text-sm font-semibold">
            {t("overview.manage")}
            <ArrowRightIcon className="size-4" />
          </span>
        </div>
      </CardContent>
    </Card>
  );
}

export default function ServerListPage() {
  const t = useT();
  const user = useAuthGuard();

  const servers = useQuery({
    queryKey: ["servers"],
    queryFn: () => apiGet("/api/servers", serversResponseSchema),
    enabled: Boolean(user),
  });
  const serverList = servers.data?.servers ?? [];

  if (!user) {
    return (
      <main className="mx-auto grid w-full max-w-6xl gap-4 p-4 md:p-6">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-40 w-full" />
      </main>
    );
  }

  return (
    <>
      <EventsListener />
      <main className="mx-auto grid w-full max-w-6xl gap-8 p-4 md:p-6">
        <header className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2.5">
            <BoxIcon className="size-6 shrink-0" />
            <span className="text-lg font-semibold">mc-panel</span>
          </div>
          <UserMenu user={user} />
        </header>

        <div className="flex flex-wrap items-end justify-between gap-3">
          <div className="grid gap-1">
            <h1 className="text-3xl font-bold tracking-tight">
              {t("serverList.title")}
            </h1>
            <p className="text-muted-foreground text-sm">
              {t("serverList.subtitle")}
            </p>
          </div>
          {/* ทางเข้าสร้าง server จากหน้านี้ — คนที่มี servers.create แต่ไม่มี servers.view_all
              ไม่เห็นเมนู admin > servers จึงต้องมีปุ่มตรงนี้ */}
          {hasCapability(user, CAPABILITY.serversCreate) && (
            <Button asChild>
              <Link href="/servers/new">
                <PlusIcon />
                {t("nav.newServer")}
              </Link>
            </Button>
          )}
        </div>

        {servers.isPending ? (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Skeleton className="h-52" />
            <Skeleton className="h-52" />
            <Skeleton className="h-52" />
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
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {serverList.map((s) => (
              <ServerCard key={s.id} server={s} />
            ))}
          </div>
        )}
      </main>
    </>
  );
}
