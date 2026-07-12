"use client";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { ServerStatus } from "@/lib/types";
import { useT, type TranslationKey } from "@/lib/i18n";

const statusClasses: Record<ServerStatus, string> = {
  running: "bg-green-500/15 text-green-400 border-green-500/30",
  stopped: "bg-zinc-500/15 text-zinc-400 border-zinc-500/30",
  provisioning: "bg-yellow-500/15 text-yellow-400 border-yellow-500/30",
  starting: "bg-yellow-500/15 text-yellow-400 border-yellow-500/30",
  stopping: "bg-yellow-500/15 text-yellow-400 border-yellow-500/30",
  deleting: "bg-yellow-500/15 text-yellow-400 border-yellow-500/30",
  errored: "bg-red-500/15 text-red-400 border-red-500/30",
};

export function StatusBadge({
  status,
  className,
}: {
  status: ServerStatus;
  className?: string;
}) {
  const t = useT();
  return (
    <Badge
      variant="outline"
      className={cn("capitalize", statusClasses[status], className)}
    >
      {t(`status.${status}` as TranslationKey)}
    </Badge>
  );
}
