"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { apiGet, apiSendVoid, listUserDirectory, ApiError } from "@/lib/api";
import {
  capabilitiesResponseSchema,
  permissionsResponseSchema,
  type Permission,
} from "@/lib/types";
import { SERVER_SCOPED_CAPABILITIES } from "@/lib/capabilities";
import {
  SERVER_ROLE_LABEL_KEYS,
  SERVER_ROLE_PRESETS,
  matchServerPreset,
} from "@/lib/server-roles";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { userIdent, userTitle } from "@/lib/user-display";
import { UserIdentity } from "@/components/user/user-identity";
import { UserPicker } from "@/components/user/user-picker";
import {
  FieldGroupLabel,
  PermissionGroups,
} from "@/components/user/permission-fields";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

interface FormState {
  // เลือกจาก directory เท่านั้น ("" = ยังไม่ได้เลือก) — ไม่มีช่องพิมพ์ username แล้ว
  // เพราะพิมพ์ผิดจะรู้ตอน submit ทีเดียว ส่วน picker กรองจากคนที่มีจริงอยู่แล้ว
  userId: string;
  role: "owner" | "member";
  capabilities: string[];
}

const emptyForm: FormState = {
  userId: "",
  role: "member",
  capabilities: SERVER_ROLE_PRESETS.find((p) => p.key === "viewer")!
    .capabilities,
};

// preset picker ต่อ server — เทียบชุด cap ปัจจุบันกับ preset แล้วไฮไลต์ตัวที่ตรง
function ServerPresetPicker({
  role,
  capabilities,
  onSelect,
}: {
  role: "owner" | "member";
  capabilities: string[];
  onSelect: (next: {
    role: "owner" | "member";
    capabilities: string[];
  }) => void;
}) {
  const t = useT();
  const selected = matchServerPreset(role, capabilities);
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
      {SERVER_ROLE_PRESETS.map((preset) => {
        const active = selected === preset.key;
        return (
          <button
            key={preset.key}
            type="button"
            onClick={() =>
              onSelect({ role: preset.role, capabilities: preset.capabilities })
            }
            className={cn(
              "h-10 rounded-md border text-sm font-medium transition-colors",
              active
                ? "border-primary bg-primary/15 text-primary"
                : "hover:bg-accent hover:text-accent-foreground",
            )}
          >
            {t(SERVER_ROLE_LABEL_KEYS[preset.key])}
          </button>
        );
      })}
    </div>
  );
}

