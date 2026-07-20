"use client";

import * as React from "react";
import { useT } from "@/lib/i18n";
import type { Capability } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
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

// username: 3-64 ตัว [A-Za-z0-9_.-] — ตรงกับ backend
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

  // ล้างฟอร์มตอนเปิดใหม่ — dialog ถูก mount ค้างไว้ ค่าเดิมจะติดมาถ้าไม่รีเซ็ต
  React.useEffect(() => {
    if (!open) return;
    setUsername("");
    setIsAdmin(false);
    setCaps([]);
  }, [open]);

  const canSubmit = USERNAME_RE.test(username.trim());

  const preview = username.trim();

  const toggleCap = (key: string, on: boolean) =>
    setCaps((prev) =>
      on ? [...new Set([...prev, key])] : prev.filter((k) => k !== key),
    );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90svh] gap-0 overflow-y-auto p-0 sm:max-w-2xl">
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (pending || !canSubmit) return;
            onSubmit({
              username: username.trim(),
              is_admin: isAdmin,
              capabilities: caps,
            });
          }}
        >
          <div className="flex items-center gap-3 px-6 py-5 pr-14">
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
            {/* role ที่จะได้จริงจากสิทธิ์ที่ติ๊กอยู่ — อัปเดตสดตอนแก้ preset/capability */}
            <RoleBadge role={matchPreset(isAdmin, caps)} />
          </div>

          <div className="grid gap-3 border-t px-6 py-4">
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
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>
          </div>

          <div className="border-t px-6 py-4">
            <FieldGroupLabel>{t("users.rolePreset")}</FieldGroupLabel>
            <RolePresetPicker
              isAdmin={isAdmin}
              capabilities={caps}
              onSelect={(next) => {
                setIsAdmin(next.isAdmin);
                setCaps(next.capabilities);
              }}
            />
          </div>

          <div className="border-t px-6 py-4">
            <FieldGroupLabel>{t("users.permissions")}</FieldGroupLabel>
            <PermissionGroups
              catalog={catalog}
              isAdmin={isAdmin}
              capabilities={caps}
              onToggle={toggleCap}
              onToggleGroup={(keys, on) =>
                setCaps((prev) =>
                  on
                    ? [...new Set([...prev, ...keys])]
                    : prev.filter((k) => !keys.includes(k)),
                )
              }
            />
            {isAdmin && (
              <p className="text-muted-foreground mt-2 text-xs">
                {t("users.adminAllPermissions")}
              </p>
            )}
          </div>

          <div className="bg-muted/30 flex justify-end gap-2 border-t px-6 py-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={pending || !canSubmit}>
              {pending ? t("users.creating") : t("users.create.submit")}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
