"use client";

import * as React from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  apiGet,
  apiSend,
  apiSendVoid,
  addPlayer,
  saveServerProperties,
  ApiError,
} from "@/lib/api";
import {
  createServerResponseSchema,
  jobResponseSchema,
  DEFAULT_GAME,
  type Permission,
  type Server,
} from "@/lib/types";
import { useT, type TranslationKey } from "@/lib/i18n";
import { LOCAL_OVERLAY_KEY } from "@/components/global-loading";
import type { ServerMetadata } from "@/components/server/new-server/use-server-metadata";

const POLL_INTERVAL_MS = 1_500;
// กันค้างถ้า job ไม่จบสักที — เลิกรอแล้วปล่อยให้ user ไปดูสถานะจริงที่ dashboard
const PROVISION_TIMEOUT_MS = 10 * 60 * 1_000;

export interface CreateServerInput {
  meta: ServerMetadata;
  // key ที่ต่างจาก default เท่านั้น — ไฟล์ยังไม่มีตอน apply, merge ฝั่ง backend จะ append ให้
  changedProps: Record<string, string>;
  accessDraft: Permission[];
  playersDraft: string[];
  // คนสร้างได้ owner จาก CreateServerWithOwner อยู่แล้ว — ข้ามตอน apply
  selfUserId?: string;
  onCreated: (server: Server) => void;
}

export interface CreateServerState {
  run: () => void;
  pending: boolean;
  // key ของ phase ที่กำลังทำ (null = ยังไม่เริ่ม) — ให้ overlay เอาไปแสดง
  phaseKey: TranslationKey | null;
}

// ลำดับการสร้างจริง — เป็น mutation เดียวที่ยิงหลาย request ต่อกันในทั้งแอป
// สร้าง → grant access → รอ provisioning job → เขียน properties → เพิ่ม whitelist
// ขั้นหลัง create ล้ม = toast บอกเป็นรายการแล้วไปต่อ (server ถูกสร้างแล้ว ห้าม rollback เงียบ ๆ)
export function useCreateServer(input: CreateServerInput): CreateServerState {
  const t = useT();
  const queryClient = useQueryClient();
  const [phaseKey, setPhaseKey] = React.useState<TranslationKey | null>(null);

  const {
    meta,
    changedProps,
    accessDraft,
    playersDraft,
    selfUserId,
    onCreated,
  } = input;

  const waitForJob = React.useCallback(async (jobId: string) => {
    const deadline = Date.now() + PROVISION_TIMEOUT_MS;
    for (;;) {
      try {
        const { job } = await apiGet(`/api/jobs/${jobId}`, jobResponseSchema);
        if (job.status === "succeeded") return true;
        if (job.status === "failed") return false;
      } catch {
        // server ถูกสร้างไปแล้ว — อ่านสถานะไม่ได้ก็แค่ถือว่ายังไม่พร้อม ห้ามโยน error
        // ออกไปให้ mutation fail (จะดูเหมือนสร้างไม่สำเร็จทั้งที่สร้างแล้ว)
        return false;
      }
      if (Date.now() > deadline) return false;
      await new Promise((r) => setTimeout(r, POLL_INTERVAL_MS));
    }
  }, []);

  const mutation = useMutation({
    // มี overlay ของตัวเองที่บอก phase อยู่แล้ว — กันไม่ให้ GlobalLoading ซ้อนทับ
    mutationKey: [LOCAL_OVERLAY_KEY, "create-server"],
    mutationFn: async (): Promise<{ server: Server; warned: boolean }> => {
      setPhaseKey("wizard.phaseCreating");

      const created = await apiSend(
        "POST",
        "/api/servers",
        {
          name: meta.name.trim(),
          node_id: meta.nodeId,
          game: DEFAULT_GAME,
          variant: meta.variant,
          game_version: meta.gameVersion,
          memory_mb: Number(meta.memoryMb),
          host_port: meta.hostPort === "" ? null : Number(meta.hostPort),
          accept_license: meta.requiresLicense ? meta.acceptLicense : true,
        },
        createServerResponseSchema,
      );

      const serverId = created.server.id;
      queryClient.invalidateQueries({ queryKey: ["servers"] });

      // access เป็นแถวใน DB ล้วน — ไม่ต้องรอไฟล์บนโหนด apply ได้ทันที
      let warned = false;
      setPhaseKey("wizard.phaseAccess");
      for (const entry of accessDraft) {
        if (entry.user_id === selfUserId) continue;
        try {
          await apiSendVoid("POST", `/api/servers/${serverId}/permissions`, {
            user_id: entry.user_id,
            role: entry.role,
            capabilities: entry.role === "owner" ? [] : entry.capabilities,
          });
        } catch {
          warned = true;
          toast.error(t("wizard.errAccessEntry", { name: entry.username }));
        }
      }

      const needsFiles =
        Object.keys(changedProps).length > 0 || playersDraft.length > 0;
      if (needsFiles) {
        setPhaseKey("wizard.phaseProvisioning");
        // job ล้ม/หมดเวลา = ไฟล์ยังไม่พร้อม ข้าม apply ไปเลย (server ยังอยู่ ตั้งต่อที่หน้า
        // settings/players ได้) — ไม่งั้นจะไปเจอ error ซ้อนที่ไม่ช่วยอะไร
        if (!(await waitForJob(created.job.id))) {
          toast.error(t("wizard.errProvisionSkipped"));
          return { server: created.server, warned: true };
        }
      }

      if (Object.keys(changedProps).length > 0) {
        setPhaseKey("wizard.phaseProperties");
        try {
          await saveServerProperties(serverId, changedProps);
        } catch {
          warned = true;
          toast.error(t("wizard.errProperties"));
        }
      }

      if (playersDraft.length > 0) {
        setPhaseKey("wizard.phasePlayers");
        for (const name of playersDraft) {
          try {
            await addPlayer(serverId, name);
          } catch {
            warned = true;
            toast.error(t("wizard.errPlayerEntry", { name }));
          }
        }
      }

      return { server: created.server, warned };
    },
    onSuccess: ({ server, warned }) => {
      if (!warned) {
        toast.success(t("wizard.createdToast", { name: server.name }));
      }
      onCreated(server);
    },
    onError: (err) => {
      if (err instanceof ApiError && err.code === "insufficient_memory") {
        // message มีตัวเลข used/total มาแล้ว — โชว์ตรง ๆ
        toast.error(err.message || t("new.errInsufficientMemory"));
        return;
      }
      toast.error(
        err instanceof ApiError ? err.message : t("new.failedCreate"),
      );
    },
    onSettled: () => setPhaseKey(null),
  });

  return {
    run: () => mutation.mutate(),
    pending: mutation.isPending,
    phaseKey,
  };
}
