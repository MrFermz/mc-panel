"use client";

import * as React from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ImageUpIcon, Trash2Icon } from "lucide-react";
import { ApiError, deleteAvatar, updateProfile, uploadAvatar } from "@/lib/api";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { userIdent, userTitle } from "@/lib/user-display";
import { detectRole } from "@/lib/user-roles";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { ChangePasswordForm } from "@/components/user/change-password-form";
import { UserAvatar } from "@/components/user/user-avatar";
import { RoleBadge } from "@/components/user/role-badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";

// ต้องตรงกับเพดานฝั่ง control-plane (internal/httpapi/profile_handlers.go)
const MAX_DISPLAY_NAME = 64;
const MAX_AVATAR_BYTES = 512 * 1024;
const AVATAR_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"];

function AvatarCard() {
  const t = useT();
  const queryClient = useQueryClient();
  const { data } = useMe();
  const user = data?.user;
  const inputRef = React.useRef<HTMLInputElement | null>(null);
  const refreshMe = () => queryClient.invalidateQueries({ queryKey: ["me"] });
  const failed = (err: unknown) =>
    toast.error(err instanceof ApiError ? err.message : t("common.unreachable"));

  const upload = useMutation({
    mutationFn: (file: File) => uploadAvatar(file),
    onSuccess: async () => {
      await refreshMe();
      toast.success(t("profile.avatarUpdated"));
    },
    onError: failed,
    onSettled: () => {
      // ล้าง input ไม่งั้นเลือกไฟล์เดิมซ้ำจะไม่ยิง onChange
      if (inputRef.current) inputRef.current.value = "";
    },
  });

  const remove = useMutation({
    mutationFn: () => deleteAvatar(),
    onSuccess: async () => {
      await refreshMe();
      toast.success(t("profile.avatarRemoved"));
    },
    onError: failed,
  });

  // แยกทีละปุ่ม ไม่ใช่ boolean เดียว — ไม่งั้นกดอัปโหลดแล้ว spinner ขึ้นที่ปุ่มลบด้วย
  const pending = upload.isPending || remove.isPending;

  if (!user) return null;

  const onPick = (file: File | undefined) => {
    if (!file) return;
    // เช็คฝั่ง client ก่อนเพื่อบอกผู้ใช้ทันที — ของจริงบังคับซ้ำที่ control-plane เสมอ
    if (!AVATAR_TYPES.includes(file.type)) {
      toast.error(t("profile.avatarType"));
      return;
    }
    if (file.size > MAX_AVATAR_BYTES) {
      toast.error(t("profile.avatarTooLarge", { kb: MAX_AVATAR_BYTES / 1024 }));
      return;
    }
    upload.mutate(file);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("profile.avatar")}</CardTitle>
        <CardDescription>{t("profile.avatarDesc")}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-4">
        <UserAvatar
          seed={user.id}
          name={userTitle(user)}
          src={user.avatar_url}
          className="size-20 rounded-2xl text-2xl"
        />
        <div className="grid gap-2">
          <div className="flex flex-wrap gap-2">
            <input
              ref={inputRef}
              type="file"
              accept={AVATAR_TYPES.join(",")}
              className="hidden"
              onChange={(e) => onPick(e.target.files?.[0])}
            />
            <Button
              type="button"
              variant="outline"
              loading={upload.isPending}
              disabled={pending}
              onClick={() => inputRef.current?.click()}
            >
              {/* ซ่อนไอคอนตอน loading — Button เติม spinner นำหน้า ไม่งั้นได้ 2 ไอคอน */}
              {!upload.isPending && <ImageUpIcon />}
              {t("profile.avatarUpload")}
            </Button>
            {user.avatar_url && (
              <Button
                type="button"
                variant="ghost"
                loading={remove.isPending}
                disabled={pending}
                onClick={() => remove.mutate()}
              >
                {!remove.isPending && <Trash2Icon />}
                {t("profile.avatarRemove")}
              </Button>
            )}
          </div>
          <p className="text-muted-foreground text-xs">
            {t("profile.avatarHint", { kb: MAX_AVATAR_BYTES / 1024 })}
          </p>
        </div>
      </CardContent>
    </Card>
  );
}

function IdentityCard() {
  const t = useT();
  const queryClient = useQueryClient();
  const { data } = useMe();
  const user = data?.user;
  const [displayName, setDisplayName] = React.useState("");

  // sync ค่าเริ่มต้นจาก server ครั้งเดียวต่อค่าที่ server ถือ — ไม่ทับสิ่งที่ผู้ใช้กำลังพิมพ์
  const serverValue = user?.display_name ?? "";
  React.useEffect(() => setDisplayName(serverValue), [serverValue]);

  const save = useMutation({
    mutationFn: () => updateProfile(displayName.trim()),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      // ชื่อโผล่ในลิสต์ access/permission ด้วย — refetch ให้ตรงกันทั้งหน้า
      queryClient.invalidateQueries({ queryKey: ["users"] });
      toast.success(t("profile.saved"));
    },
    onError: (err) =>
      toast.error(
        err instanceof ApiError ? err.message : t("common.unreachable"),
      ),
  });
  const pending = save.isPending;

  if (!user) return null;

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    save.mutate();
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("profile.identity")}</CardTitle>
        <CardDescription>{t("profile.identityDesc")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={onSubmit} className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="profile-display-name">
              {t("profile.displayName")}
            </Label>
            <Input
              id="profile-display-name"
              value={displayName}
              maxLength={MAX_DISPLAY_NAME}
              placeholder={userIdent(user)}
              onChange={(e) => setDisplayName(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              {t("profile.displayNameHint")}
            </p>
          </div>

          {/* อ่านอย่างเดียว — เปลี่ยน username เป็นงานของ admin (หน้า /admin/users) */}
          <div className="grid gap-2">
            <Label>{t("profile.username")}</Label>
            <Input value={user.username} readOnly disabled />
          </div>

          <div className="flex items-center justify-between gap-3">
            <RoleBadge role={detectRole(user)} size="sm" />
            <Button type="submit" loading={pending}>
              {pending ? t("common.saving") : t("common.save")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

// หน้าโปรไฟล์ของตัวเอง — คนละหน้ากับ /preferences (ธีม/ภาษา) และ /admin/users (จัดการคนอื่น)
export default function ProfilePage() {
  const t = useT();
  useSetBreadcrumbs(React.useMemo(() => [{ label: t("profile.title") }], [t]));
  const { data, isPending } = useMe();

  if (isPending || !data) {
    return (
      <div className="grid gap-6">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  return (
    <div className="grid gap-6">
      <div className="grid gap-1">
        <h1 className="text-xl font-semibold">{t("profile.title")}</h1>
        <p className="text-muted-foreground text-sm">{t("profile.subtitle")}</p>
      </div>

      <AvatarCard />
      <IdentityCard />

      <Card>
        <CardHeader>
          <CardTitle>{t("changePassword.title")}</CardTitle>
          <CardDescription>
            {t("changePassword.voluntarySubtitle")}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ChangePasswordForm layout="card" />
        </CardContent>
      </Card>
    </div>
  );
}
