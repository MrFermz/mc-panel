"use client";

import * as React from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { PlayIcon, RotateCwIcon, SquareIcon, ZapIcon } from "lucide-react";
import { apiSend, ApiError } from "@/lib/api";
import { jobResponseSchema, type Server } from "@/lib/types";
import { useT, type TranslationKey } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { ConfirmDialog } from "@/components/confirm-dialog";

type Action = "start" | "stop" | "restart" | "kill";

// ปุ่มสั่งงาน server บน top bar — Start/Stop สลับกันตามสถานะ, Restart, Kill (มี confirm)
// ใช้ทั้ง dashboard overview และหน้า detail (server มาจาก header context)
export function ServerHeaderControls({ server }: { server: Server }) {
  const t = useT();
  const queryClient = useQueryClient();
  const [killOpen, setKillOpen] = React.useState(false);

  const action = useMutation({
    mutationFn: (a: Action) =>
      apiSend(
        "POST",
        `/api/servers/${server.id}/actions`,
        { action: a },
        jobResponseSchema,
      ),
    onSuccess: (_data, a) => {
      toast.success(
        t("server.requested", {
          action: t(`server.action.${a}` as TranslationKey),
        }),
      );
      setKillOpen(false);
      queryClient.invalidateQueries({ queryKey: ["servers"] });
      queryClient.invalidateQueries({ queryKey: ["servers", server.id] });
      queryClient.invalidateQueries({ queryKey: ["servers", server.id, "jobs"] });
    },
    onError: (err) => {
      toast.error(
        err instanceof ApiError ? err.message : t("server.actionFailed"),
      );
    },
  });

  const s = server.status;
  // Stop โชว์แทน Start เมื่อกำลังรัน/กำลังเริ่ม/กำลังหยุด; นอกนั้นโชว์ Start
  const showStop = s === "running" || s === "starting" || s === "stopping";
  const canStart = s === "stopped" || s === "errored";
  const canStop = s === "running" || s === "starting";
  const canRestart = s === "running";
  const canKill = s === "running" || s === "starting" || s === "stopping";
  const busy = action.isPending || s === "provisioning" || s === "deleting";

  return (
    <div className="flex items-center gap-1.5">
      {showStop ? (
        <Button
          size="sm"
          variant="secondary"
          disabled={!canStop || busy}
          onClick={() => action.mutate("stop")}
        >
          <SquareIcon />
          <span className="hidden sm:inline">{t("server.stop")}</span>
        </Button>
      ) : (
        <Button
          size="sm"
          disabled={!canStart || busy}
          onClick={() => action.mutate("start")}
        >
          <PlayIcon />
          <span className="hidden sm:inline">{t("server.start")}</span>
        </Button>
      )}
      <Button
        size="sm"
        variant="secondary"
        disabled={!canRestart || busy}
        onClick={() => action.mutate("restart")}
      >
        <RotateCwIcon />
        <span className="hidden sm:inline">{t("server.restart")}</span>
      </Button>
      <Button
        size="sm"
        variant="destructive"
        disabled={!canKill || busy}
        onClick={() => setKillOpen(true)}
      >
        <ZapIcon />
        <span className="hidden sm:inline">{t("server.kill")}</span>
      </Button>

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
