"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { CheckIcon, Loader2Icon, XIcon } from "lucide-react";
import { checkUsername } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import type { Capability } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { UserAvatar } from "@/components/user/user-avatar";
import { RoleBadge } from "@/components/user/role-badge";
import { matchPreset } from "@/lib/user-roles";
import {
  FieldGroupLabel,
  PermissionGroups,
  RolePresetPicker,
} from "@/components/user/permission-fields";

// ตอนกรอกรับทุก case (ไม่แปลงตัวอักษรใต้มือที่กำลังพิมพ์ — ทำแบบนั้น UX แปลก)
// แล้วค่อย lower ตอนเช็คซ้ำ/ส่งจริง ให้ตรงกับ canonicalUsername() ฝั่ง backend
const USERNAME_RE = /^[A-Za-z0-9_.-]{3,64}$/;

export interface CreateUserBody {
  username: string;
  is_admin: boolean;
  capabilities: string[];
}

export function CreateUserDialog({
  open,
  catalog,
  pending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  catalog: Capability[];
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (body: CreateUserBody) => void;
}) {
  const t = useT();
  const [username, setUsername] = React.useState("");
  const [isAdmin, setIsAdmin] = React.useState(false);
  const [caps, setCaps] = React.useState<string[]>([]);
  const [debounced, setDebounced] = React.useState("");

  // ล้างฟอร์มตอนเปิดใหม่ — dialog ถูก mount ค้างไว้ ค่าเดิมจะติดมาถ้าไม่รีเซ็ต
  React.useEffect(() => {
    if (!open) return;
    setUsername("");
    setIsAdmin(false);
    setCaps([]);
    setDebounced("");
  }, [open]);

  const preview = username.trim();
  const formatOk = USERNAME_RE.test(preview);
  // ชื่อที่จะถูกบันทึกจริง — เช็คซ้ำและส่ง create ด้วยค่านี้เสมอ
  const canonical = preview.toLowerCase();

  // debounce 400ms — ไม่ยิงเช็คทุก keystroke
  React.useEffect(() => {
    const id = setTimeout(() => setDebounced(canonical), 400);
    return () => clearTimeout(id);
  }, [canonical]);

  const check = useQuery({
    queryKey: ["users", "check-username", debounced],
    queryFn: () => checkUsername(debounced),
    enabled: open && debounced !== "" && USERNAME_RE.test(debounced),
    staleTime: 30_000,
    retry: false,
  });

  // ผลที่เชื่อได้ต้องเป็นของ "ชื่อที่พิมพ์อยู่ตอนนี้" เท่านั้น — ระหว่างรอ debounce
  // หรือระหว่าง fetch ให้ถือว่ายังไม่รู้ผล ไม่งั้นจะโชว์ผลของชื่อก่อนหน้า
  const settled = canonical === debounced && !check.isFetching;
  const result = settled && formatOk ? check.data : undefined;
  const checking = formatOk && !settled;

  // เช็คไม่สำเร็จ (เน็ตหลุด/พังชั่วคราว) = ไม่บล็อกการสร้าง — backend เป็นด่านจริงอยู่แล้ว
  const canSubmit = formatOk && (check.isError || result?.available !== false);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <form
          className="contents"
          onSubmit={(e) => {
            e.preventDefault();
            if (pending || !canSubmit) return;
            onSubmit({
              // ส่งชื่อที่ lower แล้วเสมอ (backend canonical ซ้ำอีกชั้นอยู่ดี)
              username: canonical,
              is_admin: isAdmin,
              capabilities: caps,
            });
          }}
        >
          <DialogHeader className="flex-row items-center gap-3">
            <UserAvatar
              seed={preview}
              name={preview || "?"}
              className="size-11 rounded-xl text-base"
            />
            <div className="mr-auto grid gap-0.5">
              <DialogTitle className="text-base">
                {preview || t("users.createTitle")}
              </DialogTitle>
              <DialogDescription className="text-xs">
                {t("users.usernameHint")}
              </DialogDescription>
            </div>
            {/* role ที่จะได้จริง — เปลี่ยนทันทีที่เลือก preset */}
            <RoleBadge role={matchPreset(isAdmin, caps)} />
          </DialogHeader>

          <DialogBody className="gap-6">
            <div className="grid gap-3">
              <FieldGroupLabel>{t("users.account")}</FieldGroupLabel>
              <div className="grid gap-2">
                <Label htmlFor="u-username">{t("users.username")}</Label>
                <Input
                  id="u-username"
                  required
                  minLength={3}
                  maxLength={64}
                  pattern="[a-zA-Z0-9_.\-]+"
                  autoComplete="off"
                  autoCapitalize="none"
                  spellCheck={false}
                  aria-invalid={result?.available === false || undefined}
                  aria-describedby="u-username-status"
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                />
                {/* สถานะชื่อซ้ำแบบสด — aria-live ให้ screen reader ได้ยินตอนผลเปลี่ยน */}
                <p
                  id="u-username-status"
                  aria-live="polite"
                  className={cn(
                    "flex items-center gap-1.5 text-xs",
                    result?.available === false
                      ? "text-destructive"
                      : result?.available
                        ? "text-emerald-500"
                        : "text-muted-foreground",
                  )}
                >
                  {preview === "" || !formatOk ? (
                    t("users.usernameFormatHint")
                  ) : checking || check.isFetching ? (
                    <>
                      <Loader2Icon className="size-3 animate-spin" />
                      {t("users.usernameChecking")}
                    </>
                  ) : result?.available ? (
                    <>
                      <CheckIcon className="size-3" />
                      {/* พิมพ์ตัวใหญ่มาได้ แต่ต้องรู้ตัวว่าจะถูกเก็บเป็นพิมพ์เล็ก */}
                      {preview === canonical
                        ? t("users.usernameFree")
                        : t("users.usernameFreeAs", { name: canonical })}
                    </>
                  ) : result?.reason === "reserved" ? (
                    <>
                      <XIcon className="size-3" />
                      {t("users.usernameReservedHint")}
                    </>
                  ) : result?.reason === "taken" ? (
                    <>
                      <XIcon className="size-3" />
                      {t("users.usernameTakenHint")}
                    </>
                  ) : (
                    t("users.usernameFormatHint")
                  )}
                </p>
              </div>
            </div>

            <div>
              <FieldGroupLabel>{t("users.rolePreset")}</FieldGroupLabel>
              <RolePresetPicker
                isAdmin={isAdmin}
                capabilities={caps}
                onSelect={(next) => {
                  setIsAdmin(next.isAdmin);
                  setCaps(next.capabilities);
                }}
              />
              <p className="text-muted-foreground mt-2 text-xs">
                {t("users.presetHint")}
              </p>
            </div>

            <div>
              <FieldGroupLabel>{t("users.permissions")}</FieldGroupLabel>
              <PermissionGroups
                catalog={catalog}
                isAdmin={isAdmin}
                capabilities={caps}
              />
              {isAdmin && (
                <p className="text-muted-foreground mt-2 text-xs">
                  {t("users.adminAllPermissions")}
                </p>
              )}
            </div>
          </DialogBody>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" loading={pending} disabled={!canSubmit}>
              {pending ? t("users.creating") : t("users.create.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
