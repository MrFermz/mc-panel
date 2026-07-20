"use client";

import * as React from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { apiSend, apiSendVoid, ApiError } from "@/lib/api";
import { userResponseSchema } from "@/lib/types";
import { useT } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DialogBody, DialogFooter } from "@/components/ui/dialog";

// ต้องตรงกับ policy ฝั่ง control-plane (docs/api.md)
export const MIN_PASSWORD_LENGTH = 10;

// ฟอร์มเปลี่ยนรหัสผ่านตัวเดียวที่ใช้ทั้งใน dialog (forced flow ตอน must_change_password)
// และเป็นการ์ดในหน้า /profile — ต่างกันแค่แถวปุ่มด้านล่าง
export function ChangePasswordForm({
  layout,
  forced = false,
  onCancel,
}: {
  layout: "dialog" | "card";
  forced?: boolean;
  onCancel?: () => void;
}) {
  const t = useT();
  const queryClient = useQueryClient();
  const [current, setCurrent] = React.useState("");
  const [next, setNext] = React.useState("");
  const [confirm, setConfirm] = React.useState("");
  const [error, setError] = React.useState<string | null>(null);
  // forced flow reload ทั้งหน้า — ค้าง pending ไว้ไม่ให้ปุ่มกลับมากดได้ระหว่างนั้น
  const [navigating, setNavigating] = React.useState(false);

  const change = useMutation({
    mutationFn: () =>
      apiSend(
        "POST",
        "/api/auth/change-password",
        { current_password: current, new_password: next },
        userResponseSchema,
      ),
    onSuccess: () => {
      if (forced) {
        setNavigating(true);
        // reload เต็มเพื่อล้าง gate must_change_password + รับ cookie ใหม่
        window.location.assign("/");
        return;
      }
      toast.success(t("changePassword.success"));
      // token_version ถูก bump — refetch me ด้วย cookie ใหม่
      queryClient.invalidateQueries({ queryKey: ["me"] });
      setCurrent("");
      setNext("");
      setConfirm("");
      onCancel?.();
    },
    onError: (err) => {
      setError(err instanceof ApiError ? err.message : t("common.unreachable"));
    },
  });

  const pending = change.isPending || navigating;

  const onSubmit = (e: React.FormEvent) => {
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
    change.mutate();
  };

  const logout = async () => {
    try {
      await apiSendVoid("POST", "/api/auth/logout");
    } finally {
      window.location.assign("/login");
    }
  };

  const submit = (
    <Button type="submit" loading={pending}>
      {pending ? t("common.saving") : t("changePassword.submit")}
    </Button>
  );

  const fields = (
    <>
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
    </>
  );

  // ใน dialog: form เป็น `contents` เพื่อให้ DialogBody/DialogFooter เป็นลูกของ
  // DialogContent โดยตรง — header/footer จึงตรึงอยู่ได้ และ body เลื่อนแยก
  if (layout === "dialog") {
    return (
      <form onSubmit={onSubmit} className="contents">
        <DialogBody>{fields}</DialogBody>
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
          {submit}
        </DialogFooter>
      </form>
    );
  }

  return (
    <form onSubmit={onSubmit} className="grid gap-4">
      {fields}
      <div className="flex justify-end">{submit}</div>
    </form>
  );
}
