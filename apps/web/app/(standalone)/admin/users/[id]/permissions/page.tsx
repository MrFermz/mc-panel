"use client";

import * as React from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeftIcon } from "lucide-react";
import { apiGet, apiSend, ApiError } from "@/lib/api";
import {
  capabilitiesResponseSchema,
  userResponseSchema,
  type User,
} from "@/lib/types";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { matchPreset } from "@/lib/user-roles";
import { userIdent, userTitle } from "@/lib/user-display";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { UserIdentity } from "@/components/user/user-identity";
import { UserDetailTabs } from "@/components/user/user-detail-tabs";
import {
  FieldGroupLabel,
  PermissionGroups,
  RolePresetPicker,
} from "@/components/user/permission-fields";

export default function UserPermissionsPage() {
  const t = useT();
  const router = useRouter();
  const params = useParams<{ id: string }>();
  const userId = params.id;
  const queryClient = useQueryClient();

  const { data: meData } = useMe();
  const me = meData?.user;
  const canView = hasCapability(me, CAPABILITY.usersView);
  const canEdit = hasCapability(me, CAPABILITY.usersEdit);

  useSetBreadcrumbs(
    React.useMemo(
      () => [
        { label: t("nav.admin") },
        { label: t("users.title"), href: "/admin/users" },
        { label: t("users.permissions") },
      ],
      [t],
    ),
  );

  const user = useQuery({
    queryKey: ["users", userId],
    queryFn: () => apiGet(`/api/users/${userId}`, userResponseSchema),
    enabled: canView,
  });

  const caps = useQuery({
    queryKey: ["meta", "capabilities"],
    queryFn: () => apiGet("/api/meta/capabilities", capabilitiesResponseSchema),
    enabled: canView,
    staleTime: 300_000,
  });
  const catalog = React.useMemo(
    () => caps.data?.capabilities ?? [],
    [caps.data],
  );

  const [isAdmin, setIsAdmin] = React.useState(false);
  const [selected, setSelected] = React.useState<string[]>([]);
  const loaded = user.data?.user;

  // seed ฟอร์มจากค่าใน DB ครั้งแรกที่โหลดเสร็จ (และตอน refetch หลังบันทึก)
  React.useEffect(() => {
    if (!loaded) return;
    setIsAdmin(loaded.is_admin);
    setSelected(loaded.capabilities);
  }, [loaded]);

  const save = useMutation({
    mutationFn: (body: { is_admin: boolean; capabilities: string[] }) =>
      apiSend("PATCH", `/api/users/${userId}`, body, userResponseSchema),
    onSuccess: (data) => {
      toast.success(t("users.capsUpdated"));
      queryClient.setQueryData(["users", userId], data);
      queryClient.invalidateQueries({ queryKey: ["users"] });
      // สิทธิ์ของตัวเองเปลี่ยน = เมนู/ปุ่มต้องอัปเดตตาม
      queryClient.invalidateQueries({ queryKey: ["me"] });
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : t("users.failedCaps"));
    },
  });

  if (me && !canView) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>
    );
  }

  if (user.isPending || caps.isPending) {
    return <Skeleton className="h-96 w-full" />;
  }
  if (user.isError || !loaded) {
    return <p className="text-destructive text-sm">{t("users.failedLoad")}</p>;
  }

  const self = loaded.id === me?.id;
  // ห้ามแก้สิทธิ์ตัวเอง (กันล็อกตัวเองออกจากระบบ) และต้องมี users.edit
  const locked = self || !canEdit;
  const roleKey = matchPreset(isAdmin, selected);
  const dirty =
    isAdmin !== loaded.is_admin ||
    selected.length !== loaded.capabilities.length ||
    selected.some((k) => !loaded.capabilities.includes(k));

  const reset = (u: User) => {
    setIsAdmin(u.is_admin);
    setSelected(u.capabilities);
  };

  return (
    <div className="grid gap-4 pb-20">
      <div>
        <Button variant="ghost" size="sm" className="-ml-2" asChild>
          <Link href="/admin/users">
            <ArrowLeftIcon />
            {t("users.backToUsers")}
          </Link>
        </Button>
      </div>

      <Card>
        <CardContent className="flex flex-wrap items-center gap-3">
          <UserIdentity
            user={loaded}
            panelRole={roleKey}
            size="lg"
            className="mr-auto"
            trailing={
              self && (
                <span className="text-muted-foreground text-sm font-normal">
                  ({t("users.you")})
                </span>
              )
            }
          />
          {/* account ที่มีแต่ username จะได้ ident เท่ากับชื่อที่โชว์อยู่แล้ว — ไม่ต้องซ้ำ */}
          {userIdent(loaded) !== userTitle(loaded) && (
            <span className="text-muted-foreground text-xs">
              {userIdent(loaded)}
            </span>
          )}
        </CardContent>
      </Card>

      <UserDetailTabs userId={userId} />

      {locked && (
        <p className="text-muted-foreground text-sm">
          {self ? t("users.selfRoleLocked") : t("users.needUsersEdit")}
        </p>
      )}

      <Card>
        <CardContent className="grid gap-4">
          <div>
            <FieldGroupLabel>{t("users.rolePreset")}</FieldGroupLabel>
            <RolePresetPicker
              isAdmin={isAdmin}
              capabilities={selected}
              disabled={locked}
              onSelect={(next) => {
                setIsAdmin(next.isAdmin);
                setSelected(next.capabilities);
              }}
            />
            <p className="text-muted-foreground mt-2 text-xs">
              {isAdmin ? t("users.adminAllPermissions") : t("users.presetHint")}
            </p>
          </div>

          <div>
            <FieldGroupLabel>{t("users.permissions")}</FieldGroupLabel>
            <PermissionGroups
              catalog={catalog}
              isAdmin={isAdmin}
              capabilities={selected}
            />
          </div>
        </CardContent>
      </Card>

      {/* action bar ลอยล่างจอ — รายการสิทธิ์ยาวกว่าหนึ่งหน้าจอ ปุ่มบันทึกต้องเอื้อมถึงเสมอ */}
      <div className="bg-background/95 fixed inset-x-0 bottom-0 z-30 border-t backdrop-blur">
        <div className="mx-auto flex max-w-5xl items-center justify-end gap-2 px-4 py-3">
          <span className="text-muted-foreground mr-auto text-xs">
            {dirty ? t("users.unsavedChanges") : ""}
          </span>
          <Button
            variant="outline"
            disabled={!dirty || save.isPending}
            onClick={() => {
              reset(loaded);
              router.refresh();
            }}
          >
            {t("common.cancel")}
          </Button>
          <Button
            disabled={locked || !dirty || save.isPending}
            onClick={() =>
              save.mutate({ is_admin: isAdmin, capabilities: selected })
            }
          >
            {save.isPending ? t("common.saving") : t("common.save")}
          </Button>
        </div>
      </div>
    </div>
  );
}
