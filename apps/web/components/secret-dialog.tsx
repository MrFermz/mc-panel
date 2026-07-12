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
import { CopyButton } from "@/components/copy-button";
import { useT } from "@/lib/i18n";

// dialog แสดง secret ที่ server เจนให้ครั้งเดียว (initial password / node token)
export function SecretDialog({
  open,
  onOpenChange,
  title,
  description,
  secret,
  extra,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: React.ReactNode;
  secret: string;
  extra?: React.ReactNode;
}) {
  const t = useT();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="flex items-center gap-2">
          <Input
            readOnly
            value={secret}
            className="font-mono text-xs"
            onFocus={(e) => e.currentTarget.select()}
          />
          <CopyButton value={secret} />
        </div>
        <p className="text-destructive text-sm font-medium">
          {t("secret.onceOnly")}
        </p>
        {extra}
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t("common.done")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
