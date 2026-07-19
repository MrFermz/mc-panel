"use client";

import * as React from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { DownloadIcon, PlusIcon } from "lucide-react";
import { apiGet, ApiError } from "@/lib/api";
import {
  metaNodesResponseSchema,
  serversResponseSchema,
} from "@/lib/types";
import { formatMb } from "@/lib/format";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { useSettingsStore } from "@/lib/settings/store";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { StatusBadge } from "@/components/status-badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

// รายการ server ทั้งหมดที่ user เห็นได้ + จุดสร้าง/import (ปุ่มย้ายมาจาก dashboard)
// /api/servers filter ตามสิทธิ์ให้แล้ว (owner/permission หรือ servers.view_all)
export default function AdminServersPage() {
  const t = useT();
  const { data: meData } = useMe();
  const canCreateServer = hasCapability(meData?.user, CAPABILITY.serversCreate);
  const setDashboardServerId = useSettingsStore((s) => s.setDashboardServerId);

  useSetBreadcrumbs(
    React.useMemo(() => [{ label: t("nav.allServers") }], [t]),
  );

  const servers = useQuery({
    queryKey: ["servers"],
    queryFn: () => apiGet("/api/servers", serversResponseSchema),
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

  const serverList = servers.data?.servers ?? [];

  const actions = canCreateServer && (
    <div className="flex flex-wrap items-center gap-2">
      <Button size="sm" variant="outline" asChild>
        <Link href="/admin/servers/new?mode=import">
          <DownloadIcon />
          {t("import.button")}
        </Link>
      </Button>
      <Button size="sm" asChild>
        <Link href="/admin/servers/new">
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
          {t("adminServers.count", { count: serverList.length })}
        </p>
        {actions}
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
            <p>{t("dashboard.noServers")}</p>
            {canCreateServer && (
              <Button variant="outline" asChild>
                <Link href="/admin/servers/new">{t("dashboard.createFirst")}</Link>
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
                </TableRow>
              </TableHeader>
              <TableBody>
                {serverList.map((s) => (
                  <TableRow key={s.id}>
                    <TableCell className="font-medium">
                      {/* ไม่มีหน้า detail ต่อ server แล้ว — ตั้งเป็น active server
                          แล้วไป dashboard (หน้า console/files/… ตามไปเอง) */}
                      <Link
                        href="/"
                        onClick={() => setDashboardServerId(s.id)}
                        className="hover:underline"
                      >
                        {s.name}
                      </Link>
                    </TableCell>
                    <TableCell className="capitalize">
                      {s.server_type} {s.mc_version}
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={s.status} />
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
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
