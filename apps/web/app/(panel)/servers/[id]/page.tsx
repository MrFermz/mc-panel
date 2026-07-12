"use client";

import * as React from "react";
import dynamic from "next/dynamic";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  PlayIcon,
  RotateCwIcon,
  SquareIcon,
  ZapIcon,
} from "lucide-react";
import { apiGet, apiSend, ApiError } from "@/lib/api";
import {
  jobResponseSchema,
  serverDetailResponseSchema,
  type Server,
} from "@/lib/types";
import { formatCpuPercent, formatMb } from "@/lib/format";
import { useMe } from "@/lib/use-me";
import { useT, type TranslationKey } from "@/lib/i18n";
import {
  useStatsHistoryStore,
  type StatPoint,
} from "@/lib/settings/stats-history";
import { StatusBadge } from "@/components/status-badge";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import SettingsTab from "@/components/server/settings-tab";
import AccessTab from "@/components/server/access-tab";
import FilesTab from "@/components/server/files-tab";
import JobsTab from "@/components/server/jobs-tab";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";

// @xterm/xterm แตะ window ตั้งแต่ import — ต้องปิด SSR ของทั้ง tab
const ConsoleTab = dynamic(() => import("@/components/server/console-tab"), {
  ssr: false,
  loading: () => <Skeleton className="h-[28rem] w-full" />,
});

// recharts ResponsiveContainer measure DOM — ปิด SSR กัน hydration mismatch
const StatsChart = dynamic(
  () => import("@/components/server/stats-chart").then((m) => m.StatsChart),
  { ssr: false, loading: () => <Skeleton className="h-[340px] w-full" /> },
);

type ServerAction = "start" | "stop" | "restart" | "kill";

function actionAvailability(server: Server): Record<ServerAction, boolean> {
  return {
    start: server.status === "stopped" || server.status === "errored",
    stop: server.status === "running" || server.status === "starting",
    restart: server.status === "running",
    kill:
      server.status === "running" ||
      server.status === "starting" ||
      server.status === "stopping",
  };
}

