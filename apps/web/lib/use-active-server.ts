"use client";

import * as React from "react";
import { useQuery, type UseQueryResult } from "@tanstack/react-query";
import { apiGet } from "@/lib/api";
import {
  serverDetailResponseSchema,
  serversResponseSchema,
  type Permission,
  type Server,
} from "@/lib/types";
import { useMe } from "@/lib/use-me";
import { useSettingsStore } from "@/lib/settings/store";
import { CAPABILITY, effectiveServerCap } from "@/lib/capabilities";

export interface ActiveServer {
  serversQuery: UseQueryResult<{ servers: Server[] }>;
  detailQuery: UseQueryResult<{ server: Server; permissions: Permission[] }>;
  serverList: Server[];
  activeId: string;
  server: Server | undefined;
  isAdmin: boolean;
  isOwner: boolean;
  // can(cap) = สิทธิ์ 2 ชั้น (global AND per-server) ต่อ active server — ใช้ gate ทุกปุ่ม/แท็บ
  can: (cap: string) => boolean;
  canOperate: boolean;
  canConsoleWrite: boolean;
  canManageFiles: boolean;
}

// "active server" = ตัวที่เลือกไว้จากหน้า `/` หรือ `/admin/servers` (จำใน dashboardServerId)
// ไม่ผูก id ใน URL — หน้า console/settings/files/logs ใช้ตัวนี้ร่วมกัน สิทธิ์ต่อ server อ่านจาก
// /api/servers/{id} (list ไม่มี role ต่อ server) เหมือนที่หน้า detail เดิมทำ
export function useActiveServer(): ActiveServer {
  const { data: meData } = useMe();
  const me = meData?.user;
  const dashboardServerId = useSettingsStore((s) => s.dashboardServerId);

  const serversQuery = useQuery({
    queryKey: ["servers"],
    queryFn: () => apiGet("/api/servers", serversResponseSchema),
  });
  const serverList = serversQuery.data?.servers ?? [];

  // id ที่โหลด detail ไม่ผ่าน (ถูกลบ/ถูกถอนสิทธิ์ระหว่างทาง) — กันไม่ให้ค้างหน้า error ตลอด
  const [rejectedId, setRejectedId] = React.useState<string | null>(null);

  // ห้ามตัด dashboardServerId ทิ้งเพราะ "ไม่อยู่ใน serverList": /api/servers คืนเฉพาะ server
  // ที่ตัวเองมีชื่อใน access ส่วน admin เข้ามาจาก /admin/servers ได้ทุกตัว — ให้ detail query
  // (ซึ่ง enforce สิทธิ์จริงฝั่ง backend) เป็นคนตัดสิน แล้วค่อย fallback เมื่อมันพัง
  const preferredId =
    dashboardServerId && dashboardServerId !== rejectedId
      ? dashboardServerId
      : "";
  const activeId = preferredId || serverList[0]?.id || "";

  const detailQuery = useQuery({
    queryKey: ["servers", activeId],
    queryFn: () =>
      apiGet(`/api/servers/${activeId}`, serverDetailResponseSchema),
    enabled: activeId !== "",
  });

  React.useEffect(() => {
    if (detailQuery.isError && activeId !== "" && activeId === preferredId) {
      setRejectedId(activeId);
    }
  }, [detailQuery.isError, activeId, preferredId]);

  const server = detailQuery.data?.server;
  const permissions = detailQuery.data?.permissions ?? [];
  const myPermission = me
    ? permissions.find((p) => p.user_id === me.id)
    : undefined;
  const isAdmin = me?.is_admin ?? false;
  const isOwner = isAdmin || myPermission?.role === "owner";

  const can = (cap: string) => effectiveServerCap(me, myPermission, cap);

  return {
    serversQuery,
    detailQuery,
    serverList,
    activeId,
    server,
    isAdmin,
    isOwner,
    can,
    canOperate: can(CAPABILITY.serversPower),
    canConsoleWrite: can(CAPABILITY.consoleWrite),
    canManageFiles: can(CAPABILITY.filesWrite),
  };
}