// สองโหมด: live (มี serverId — ยิง REST ทันที) กับ draft (wizard สร้าง server ที่ instance
// ยังไม่มี — เก็บใน state แล้ว apply หลังสร้างเสร็จ). ทั้งสองโหมดเลือก user จาก directory
// เท่านั้น (backend ยังรับ `username` อยู่ แต่ UI ไม่ใช้แล้ว)
export default function ServerAccess({
  serverId,
  draft,
  onDraftChange,
  lockedUserId,
}: {
  serverId?: string;
  draft?: Permission[];
  onDraftChange?: (next: Permission[]) => void;
  // แถวที่แก้/ลบไม่ได้ — ใช้กับคนสร้าง server ใน wizard ซึ่ง backend ตั้งเป็น owner
  // ให้เองตอน CreateServerWithOwner อยู่แล้ว (ปุ่มที่กดแล้วไม่มีผลจริง = หลอก user)
  lockedUserId?: string;
}) {
  const t = useT();
  const queryClient = useQueryClient();
  const live = serverId !== undefined;
  const [dialogOpen, setDialogOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<Permission | null>(null);
  const [form, setForm] = React.useState<FormState>(emptyForm);
  const [removeTarget, setRemoveTarget] = React.useState<Permission | null>(
    null,
  );

  const me = useMe();

  const permissions = useQuery({
    queryKey: ["servers", serverId, "permissions"],
    queryFn: () =>
      apiGet(`/api/servers/${serverId}/permissions`, permissionsResponseSchema),
    enabled: live,
  });

  const rows = React.useMemo(
    () => (live ? (permissions.data?.permissions ?? []) : (draft ?? [])),
    [live, permissions.data, draft],
  );

  const directory = useQuery({
    queryKey: ["users", "directory"],
    queryFn: () => listUserDirectory(),
    staleTime: 30_000,
    retry: false,
  });

  // catalog ทั้งหมดจาก backend แล้ว filter เหลือเฉพาะ server-scoped cap (ชั้น access)
  const caps = useQuery({
    queryKey: ["meta", "capabilities"],
    queryFn: () => apiGet("/api/meta/capabilities", capabilitiesResponseSchema),
    staleTime: 300_000,
  });
  const serverCatalog = React.useMemo(
    () =>
      (caps.data?.capabilities ?? []).filter((c) =>
        (SERVER_SCOPED_CAPABILITIES as readonly string[]).includes(c.key),
      ),
    [caps.data],
  );

  // user ที่เลือกได้: active ทั้งหมด ตัดตัวเอง + คนที่มีสิทธิ์อยู่แล้วออก
  const pickableUsers = React.useMemo(() => {
    const already = new Set(rows.map((p) => p.user_id));
    const meId = me.data?.user.id;
    return (directory.data?.users ?? []).filter(
      (u) => u.id !== meId && !already.has(u.id),
    );
  }, [directory.data, rows, me.data]);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["servers", serverId] });
  };

  // โหมด draft: upsert/remove ลงใน array ที่ parent ถืออยู่ (key ด้วย user_id)
  const applyDraft = (payload: FormState) => {
    const picked = (directory.data?.users ?? []).find(
      (u) => u.id === payload.userId,
    );
    if (!picked) return;
    const entry: Permission = {
      user_id: picked.id,
      username: picked.username,
      display_name: picked.display_name ?? "",
      avatar_url: picked.avatar_url ?? null,
      role: payload.role,
      capabilities: payload.role === "owner" ? [] : payload.capabilities,
    };
    const current = draft ?? [];
    const idx = current.findIndex((p) => p.user_id === entry.user_id);
    onDraftChange?.(
      idx >= 0
        ? current.map((p, i) => (i === idx ? entry : p))
        : [...current, entry],
    );
    toast.success(editing ? t("access.updated") : t("access.added"));
    setDialogOpen(false);
  };

  const upsert = useMutation({
    mutationFn: (payload: FormState) => {
      // owner ได้ทุก cap โดยปริยาย — ไม่ต้องส่ง capabilities
      const base = {
        role: payload.role,
        capabilities: payload.role === "owner" ? [] : payload.capabilities,
      };
      return apiSendVoid("POST", `/api/servers/${serverId}/permissions`, {
        ...base,
        user_id: payload.userId,
      });
    },
    onSuccess: () => {
      toast.success(editing ? t("access.updated") : t("access.added"));
      setDialogOpen(false);
      invalidate();
    },
    onError: (err) => {
      if (err instanceof ApiError && err.code === "user_not_found") {
        toast.error(t("access.userNotFound"));
      } else {
        toast.error(
          err instanceof ApiError ? err.message : t("access.failedSave"),
        );
      }
    },
  });

  const remove = useMutation({
    mutationFn: (userId: string) =>
      apiSendVoid("DELETE", `/api/servers/${serverId}/permissions/${userId}`),
    onSuccess: () => {
      toast.success(t("access.removed"));
      setRemoveTarget(null);
      invalidate();
    },
    onError: (err) => {
      toast.error(
        err instanceof ApiError ? err.message : t("access.failedRemove"),
      );
    },
  });

  const openAdd = () => {
    setEditing(null);
    setForm(emptyForm);
    setDialogOpen(true);
  };

  const openEdit = (perm: Permission) => {
    setEditing(perm);
    setForm({
      userId: perm.user_id,
      role: perm.role,
      capabilities: perm.capabilities,
    });
    setDialogOpen(true);
  };

  // ต้องเลือก user ก่อนเสมอ (ตอนแก้ไขมี user ติดมาแล้ว)
  const canSubmit = form.userId !== "";

  const submit = () => {
    if (!canSubmit) return;
    if (!live) {
      applyDraft(form);
      return;
    }
    if (!upsert.isPending) upsert.mutate(form);
  };

  const confirmRemove = () => {
    if (!removeTarget) return;
    if (!live) {
      onDraftChange?.(
        (draft ?? []).filter((p) => p.user_id !== removeTarget.user_id),
      );
      setRemoveTarget(null);
      toast.success(t("access.removed"));
      return;
    }
    remove.mutate(removeTarget.user_id);
  };

  return (
    <div className="grid gap-4">
      <div className="flex items-center justify-between">
        <p className="text-muted-foreground text-sm">{t("access.subtitle")}</p>
        <Button size="sm" onClick={openAdd}>
          <PlusIcon />
          {t("access.addUser")}
        </Button>
      </div>

      {live && permissions.isPending ? (
        <Skeleton className="h-32 w-full" />
      ) : live && permissions.isError ? (
        <p className="text-destructive text-sm">{t("access.failedLoad")}</p>
      ) : (
        <Card className="py-0">
          <CardContent className="overflow-x-auto px-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("access.user")}</TableHead>
                  <TableHead>{t("access.accessLevel")}</TableHead>
                  <TableHead className="w-24" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((perm) => {
                  const preset = matchServerPreset(
                    perm.role,
                    perm.capabilities,
                  );
                  return (
                    <TableRow key={perm.user_id}>
                      <TableCell>
                        <UserIdentity
                          user={{ id: perm.user_id, ...perm }}
                          serverRole={perm.role}
                          size="sm"
                        />
                      </TableCell>
                      <TableCell>
                        <span className="text-sm">
                          {t(SERVER_ROLE_LABEL_KEYS[preset])}
                          {perm.role === "member" && (
                            <span className="text-muted-foreground">
                              {" · "}
                              {t("access.capsCount", {
                                count: perm.capabilities.length,
                              })}
                            </span>
                          )}
                        </span>
                      </TableCell>
                      <TableCell>
                        {perm.user_id === lockedUserId ? (
                          <span className="text-muted-foreground flex justify-end text-xs">
                            {t("access.you")}
                          </span>
                        ) : (
                          <div className="flex justify-end gap-1">
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => openEdit(perm)}
                              aria-label={`${t("common.edit")} ${userTitle(perm)}`}
                            >
                              <PencilIcon />
                            </Button>
                            <Button
                              variant="ghost"
                              size="icon"
                              className="text-destructive"
                              onClick={() => setRemoveTarget(perm)}
                              aria-label={`${t("common.remove")} ${userTitle(perm)}`}
                            >
                              <Trash2Icon />
                            </Button>
                          </div>
                        )}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {editing
                ? t("access.editTitle", { name: userTitle(editing) })
                : t("access.addUser")}
            </DialogTitle>
            {!editing && (
              <DialogDescription>
                {t("access.accountRequired")}
              </DialogDescription>
            )}
          </DialogHeader>
          <form
            className="contents"
            onSubmit={(e) => {
              e.preventDefault();
              submit();
            }}
          >
            <DialogBody>
              {editing ? (
                // ตอนแก้ไข user เปลี่ยนไม่ได้ — โชว์ว่ากำลังแก้ของใครอยู่แทนช่องเลือก
                <div className="bg-muted/40 flex items-center justify-between gap-3 rounded-md border p-3">
                  <UserIdentity user={{ ...editing, id: editing.user_id }} />
                  <span className="text-muted-foreground shrink-0 text-xs">
                    {userIdent(editing)}
                  </span>
                </div>
              ) : (
                <div className="grid gap-2">
                  <Label htmlFor="perm-user">{t("access.pickUser")}</Label>
                  <UserPicker
                    id="perm-user"
                    users={pickableUsers}
                    value={form.userId}
                    onSelect={(userId) => setForm({ ...form, userId })}
                    disabled={directory.isPending}
                    placeholder={
                      directory.isPending
                        ? t("common.loading")
                        : t("access.pickUserPlaceholder")
                    }
                  />
                </div>
              )}

              <div>
                <FieldGroupLabel>{t("access.rolePreset")}</FieldGroupLabel>
                <ServerPresetPicker
                  role={form.role}
                  capabilities={form.capabilities}
                  onSelect={(next) => setForm({ ...form, ...next })}
                />
                <p className="text-muted-foreground mt-2 text-xs">
                  {form.role === "owner"
                    ? t("access.ownerHint")
                    : t("access.presetHint")}
                </p>
              </div>

              <div>
                <FieldGroupLabel>{t("access.permissions")}</FieldGroupLabel>
                {caps.isPending ? (
                  <Skeleton className="h-64 w-full" />
                ) : (
                  <PermissionGroups
                    catalog={serverCatalog}
                    // owner ได้ทุก server-scoped cap โดยปริยาย — โชว์ติ๊กครบทุกข้อ
                    isAdmin={form.role === "owner"}
                    capabilities={form.capabilities}
                  />
                )}
              </div>
            </DialogBody>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialogOpen(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                loading={upsert.isPending}
                disabled={!canSubmit}
              >
                {upsert.isPending ? t("common.saving") : t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRemoveTarget(null);
        }}
        title={t("access.removeTitle")}
        description={
          removeTarget
            ? t("access.removeDesc", { name: userTitle(removeTarget) })
            : ""
        }
        confirmLabel={t("common.remove")}
        destructive
        pending={remove.isPending}
        onConfirm={confirmRemove}
      />
    </div>
  );
}
