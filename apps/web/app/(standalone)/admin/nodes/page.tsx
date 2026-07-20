"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ActivityIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { apiGet, apiSend, apiSendVoid, ApiError } from "@/lib/api";
import {
  createNodeResponseSchema,
  nodesResponseSchema,
  serversResponseSchema,
  type Node,
} from "@/lib/types";
import { formatMb, formatRelative } from "@/lib/format";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { SecretDialog } from "@/components/secret-dialog";
import { NodeStatsChart } from "@/components/node/node-stats-chart";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { cn } from "@/lib/utils";

export default function AdminNodesPage() {
  const t = useT();
  useSetBreadcrumbs(
    React.useMemo(
      () => [{ label: t("nav.admin") }, { label: t("nodes.title") }],
      [t],
    ),
  );
  const queryClient = useQueryClient();
  const { data: meData } = useMe();
  const me = meData?.user;

  const [registerOpen, setRegisterOpen] = React.useState(false);
  const [nodeName, setNodeName] = React.useState("");
  const [token, setToken] = React.useState<{
    name: string;
    value: string;
  } | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<Node | null>(null);
  // node id ที่กางกราฟ resource อยู่ (หลาย node พร้อมกันได้)
  const [chartOpen, setChartOpen] = React.useState<Set<string>>(new Set());

  const toggleChart = (id: string) =>
    setChartOpen((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });

  const nodes = useQuery({
    // realtime updates มาจาก events WS (node_stats) — ไม่ poll
    queryKey: ["nodes"],
    queryFn: () => apiGet("/api/nodes", nodesResponseSchema),
    enabled: me?.is_admin === true,
  });

  // ใช้เช็คว่า node ไหนยังมี server (ปุ่มลบต้อง disabled) — ต้องเป็น scope=all เท่านั้น
  // เพราะ /api/servers ปกติคืนเฉพาะ server ที่ตัวเองมีชื่อใน access (admin ก็ด้วย) จะนับไม่ครบ
  // แล้วเปิดปุ่มลบ node ที่ยังมี server อยู่. scope=all รวมตัวที่อยู่ในถังขยะด้วย — ถูกแล้ว
  // เพราะ row พวกนั้นยังอ้าง node อยู่จริง (FK RESTRICT)
  const servers = useQuery({
    queryKey: ["servers", "all"],
    queryFn: () => apiGet("/api/servers?scope=all", serversResponseSchema),
    enabled: me?.is_admin === true,
  });

  const serverCountByNode = React.useMemo(() => {
    const counts = new Map<string, number>();
    for (const s of servers.data?.servers ?? []) {
      counts.set(s.node_id, (counts.get(s.node_id) ?? 0) + 1);
    }
    return counts;
  }, [servers.data]);

  const register = useMutation({
    mutationFn: () =>
      apiSend(
        "POST",
        "/api/nodes",
        { name: nodeName.trim() },
        createNodeResponseSchema,
      ),
    onSuccess: (data) => {
      setRegisterOpen(false);
      setNodeName("");
      setToken({ name: data.node.name, value: data.token });
      queryClient.invalidateQueries({ queryKey: ["nodes"] });
    },
    onError: (err) => {
      toast.error(
        err instanceof ApiError ? err.message : t("nodes.failedRegister"),
      );
    },
  });

  const remove = useMutation({
    mutationFn: (id: string) => apiSendVoid("DELETE", `/api/nodes/${id}`),
    onSuccess: () => {
      toast.success(t("nodes.deleted"));
      setDeleteTarget(null);
      queryClient.invalidateQueries({ queryKey: ["nodes"] });
    },
    onError: (err) => {
      toast.error(
        err instanceof ApiError ? err.message : t("nodes.failedDelete"),
      );
    },
  });

  if (me && !me.is_admin) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>
    );
  }

  return (
    <div className="grid gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">{t("nodes.title")}</h1>
        <Button size="sm" onClick={() => setRegisterOpen(true)}>
          <PlusIcon />
          {t("nodes.register")}
        </Button>
      </div>

      {nodes.isPending ? (
        <Skeleton className="h-40 w-full" />
      ) : nodes.isError ? (
        <p className="text-destructive text-sm">{t("nodes.failedLoad")}</p>
      ) : nodes.data.nodes.length === 0 ? (
        <Card className="py-10">
          <CardContent className="text-muted-foreground text-center text-sm">
            {t("nodes.empty")}
          </CardContent>
        </Card>
      ) : (
        <Card className="py-0">
          <CardContent className="overflow-x-auto px-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("nodes.colName")}</TableHead>
                  <TableHead>{t("nodes.colStatus")}</TableHead>
                  <TableHead>{t("nodes.colAgent")}</TableHead>
                  <TableHead>{t("nodes.colCpu")}</TableHead>
                  <TableHead>{t("nodes.colRam")}</TableHead>
                  <TableHead>{t("nodes.colDisk")}</TableHead>
                  <TableHead>{t("nodes.colServers")}</TableHead>
                  <TableHead>{t("nodes.colHeartbeat")}</TableHead>
                  <TableHead className="w-24" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {nodes.data.nodes.map((node) => {
                  const serverCount = serverCountByNode.get(node.id) ?? 0;
                  const online = node.status === "online";
                  const showChart = chartOpen.has(node.id);
                  return (
                    <React.Fragment key={node.id}>
                      <TableRow>
                        <TableCell className="font-medium">
                          {node.name}
                        </TableCell>
                        <TableCell>
                          <Badge
                            variant="outline"
                            className={cn(
                              online
                                ? "bg-green-500/15 text-green-400 border-green-500/30"
                                : "bg-red-500/15 text-red-400 border-red-500/30",
                            )}
                          >
                            {t(
                              online
                                ? "nodeStatus.online"
                                : "nodeStatus.offline",
                            )}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-muted-foreground text-xs">
                          {node.agent_version || "-"}
                          {node.os && (
                            <span>
                              {" "}
                              ({node.os}/{node.arch})
                            </span>
                          )}
                        </TableCell>
                        <TableCell>{node.cpu_percent.toFixed(1)}%</TableCell>
                        <TableCell className="text-xs">
                          {formatMb(node.memory_used_mb)} /{" "}
                          {formatMb(node.memory_total_mb)}
                        </TableCell>
                        <TableCell className="text-xs">
                          {formatMb(node.disk_used_mb)} /{" "}
                          {formatMb(node.disk_total_mb)}
                        </TableCell>
                        <TableCell>{serverCount}</TableCell>
                        <TableCell className="text-muted-foreground text-xs">
                          {formatRelative(node.last_heartbeat_at)}
                        </TableCell>
                        <TableCell>
                          <div className="flex justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="icon"
                              aria-label={t("nodes.viewChart", {
                                name: node.name,
                              })}
                              aria-expanded={showChart}
                              className={cn(
                                showChart && "text-foreground bg-accent",
                              )}
                              onClick={() => toggleChart(node.id)}
                            >
                              <ActivityIcon />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="text-destructive"
                              disabled={serverCount > 0 || servers.isPending}
                              title={
                                serverCount > 0
                                  ? t("nodes.deleteAllFirst")
                                  : undefined
                              }
                              onClick={() => setDeleteTarget(node)}
                              aria-label={t("nodes.deleteAria", {
                                name: node.name,
                              })}
                            >
                              <Trash2Icon />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                      {showChart && (
                        <TableRow className="hover:bg-transparent">
                          <TableCell colSpan={9} className="bg-muted/30">
                            {online ? (
                              <NodeStatsChart node={node} />
                            ) : (
                              <p className="text-muted-foreground text-xs">
                                {t("nodes.offlineNoStats")}
                              </p>
                            )}
                          </TableCell>
                        </TableRow>
                      )}
                    </React.Fragment>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      <Dialog open={registerOpen} onOpenChange={setRegisterOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("nodes.registerTitle")}</DialogTitle>
            <DialogDescription>{t("nodes.registerDesc")}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (nodeName.trim() !== "" && !register.isPending)
                register.mutate();
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="n-name">{t("nodes.nodeName")}</Label>
              <Input
                id="n-name"
                required
                maxLength={100}
                placeholder="node-1"
                value={nodeName}
                onChange={(e) => setNodeName(e.target.value)}
              />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setRegisterOpen(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={nodeName.trim() === "" || register.isPending}
              >
                {register.isPending
                  ? t("nodes.registering")
                  : t("nodes.register.submit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <SecretDialog
        open={token !== null}
        onOpenChange={(open) => {
          if (!open) setToken(null);
        }}
        title={t("nodes.tokenTitle", { name: token?.name ?? "" })}
        description={t("nodes.tokenDesc")}
        secret={token?.value ?? ""}
        extra={
          <p className="text-muted-foreground text-xs">
            e.g. <code className="font-mono">AGENT_TOKEN=&lt;token&gt;</code> in
            the agent&apos;s env file, or pass it to{" "}
            <code className="font-mono">scripts/install-agent.sh</code>.
          </p>
        }
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("nodes.deleteTitle", { name: deleteTarget?.name ?? "" })}
        description={t("nodes.deleteDesc")}
        confirmLabel={t("nodes.deleteConfirm")}
        destructive
        pending={remove.isPending}
        onConfirm={() => {
          if (deleteTarget) remove.mutate(deleteTarget.id);
        }}
      />
    </div>
  );
}
