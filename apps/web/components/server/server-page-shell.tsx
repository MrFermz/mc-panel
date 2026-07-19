"use client";

import * as React from "react";
import { ApiError } from "@/lib/api";
import type { Server } from "@/lib/types";
import { useT, type TranslationKey } from "@/lib/i18n";
import { useActiveServer, type ActiveServer } from "@/lib/use-active-server";
import {
  useSetBreadcrumbs,
  useSetPageServer,
} from "@/components/layout/breadcrumb-context";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

// เปลือกร่วมของหน้าที่ทำงานกับ active server (console/settings/files/logs):
// ตั้งชื่อหน้าใน top bar, ผูก server เข้ากับ header (status + ปุ่มสั่งงาน), แล้วจัดการ
// state loading/error/ไม่มี server/ไม่มีสิทธิ์ ให้เหมือนกันทุกหน้า
export function ServerPageShell({
  titleKey,
  need,
  children,
}: {
  titleKey: TranslationKey;
  // gate ต่อหน้า (เช่น settings = owner, files = canManageFiles) — undefined = ทุกคนที่เห็น server
  need?: (ctx: ActiveServer) => boolean;
  children: (ctx: ActiveServer & { server: Server }) => React.ReactNode;
}) {
  const t = useT();
  const ctx = useActiveServer();
  const { serversQuery, detailQuery, serverList, activeId, server } = ctx;

  useSetBreadcrumbs(
    React.useMemo(() => [{ label: t(titleKey) }], [t, titleKey]),
  );
  useSetPageServer(server, ctx.canOperate);

  if (serversQuery.isPending || (activeId !== "" && detailQuery.isPending)) {
    return <Skeleton className="h-96 w-full" />;
  }

  if (serversQuery.isError) {
    return (
      <p className="text-destructive text-sm">
        {t("dashboard.failedServers")}
        {serversQuery.error instanceof ApiError
          ? `: ${serversQuery.error.message}`
          : "."}
      </p>
    );
  }

  if (serverList.length === 0) {
    return (
      <Card className="py-10">
        <CardContent className="text-muted-foreground flex justify-center text-sm">
          {t("dashboard.noServers")}
        </CardContent>
      </Card>
    );
  }

  if (detailQuery.isError) {
    return (
      <p className="text-destructive text-sm">
        {t("server.failedLoad")}
        {detailQuery.error instanceof ApiError
          ? `: ${detailQuery.error.message}`
          : "."}
      </p>
    );
  }

  if (!server) return null;

  if (need && !need(ctx)) {
    return <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>;
  }

  return <>{children({ ...ctx, server })}</>;
}
