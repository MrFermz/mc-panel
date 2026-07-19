"use client";

import * as React from "react";
import { ChevronDownIcon } from "lucide-react";
import type { Server, ServerStatus } from "@/lib/types";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

// สีจุดสถานะต่อ server — ใช้ชุดเดียวกับ StatusBadge (running=เขียว, transitional=เหลือง,
// errored=แดง, stopped/อื่น=เทา) เพื่อไม่ให้ความหมายสี drift กัน
const dotClass: Record<ServerStatus, string> = {
  running: "bg-green-400",
  stopped: "bg-zinc-500",
  provisioning: "bg-yellow-400",
  starting: "bg-yellow-400",
  stopping: "bg-yellow-400",
  deleting: "bg-yellow-400",
  errored: "bg-red-400",
};

export function StatusDot({
  status,
  className,
}: {
  status: ServerStatus;
  className?: string;
}) {
  const pulse = status === "starting" || status === "stopping";
  return (
    <span
      className={cn(
        "size-2.5 shrink-0 rounded-full",
        dotClass[status],
        pulse && "animate-pulse",
        className,
      )}
    />
  );
}

// ddl detail แต่ละ server (สถานะ + ชื่อ + type/version) — reuse ทั้ง sidebar quick-switch
// และ header ของ dashboard overview เพื่อไม่ให้ dropdown สอง look ต่างกัน
export function ServerSwitcher({
  servers,
  value,
  onSelect,
  className,
  align = "start",
}: {
  servers: Server[];
  value: string;
  onSelect: (id: string) => void;
  className?: string;
  align?: "start" | "center" | "end";
}) {
  const t = useT();
  const selected = servers.find((s) => s.id === value);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className={cn(
          "border-input bg-input/30 hover:bg-input/50 focus-visible:border-ring focus-visible:ring-ring/50 flex w-full items-center gap-2.5 rounded-md border px-3 py-2 text-left transition-colors outline-none focus-visible:ring-[3px] disabled:cursor-not-allowed disabled:opacity-50 cursor-pointer",
          className,
        )}
      >
        {selected ? (
          <StatusDot status={selected.status} />
        ) : (
          <span className="bg-muted-foreground/50 size-2.5 shrink-0 rounded-full" />
        )}
        <span className="grid min-w-0 flex-1">
          <span className="truncate text-sm font-semibold leading-tight">
            {selected ? selected.name : t("overview.selectServer")}
          </span>
          {selected && (
            <span className="text-muted-foreground truncate text-xs leading-tight capitalize">
              {selected.server_type} {selected.mc_version}
            </span>
          )}
        </span>
        <ChevronDownIcon className="text-muted-foreground size-4 shrink-0" />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align={align}
        className="w-[var(--radix-dropdown-menu-trigger-width)] min-w-56"
      >
        {servers.map((s) => (
          <DropdownMenuItem
            key={s.id}
            onSelect={() => onSelect(s.id)}
            className="gap-2.5 py-2"
          >
            <StatusDot status={s.status} />
            <span className="grid min-w-0 flex-1">
              <span className="truncate text-sm font-semibold leading-tight">
                {s.name}
              </span>
              <span className="text-muted-foreground truncate text-xs leading-tight capitalize">
                {s.server_type} {s.mc_version}
              </span>
            </span>
            {s.id === value && (
              <span className="bg-primary size-2 shrink-0 rounded-full" />
            )}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
