"use client";

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

export interface ActiveServer {
  serversQuery: UseQueryResult<{ servers: Server[] }>;
  detailQuery: UseQueryResult<{ server: Server; permissions: Permission[] }>;
  serverList: Server[];
  activeId: string;
  server: Server | undefined;
  isAdmin: boolean;
  isOwner: boolean;
  canOperate: boolean;
  canConsoleWrite: boolean;
  canManageFiles: boolean;
}

// "active server" = ตัวที่เลือกจาก switcher ใน sidebar (จำใน dashboardServerId) ไม่ผูก id ใน URL
// หน้า console/settings/files/logs ใช้ตัวนี้ร่วมกัน — สิทธิ์ต่อ server อ่านจาก /api/servers/{id}
// (list ไม่มี role ต่อ server) เหมือนที่หน้า detail เดิมทำ
export function useActiveServer(): ActiveServer {
  const { data: meData } = useMe();
  const me = meData?.user;
  const dashboardServerId = useSettingsStore((s) => s.dashboardServerId);

  const serversQuery = useQuery({
    queryKey: ["servers"],
    queryFn: () => apiGet("/api/servers", serversResponseSchema),
  });
  const serverList = serversQuery.data?.servers ?? [];

  const activeId =
    dashboardServerId && serverList.some((s) => s.id === dashboardServerId)
      ? dashboardServerId
      : serverList[0]?.id ?? "";

  const detailQuery = useQuery({
    queryKey: ["servers", activeId],
    queryFn: () =>
      apiGet(`/api/servers/${activeId}`, serverDetailResponseSchema),
    enabled: activeId !== "",
  });

  const server = detailQuery.data?.server;
  const permissions = detailQuery.data?.permissions ?? [];
  const myPermission = me
    ? permissions.find((p) => p.user_id === me.id)
    : undefined;
  const isAdmin = me?.is_admin ?? false;
  const isOwner = isAdmin || myPermission?.role === "owner";

  return {
    serversQuery,
    detailQuery,
    serverList,
    activeId,
    server,
    isAdmin,
    isOwner,
    canOperate: isOwner || myPermission?.role === "operator",
    canConsoleWrite: isOwner || (myPermission?.can_console_write ?? false),
    canManageFiles: isOwner || (myPermission?.can_manage_files ?? false),
  };
}
