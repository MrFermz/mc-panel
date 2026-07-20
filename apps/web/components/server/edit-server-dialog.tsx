"use client";

import * as React from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT, type TranslationKey } from "@/lib/i18n";
import { MemoryPresets } from "@/components/server/memory-presets";
import type { Server } from "@/lib/types";

export interface EditServerBody {
  name?: string;
  memory_mb?: number;
  host_port?: number;
}

interface EditServerDialogProps {
  server: Server | null;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: EditServerBody) => void;
}

// memory/พอร์ตแก้ได้เฉพาะตอนหยุด/error เหมือนหน้า settings (backend ตอบ 409 invalid_state)
function isStoppedLike(status: string): boolean {
  return status === "stopped" || status === "errored";
}

// ฟอร์มแก้ server จากตาราง admin — ส่งเฉพาะ field ที่เปลี่ยนจริง เพื่อไม่ให้ PATCH ไปกระตุ้น
// การเช็ค stopped ของ backend ทั้งที่ user แค่เปลี่ยนชื่อ
export function EditServerDialog({
  server,
  pending = false,
  onOpenChange,
  onSubmit,
}: EditServerDialogProps) {
  const t = useT();
  const [name, setName] = React.useState("");
  const [memoryMb, setMemoryMb] = React.useState("");
  const [hostPort, setHostPort] = React.useState("");

  React.useEffect(() => {
    if (!server) return;
    setName(server.name);
    setMemoryMb(String(server.memory_mb));
    setHostPort(server.host_port === null ? "" : String(server.host_port));
  }, [server]);

  if (!server) return null;

  const canEditRuntime = isStoppedLike(server.status);
  const memory = Number(memoryMb);
  const port = hostPort === "" ? null : Number(hostPort);
  const valid =
    name.trim().length > 0 &&
    name.trim().length <= 100 &&
    Number.isInteger(memory) &&
    memory >= 512 &&
    (port === null ||
      (Number.isInteger(port) && port >= 1024 && port <= 65535));

  const submit = () => {
    const body: EditServerBody = {};
    if (name.trim() !== server.name) body.name = name.trim();
    if (canEditRuntime) {
      if (memory !== server.memory_mb) body.memory_mb = memory;
      // host_port = 0 คือสั่งเลิก expose พอร์ต (ตาม contract ของ PATCH)
      if (port !== server.host_port) body.host_port = port ?? 0;
    }
    onSubmit(body);
  };

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t("adminServers.editTitle", { name: server.name })}
          </DialogTitle>
          <DialogDescription>
            {canEditRuntime
              ? t("adminServers.editDesc")
              : t("adminServers.editStopHint", {
                  // แปลสถานะด้วย ไม่งั้นประโยคไทยจะมีคำอังกฤษโผล่กลางประโยค
                  status: t(`status.${server.status}` as TranslationKey),
                })}
          </DialogDescription>
        </DialogHeader>

        <form
          className="grid gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (valid && !pending) submit();
          }}
        >
          <div className="grid gap-2">
            <Label htmlFor="edit-s-name">{t("sset.name")}</Label>
            <Input
              id="edit-s-name"
              maxLength={100}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="edit-s-memory">{t("sset.memory")}</Label>
            <Input
              id="edit-s-memory"
              type="number"
              min={512}
              disabled={!canEditRuntime}
              value={memoryMb}
              onChange={(e) => setMemoryMb(e.target.value)}
            />
            <MemoryPresets
              value={memoryMb}
              onChange={setMemoryMb}
              disabled={!canEditRuntime}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="edit-s-port">{t("sset.hostPort")}</Label>
            <Input
              id="edit-s-port"
              type="number"
              min={1024}
              max={65535}
              placeholder={t("sset.hostPortPlaceholder")}
              disabled={!canEditRuntime}
              value={hostPort}
              onChange={(e) => setHostPort(e.target.value)}
            />
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!valid || pending}>
              {t("sset.saveChanges")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
