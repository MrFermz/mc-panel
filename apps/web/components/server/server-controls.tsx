"use client";

import * as React from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { PlayIcon, SquareIcon } from "lucide-react";
import { apiSend, ApiError } from "@/lib/api";
import { jobResponseSchema, type Server } from "@/lib/types";
import { useT, type TranslationKey } from "@/lib/i18n";
import { Button } from "@/components/ui/button";

// กันไม่ให้คลิกปุ่มทะลุไป navigate การ์ด/แถว
function stop(e: React.SyntheticEvent) {
  e.stopPropagation();
}

export function ServerControls({ server }: { server: Server }) {
  const t = useT();
  const queryClient = useQueryClient();

  const action = useMutation({
    mutationFn: (a: "start" | "stop") =>
      apiSend(
        "POST",
        `/api/servers/${server.id}/actions`,
        { action: a },
        jobResponseSchema,
      ),
    onSuccess: (_data, a) => {
      toast.success(
        t("dashboard.requested", {
          action: t(`server.action.${a}` as TranslationKey),
          name: server.name,
        }),
      );
      queryClient.invalidateQueries({ queryKey: ["servers"] });
    },
    onError: (err) => {
      toast.error(
        err instanceof ApiError ? err.message : t("dashboard.actionFailed"),
      );
    },
  });

  const canStart = server.status === "stopped" || server.status === "errored";
  const canStop = server.status === "running" || server.status === "starting";
  const busy =
    action.isPending ||
    server.status === "provisioning" ||
    server.status === "stopping" ||
    server.status === "deleting";

  return (
    <div className="flex items-center gap-1.5" onClick={stop}>
      {canStart && (
        <Button
          size="sm"
          variant="outline"
          disabled={busy}
          onClick={(e) => {
            stop(e);
            action.mutate("start");
          }}
        >
          <PlayIcon />
          {t("server.start")}
        </Button>
      )}
      {canStop && (
        <Button
          size="sm"
          variant="outline"
          disabled={busy}
          onClick={(e) => {
            stop(e);
            action.mutate("stop");
          }}
        >
          <SquareIcon />
          {t("server.stop")}
        </Button>
      )}
    </div>
  );
}
