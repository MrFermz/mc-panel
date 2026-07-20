"use client";

import * as React from "react";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { DownloadIcon, PlusIcon } from "lucide-react";
import { apiGet, apiSend, ApiError } from "@/lib/api";
import {
  jobResponseSchema,
  metaNodesResponseSchema,
  serverResponseSchema,
  serversResponseSchema,
  serverStatusSchema,
  type Server,
} from "@/lib/types";
import { formatMb, formatDateTime } from "@/lib/format";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useMe } from "@/lib/use-me";
import { useT, type TranslationKey } from "@/lib/i18n";
import { useSettingsStore } from "@/lib/settings/store";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { StatusBadge } from "@/components/status-badge";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  EditServerDialog,
  type EditServerBody,
} from "@/components/server/edit-server-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

type StateFilter = "active" | "deleted" | "all";

// หน้าจัดการ server ระดับแพเนล — ที่เดียวที่เห็น "ทุก" server (scope=all ต้องมี servers.view_all)
// รวมตัวที่อยู่ในถังขยะเพื่อ restore/purge ; หน้า `/` เห็นเฉพาะ server ที่ตัวเองมีชื่อใน access
export default function AdminServersPage() {
  const t = useT();
  const queryClient = useQueryClient();
  const { data: meData } = useMe();
  const me = meData?.user;
  const setDashboardServerId = useSettingsStore((s) => s.setDashboardServerId);

  const canViewAll = hasCapability(me, CAPABILITY.serversViewAll);
  const canCreate = hasCapability(me, CAPABILITY.serversCreate);
  const canEdit = hasCapability(me, CAPABILITY.serversEdit);
  const canDelete = hasCapability(me, CAPABILITY.serversDelete);
  const canRestore = hasCapability(me, CAPABILITY.serversRestore);
  const canPurge = hasCapability(me, CAPABILITY.serversPurge);

  useSetBreadcrumbs(
    React.useMemo(() => [{ label: t("nav.allServers") }], [t]),
  );

  const [editTarget, setEditTarget] = React.useState<Server | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<Server | null>(null);
  const [restoreTarget, setRestoreTarget] = React.useState<Server | null>(null);
  const [purgeTarget, setPurgeTarget] = React.useState<Server | null>(null);

  // filter ทั้งหมดทำฝั่ง UI — payload เป็นรายการ server ก้อนเดียวที่ WS คอย invalidate ให้อยู่แล้ว
  const [search, setSearch] = React.useState("");
  const [node, setNode] = React.useState("all");
  const [status, setStatus] = React.useState("all");
  const [state, setState] = React.useState<StateFilter>("active");

  const servers = useQuery({
    queryKey: ["servers", "all"],
    queryFn: () => apiGet("/api/servers?scope=all", serversResponseSchema),
    enabled: canViewAll,
  });

  const metaNodes = useQuery({
    queryKey: ["meta", "nodes"],
    queryFn: () => apiGet("/api/meta/nodes", metaNodesResponseSchema),
    staleTime: 60_000,
  });

  const nodeNames = React.useMemo(() => {
    const map = new Map<string, string>();
    for (const n of metaNodes.data?.nodes ?? []) map.set(n.id, n.name);
    return map;
  }, [metaNodes.data]);

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["servers"] });

  const failed = (err: unknown, fallback: string) =>
    toast.error(err instanceof ApiError ? err.message : fallback);

  const updateServer = useMutation({
    mutationFn: ({ id, body }: { id: string; body: EditServerBody }) =>
      apiSend("PATCH", `/api/servers/${id}`, body, serverResponseSchema),
    onSuccess: () => {
      toast.success(t("adminServers.updated"));
      setEditTarget(null);
      invalidate();
    },
    onError: (err) => failed(err, t("adminServers.failedUpdate")),
  });

  const deleteServer = useMutation({
    mutationFn: (server: Server) =>
      apiSend("DELETE", `/api/servers/${server.id}`, undefined, serverResponseSchema),
    onSuccess: (_data, server) => {
      toast.success(t("adminServers.deleted", { name: server.name }));
      setDeleteTarget(null);
      invalidate();
    },
    onError: (err) => {
      // ต้องหยุดก่อนลบ — backend ตอบ 409 invalid_state ถ้ายังรันอยู่
      if (err instanceof ApiError && err.code === "invalid_state") {
        toast.error(t("adminServers.stopFirst"));
        return;
      }
      failed(err, t("adminServers.failedDelete"));
    },
  });

  const restoreServer = useMutation({
    mutationFn: (server: Server) =>
      apiSend(
        "POST",
        `/api/servers/${server.id}/restore`,
        undefined,
        serverResponseSchema,
      ),
    onSuccess: (_data, server) => {
      toast.success(t("adminServers.restored", { name: server.name }));
      setRestoreTarget(null);
      invalidate();
    },
    onError: (err) => failed(err, t("adminServers.failedRestore")),
  });

  const purgeServer = useMutation({
    mutationFn: (server: Server) =>
      apiSend(
        "POST",
        `/api/servers/${server.id}/purge`,
        undefined,
        jobResponseSchema,
      ),
    onSuccess: (_data, server) => {
      // row หายจริงตอน job สำเร็จ (server_removed ผ่าน WS) — ที่นี่แค่บอกว่างานเริ่มแล้ว
      toast.success(t("adminServers.purging", { name: server.name }));
      setPurgeTarget(null);
      invalidate();
    },
    onError: (err) => failed(err, t("adminServers.failedPurge")),
  });

  // กันเข้าตรง URL — เมนูซ่อนอยู่แล้วแต่ต้องกันซ้ำ
  if (me && !canViewAll) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>
    );
  }

  const serverList = (servers.data?.servers ?? []).filter((s) => {
    if (state === "active" && s.deleted_at !== null) return false;
    if (state === "deleted" && s.deleted_at === null) return false;
    if (node !== "all" && s.node_id !== node) return false;
    if (status !== "all" && s.status !== status) return false;
    if (search && !s.name.toLowerCase().includes(search.trim().toLowerCase())) {
      return false;
    }
    return true;
  });

  const actions = canCreate && (
    <div className="flex flex-wrap items-center gap-2">
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
    </div>
  );

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">
          {t("adminServers.subtitle")}
        </p>
        {actions}
      </div>

      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
        <Input
          className="w-full sm:max-w-xs"
          placeholder={t("adminServers.filterSearch")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Select value={state} onValueChange={(v) => setState(v as StateFilter)}>
          <SelectTrigger
            className="w-full sm:w-44"
            aria-label={t("adminServers.filterState")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">
              {t("adminServers.stateActive")}
            </SelectItem>
            <SelectItem value="deleted">
              {t("adminServers.stateDeleted")}
            </SelectItem>
            <SelectItem value="all">{t("adminServers.stateAll")}</SelectItem>
          </SelectContent>
        </Select>
        <Select value={status} onValueChange={setStatus}>
          <SelectTrigger
            className="w-full sm:w-44"
            aria-label={t("adminServers.filterStatus")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("adminServers.statusAll")}</SelectItem>
            {serverStatusSchema.options.map((s) => (
              <SelectItem key={s} value={s} className="capitalize">
                {t(`status.${s}` as TranslationKey)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select value={node} onValueChange={setNode}>
          <SelectTrigger
            className="w-full sm:w-44"
            aria-label={t("adminServers.filterNode")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("adminServers.nodeAll")}</SelectItem>
            {(metaNodes.data?.nodes ?? []).map((n) => (
              <SelectItem key={n.id} value={n.id}>
                {n.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-muted-foreground text-sm sm:ml-auto">
          {t("adminServers.count", { count: serverList.length })}
        </p>
      </div>

      {servers.isPending ? (
        <Skeleton className="h-64 w-full" />
      ) : servers.isError ? (
        <p className="text-destructive text-sm">
          {t("dashboard.failedServers")}
          {servers.error instanceof ApiError
            ? `: ${servers.error.message}`
            : "."}
        </p>
      ) : serverList.length === 0 ? (
        <Card className="py-10">
          <CardContent className="text-muted-foreground flex flex-col items-center gap-3 text-sm">
            <p>{t("adminServers.empty")}</p>
            {canCreate && state !== "deleted" && (
              <Button variant="outline" asChild>
                <Link href="/servers/new">{t("dashboard.createFirst")}</Link>
              </Button>
            )}
          </CardContent>
        </Card>
      ) : (
        <Card className="py-0">
          <CardContent className="overflow-x-auto px-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("adminServers.name")}</TableHead>
                  <TableHead>{t("adminServers.type")}</TableHead>
                  <TableHead>{t("adminServers.status")}</TableHead>
                  <TableHead>{t("adminServers.node")}</TableHead>
                  <TableHead>{t("adminServers.port")}</TableHead>
                  <TableHead>{t("res.ram")}</TableHead>
                  <TableHead className="text-right">
                    {t("adminServers.actions")}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {serverList.map((s) => {
                  const trashed = s.deleted_at !== null;
                  return (
                    <TableRow key={s.id} className={trashed ? "opacity-70" : undefined}>
                      <TableCell className="font-medium">
                        <div className="grid gap-0.5">
                          {trashed ? (
                            // อยู่ในถังขยะ = เข้าไปจัดการไม่ได้ (ทุก endpoint ตอบ 404)
                            <span>{s.name}</span>
                          ) : (
                            // ไม่มีหน้า detail ต่อ server แล้ว — ตั้งเป็น active server
                            // แล้วไป dashboard (หน้า console/files/… ตามไปเอง)
                            <Link
                              href="/dashboard"
                              onClick={() => setDashboardServerId(s.id)}
                              className="hover:underline"
                            >
                              {s.name}
                            </Link>
                          )}
                          {trashed && (
                            <span className="text-muted-foreground text-xs font-normal">
                              {t("adminServers.deletedAt", {
                                date: formatDateTime(s.deleted_at),
                              })}
                            </span>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="capitalize">
                        {s.server_type} {s.mc_version}
                      </TableCell>
                      <TableCell>
                        {trashed ? (
                          <Badge variant="outline" className="text-destructive">
                            {t("adminServers.deletedBadge")}
                          </Badge>
                        ) : (
                          <StatusBadge status={s.status} />
                        )}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {nodeNames.get(s.node_id) ?? "—"}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {s.host_port ?? "—"}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatMb(s.memory_mb)}
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end gap-2">
                          {trashed ? (
                            <>
                              {canRestore && (
                                <Button
                                  variant="outline"
                                  size="sm"
                                  onClick={() => setRestoreTarget(s)}
                                >
                                  {t("adminServers.restore")}
                                </Button>
                              )}
                              {canPurge && (
                                <Button
                                  variant="outline"
                                  size="sm"
                                  className="text-destructive hover:text-destructive"
                                  onClick={() => setPurgeTarget(s)}
                                >
                                  {t("adminServers.purge")}
                                </Button>
                              )}
                            </>
                          ) : (
                            <>
                              {canEdit && (
                                <Button
                                  variant="outline"
                                  size="sm"
                                  onClick={() => setEditTarget(s)}
                                >
                                  {t("adminServers.edit")}
                                </Button>
                              )}
                              {canDelete && (
                                <Button
                                  variant="outline"
                                  size="sm"
                                  className="text-destructive hover:text-destructive"
                                  onClick={() => setDeleteTarget(s)}
                                >
                                  {t("adminServers.delete")}
                                </Button>
                              )}
                            </>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      <EditServerDialog
        server={editTarget}
        pending={updateServer.isPending}
        onOpenChange={(open) => {
          if (!open) setEditTarget(null);
        }}
        onSubmit={(body) => {
          if (!editTarget) return;
          if (Object.keys(body).length === 0) {
            setEditTarget(null);
            return;
          }
          updateServer.mutate({ id: editTarget.id, body });
        }}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("adminServers.deleteTitle", { name: deleteTarget?.name ?? "" })}
        description={t("adminServers.deleteDesc")}
        confirmLabel={t("adminServers.delete")}
        destructive
        pending={deleteServer.isPending}
        onConfirm={() => {
          if (deleteTarget) deleteServer.mutate(deleteTarget);
        }}
      />

      <ConfirmDialog
        open={restoreTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRestoreTarget(null);
        }}
        title={t("adminServers.restoreTitle", {
          name: restoreTarget?.name ?? "",
        })}
        description={t("adminServers.restoreDesc")}
        confirmLabel={t("adminServers.restore")}
        pending={restoreServer.isPending}
        onConfirm={() => {
          if (restoreTarget) restoreServer.mutate(restoreTarget);
        }}
      />

      <ConfirmDialog
        open={purgeTarget !== null}
        onOpenChange={(open) => {
          if (!open) setPurgeTarget(null);
        }}
        title={t("adminServers.purgeTitle", { name: purgeTarget?.name ?? "" })}
        description={t("adminServers.purgeDesc")}
        confirmLabel={t("adminServers.purge")}
        destructive
        // พิมพ์ชื่อยืนยันก่อน — จุดเดียวในระบบที่ข้อมูลหายจริง
        requireText={purgeTarget?.name}
        pending={purgeServer.isPending}
        onConfirm={() => {
          if (purgeTarget) purgeServer.mutate(purgeTarget);
        }}
      />
    </div>
  );
}