function ServerDetailPage() {
  const { id } = useParams<{ id: string }>();
  const t = useT();
  const router = useRouter();
  const searchParams = useSearchParams();
  const queryClient = useQueryClient();
  const { data: meData } = useMe();
  const [killOpen, setKillOpen] = React.useState(false);

  const history = useStatsHistoryStore((s) => s.history[id] ?? EMPTY_HISTORY);
  const pushStats = useStatsHistoryStore((s) => s.push);
  const resetStats = useStatsHistoryStore((s) => s.reset);

  // ไม่ poll แล้ว — stats/status อัปเดตผ่าน WS /ws/events (useEvents ที่ layout patch cache นี้)
  const detail = useQuery({
    queryKey: ["servers", id],
    queryFn: () => apiGet(`/api/servers/${id}`, serverDetailResponseSchema),
  });

  const server = detail.data?.server;
  const stats = server?.stats;
  const isRunning = server?.status === "running";

  // เก็บ history stats ฝั่ง client ทุกครั้งที่ค่าใหม่เข้ามา (refetch ทุก 5s)
  React.useEffect(() => {
    if (!isRunning || !stats) return;
    pushStats(id, {
      t: new Date(stats.updated_at).getTime() || Date.now(),
      cpu: stats.cpu_percent,
      memUsed: stats.memory_used_mb,
      memLimit: stats.memory_limit_mb,
    });
  }, [id, isRunning, stats, pushStats]);

  // reset history เมื่อ server ไม่ได้รันแล้ว (กราฟเริ่มใหม่รอบหน้า)
  React.useEffect(() => {
    if (server && !isRunning) resetStats(id);
  }, [id, server, isRunning, resetStats]);

  const action = useMutation({
    mutationFn: (a: ServerAction) =>
      apiSend("POST", `/api/servers/${id}/actions`, { action: a }, jobResponseSchema),
    onSuccess: (_data, a) => {
      toast.success(
        t("server.requested", { action: t(`server.action.${a}` as TranslationKey) }),
      );
      setKillOpen(false);
      queryClient.invalidateQueries({ queryKey: ["servers", id] });
      queryClient.invalidateQueries({ queryKey: ["servers", id, "jobs"] });
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : t("server.actionFailed"));
    },
  });

  const me = meData?.user;
  const permissions = detail.data?.permissions ?? [];
  const myPermission = me
    ? permissions.find((p) => p.user_id === me.id)
    : undefined;
  const isAdmin = me?.is_admin ?? false;
  const isOwner = isAdmin || myPermission?.role === "owner";
  const canOperate = isOwner || myPermission?.role === "operator";
  const canConsoleWrite = isOwner || (myPermission?.can_console_write ?? false);
  const canManageFiles = isOwner || (myPermission?.can_manage_files ?? false);

  // ชุด tab ที่ user เห็นจริง ใช้ทั้ง validate ?tab= และ map เป็น label ของ breadcrumb
  const availableTabs = React.useMemo(() => {
    const tabs: { value: string; labelKey: TranslationKey }[] = [
      { value: "console", labelKey: "tab.console" },
    ];
    if (isOwner) {
      tabs.push({ value: "settings", labelKey: "tab.settings" });
      tabs.push({ value: "access", labelKey: "tab.access" });
    }
    if (canManageFiles) tabs.push({ value: "files", labelKey: "tab.files" });
    tabs.push({ value: "jobs", labelKey: "tab.jobs" });
    return tabs;
  }, [isOwner, canManageFiles]);

  const tabParam = searchParams.get("tab");
  const activeTab =
    tabParam && availableTabs.some((tb) => tb.value === tabParam)
      ? tabParam
      : "console";

  const onTabChange = (v: string) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set("tab", v);
    router.replace(`/servers/${id}?${params.toString()}`, { scroll: false });
  };

  const serverName = server?.name;
  useSetBreadcrumbs(
    React.useMemo(() => {
      if (!serverName) return [];
      return [
        { label: serverName, href: `/servers/${id}` },
        { label: t(`tab.${activeTab}` as TranslationKey) },
      ];
    }, [serverName, id, activeTab, t]),
  );

  if (detail.isPending) {
    return (
      <div className="grid gap-4">
        <Skeleton className="h-10 w-64" />
        <Skeleton className="h-[28rem] w-full" />
      </div>
    );
  }

  if (detail.isError) {
    return (
      <p className="text-destructive text-sm">
        {t("server.failedLoad")}
        {detail.error instanceof ApiError ? `: ${detail.error.message}` : "."}
      </p>
    );
  }

  if (!server) return null;

  const available = actionAvailability(server);
  const busy = action.isPending;

  return (
    <div className="grid gap-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <h1 className="truncate text-xl font-semibold">{server.name}</h1>
          <StatusBadge status={server.status} />
        </div>
        {canOperate && (
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              disabled={!available.start || busy}
              onClick={() => action.mutate("start")}
            >
              <PlayIcon />
              {t("server.start")}
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={!available.stop || busy}
              onClick={() => action.mutate("stop")}
            >
              <SquareIcon />
              {t("server.stop")}
            </Button>
            <Button
              size="sm"
              variant="secondary"
              disabled={!available.restart || busy}
              onClick={() => action.mutate("restart")}
            >
              <RotateCwIcon />
              {t("server.restart")}
            </Button>
            <Button
              size="sm"
              variant="destructive"
              disabled={!available.kill || busy}
              onClick={() => setKillOpen(true)}
            >
              <ZapIcon />
              {t("server.kill")}
            </Button>
          </div>
        )}
      </div>

      <p className="text-muted-foreground -mt-4 text-sm capitalize">
        {server.server_type} {server.mc_version} · {formatMb(server.memory_mb)} RAM
        {server.host_port !== null && (
          <span className="normal-case">
            {" · "}
            {t("server.hostPort", { port: server.host_port })}
          </span>
        )}
        {server.host_port === null && (
          <span className="normal-case"> · {t("server.velocityOnly")}</span>
        )}
        {isRunning && stats && (
          <span className="normal-case">
            {" · "}
            {t("res.cpu")} {formatCpuPercent(stats.cpu_percent)} · {t("res.ram")}{" "}
            {formatMb(stats.memory_used_mb)} / {formatMb(stats.memory_limit_mb)}
          </span>
        )}
      </p>

      {isRunning && stats && (
        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle className="text-muted-foreground text-xs font-semibold tracking-wider uppercase">
              {t("stats.title")}
            </CardTitle>
          </CardHeader>
          <CardContent className="px-4">
            <StatsChart stats={stats} history={history} height={140} />
          </CardContent>
        </Card>
      )}

      <Tabs value={activeTab} onValueChange={onTabChange}>
        <div className="max-w-full overflow-x-auto">
          <TabsList className="w-max max-w-none">
            <TabsTrigger value="console">{t("tab.console")}</TabsTrigger>
            {isOwner && (
              <TabsTrigger value="settings">{t("tab.settings")}</TabsTrigger>
            )}
            {isOwner && (
              <TabsTrigger value="access">{t("tab.access")}</TabsTrigger>
            )}
            {canManageFiles && (
              <TabsTrigger value="files">{t("tab.files")}</TabsTrigger>
            )}
            <TabsTrigger value="jobs">{t("tab.jobs")}</TabsTrigger>
          </TabsList>
        </div>
        <TabsContent value="console" className="mt-2">
          <ConsoleTab serverId={server.id} canWrite={canConsoleWrite} />
        </TabsContent>
        {isOwner && (
          <TabsContent value="settings" className="mt-2">
            <SettingsTab server={server} />
          </TabsContent>
        )}
        {isOwner && (
          <TabsContent value="access" className="mt-2">
            <AccessTab serverId={server.id} />
          </TabsContent>
        )}
        {canManageFiles && (
          <TabsContent value="files" className="mt-2">
            <FilesTab serverId={server.id} />
          </TabsContent>
        )}
        <TabsContent value="jobs" className="mt-2">
          <JobsTab serverId={server.id} />
        </TabsContent>
      </Tabs>

      <ConfirmDialog
        open={killOpen}
        onOpenChange={setKillOpen}
        title={t("server.killTitle", { name: server.name })}
        description={t("server.killDesc")}
        confirmLabel={t("server.killConfirm")}
        destructive
        pending={action.isPending}
        onConfirm={() => action.mutate("kill")}
      />
    </div>
  );
}

const EMPTY_HISTORY: StatPoint[] = [];

// useSearchParams ต้องอยู่ใน Suspense boundary ไม่งั้น build บ่น CSR bailout
export default function ServerDetailPageWrapper() {
  return (
    <React.Suspense
      fallback={
        <div className="grid gap-4">
          <Skeleton className="h-10 w-64" />
          <Skeleton className="h-[28rem] w-full" />
        </div>
      }
    >
      <ServerDetailPage />
    </React.Suspense>
  );
}
