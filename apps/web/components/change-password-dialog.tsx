"use client";

import { useT } from "@/lib/i18n";
import { ChangePasswordForm } from "@/components/user/change-password-form";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

// dialog นี้เหลือไว้สำหรับ forced flow (must_change_password) เป็นหลัก —
// การเปลี่ยนรหัสแบบสมัครใจย้ายไปเป็นการ์ดในหน้า /profile แล้ว
export function ChangePasswordDialog({
  open,
  onOpenChange,
  forced = false,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  forced?: boolean;
}) {
  const t = useT();
  return (
    <Dialog open={open} onOpenChange={forced ? undefined : onOpenChange}>
      <DialogContent
        // forced: ปิดไม่ได้ (ไม่มี X, esc/click-outside ไม่ทำงาน) — ต้องเปลี่ยนรหัสก่อน
        className={forced ? "[&>button]:hidden sm:max-w-sm" : "sm:max-w-sm"}
        onEscapeKeyDown={forced ? (e) => e.preventDefault() : undefined}
        onInteractOutside={forced ? (e) => e.preventDefault() : undefined}
      >
        <DialogHeader>
          <DialogTitle>{t("changePassword.title")}</DialogTitle>
          <DialogDescription>
            {forced
              ? t("changePassword.subtitle")
              : t("changePassword.voluntarySubtitle")}
          </DialogDescription>
        </DialogHeader>
        <ChangePasswordForm
          layout="dialog"
          forced={forced}
          onCancel={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
