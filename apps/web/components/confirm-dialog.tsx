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
import { useT } from "@/lib/i18n";

interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: React.ReactNode;
  confirmLabel: string;
  destructive?: boolean;
  // ถ้ากำหนด ผู้ใช้ต้องพิมพ์ข้อความนี้เป๊ะ ๆ ก่อนกดยืนยันได้ (ใช้กับ delete server)
  requireText?: string;
  pending?: boolean;
  onConfirm: () => void;
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  destructive = false,
  requireText,
  pending = false,
  onConfirm,
}: ConfirmDialogProps) {
  const t = useT();
  const [typed, setTyped] = React.useState("");

  React.useEffect(() => {
    if (!open) setTyped("");
  }, [open]);

  const blocked = requireText !== undefined && typed !== requireText;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {requireText !== undefined && (
          <div className="grid gap-2">
            <Label htmlFor="confirm-text">
              {t("common.typeToConfirmBefore")}{" "}
              <span className="font-mono font-semibold">{requireText}</span>{" "}
              {t("common.typeToConfirmAfter")}
            </Label>
            <Input
              id="confirm-text"
              value={typed}
              onChange={(e) => setTyped(e.target.value)}
              autoComplete="off"
            />
          </div>
        )}
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={pending}
          >
            {t("common.cancel")}
          </Button>
          <Button
            variant={destructive ? "destructive" : "default"}
            onClick={onConfirm}
            disabled={blocked || pending}
          >
            {pending ? t("common.working") : confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
