"use client";

import * as React from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiSend, apiSendVoid, ApiError } from "@/lib/api";
import { userResponseSchema } from "@/lib/types";
import { useT } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

// ต้องตรงกับ policy ฝั่ง control-plane (docs/api.md)
const MIN_PASSWORD_LENGTH = 10;

function ChangePasswordForm({
  forced,
  onCancel,
}: {
  forced: boolean;
  onCancel: () => void;
}) {
  const t = useT();
  const queryClient = useQueryClient();
  const [current, setCurrent] = React.useState("");
  const [next, setNext] = React.useState("");
  const [confirm, setConfirm] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  const [pending, setPending] = React.useState(false);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    if (next.length < MIN_PASSWORD_LENGTH) {
      setError(t("changePassword.tooShort", { n: MIN_PASSWORD_LENGTH }));
      return;
    }
    if (next !== confirm) {
      setError(t("changePassword.mismatch"));
      return;
    }
    setPending(true);
    try {
      await apiSend(
        "POST",
        "/api/auth/change-password",
        { current_password: current, new_password: next },
        userResponseSchema,
      );
      if (forced) {
        // reload เต็มเพื่อล้าง gate must_change_password + รับ cookie ใหม่
        window.location.assign("/");
        return;
      }
      toast.success(t("changePassword.success"));
      // token_version ถูก bump — refetch me ด้วย cookie ใหม่
      queryClient.invalidateQueries({ queryKey: ["me"] });
      onCancel();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("common.unreachable"));
      setPending(false);
    }
  };

  const logout = async () => {
    try {
      await apiSendVoid("POST", "/api/auth/logout");
    } finally {
      window.location.assign("/login");
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("changePassword.title")}</DialogTitle>
        <DialogDescription>
          {forced
            ? t("changePassword.subtitle")
            : t("changePassword.voluntarySubtitle")}
        </DialogDescription>
      </DialogHeader>
      <form onSubmit={onSubmit} className="grid gap-4">
        <div className="grid gap-2">
          <Label htmlFor="cp-current">{t("changePassword.current")}</Label>
          <Input
            id="cp-current"
            type="password"
            autoComplete="current-password"
            required
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="cp-new">{t("changePassword.new")}</Label>
          <Input
            id="cp-new"
            type="password"
            autoComplete="new-password"
            required
            minLength={MIN_PASSWORD_LENGTH}
            value={next}
            onChange={(e) => setNext(e.target.value)}
          />
          <p className="text-muted-foreground text-xs">
            {t("changePassword.minChars", { n: MIN_PASSWORD_LENGTH })}
          </p>
        </div>
        <div className="grid gap-2">
          <Label htmlFor="cp-confirm">{t("changePassword.confirm")}</Label>
          <Input
            id="cp-confirm"
            type="password"
            autoComplete="new-password"
            required
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
          />
        </div>
        {error && <p className="text-destructive text-sm">{error}</p>}
        <DialogFooter>
          {forced ? (
            <Button type="button" variant="ghost" onClick={logout}>
              {t("userMenu.logout")}
            </Button>
          ) : (
            <Button
              type="button"
              variant="outline"
              onClick={onCancel}
              disabled={pending}
            >
              {t("common.cancel")}
            </Button>
          )}
          <Button type="submit" disabled={pending}>
            {pending ? t("common.saving") : t("changePassword.submit")}
          </Button>
        </DialogFooter>
      </form>
    </>
  );
}

export function ChangePasswordDialog({
  open,
  onOpenChange,
  forced = false,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  forced?: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={forced ? undefined : onOpenChange}>
      <DialogContent
        // forced: ปิดไม่ได้ (ไม่มี X, esc/click-outside ไม่ทำงาน) — ต้องเปลี่ยนรหัสก่อน
        className={forced ? "[&>button]:hidden sm:max-w-sm" : "sm:max-w-sm"}
        onEscapeKeyDown={forced ? (e) => e.preventDefault() : undefined}
        onInteractOutside={forced ? (e) => e.preventDefault() : undefined}
      >
        <ChangePasswordForm
          forced={forced}
          onCancel={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
