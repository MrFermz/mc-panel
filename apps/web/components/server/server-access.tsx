"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import {
  apiGet,
  apiSendVoid,
  listUserDirectory,
  ApiError,
} from "@/lib/api";
import {
  capabilitiesResponseSchema,
  permissionsResponseSchema,
  type DirectoryUser,
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
import {
  FieldGroupLabel,
  PermissionGroups,
} from "@/components/user/permission-fields";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent } from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
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
  // userId !== "" → ส่ง user_id (เลือกจาก dropdown); "" → ใช้ free-text email แทน
  userId: string;
  email: string;
  role: "owner" | "member";
  capabilities: string[];
}

const emptyForm: FormState = {
  userId: "",
  email: "",
  role: "member",
  capabilities: SERVER_ROLE_PRESETS.find((p) => p.key === "viewer")!
    .capabilities,
};

// label ของ user ใน dropdown — username + email ถ้ามีทั้งคู่
function directoryLabel(u: DirectoryUser): string {
  const primary = userTitle(u);
  const secondary = userIdent(u);
  return secondary !== primary ? `${primary} (${secondary})` : primary;
}

// preset picker ต่อ server — เทียบชุด cap ปัจจุบันกับ preset แล้วไฮไลต์ตัวที่ตรง
function ServerPresetPicker({
  role,
  capabilities,
  onSelect,
}: {
  role: "owner" | "member";
  capabilities: string[];
  onSelect: (next: { role: "owner" | "member"; capabilities: string[] }) => void;
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

export default function ServerAccess({ serverId }: { serverId: string }) {
  const t = useT();
  const queryClient = useQueryClient();
  const [dialogOpen, setDialogOpen] = React.useState(false);
  const [editingIdent, setEditingIdent] = React.useState<string | null>(null);
  const [form, setForm] = React.useState<FormState>(emptyForm);
  const [removeTarget, setRemoveTarget] = React.useState<Permission | null>(
    null,
  );

  const me = useMe();

  const permissions = useQuery({
    queryKey: ["servers", serverId, "permissions"],
    queryFn: () =>
      apiGet(`/api/servers/${serverId}/permissions`, permissionsResponseSchema),
  });

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
    const already = new Set(
      (permissions.data?.permissions ?? []).map((p) => p.user_id),
    );
    const meId = me.data?.user.id;
    return (directory.data?.users ?? []).filter(
      (u) => u.id !== meId && !already.has(u.id),
    );
  }, [directory.data, permissions.data, me.data]);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["servers", serverId] });
  };

  const upsert = useMutation({
    mutationFn: (payload: FormState) => {
      // owner ได้ทุก cap โดยปริยาย — ไม่ต้องส่ง capabilities
      const base = {
        role: payload.role,
        capabilities: payload.role === "owner" ? [] : payload.capabilities,
      };
      const body = payload.userId
        ? { ...base, user_id: payload.userId }
        : { ...base, email: payload.email.trim() };
      return apiSendVoid("POST", `/api/servers/${serverId}/permissions`, body);
    },
    onSuccess: () => {
      toast.success(editingIdent ? t("access.updated") : t("access.added"));
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
    setEditingIdent(null);
    setForm(emptyForm);
    setDialogOpen(true);
  };

  const openEdit = (perm: Permission) => {
    setEditingIdent(userIdent(perm));
    setForm({
      // ยึด user_id ตอนแก้ไข — collaborator ที่เป็น username-only account มี email = ""
      userId: perm.user_id,
      email: userIdent(perm),
      role: perm.role,
      capabilities: perm.capabilities,
    });
    setDialogOpen(true);
  };

  // ปุ่ม add ปิดไว้จนกว่าจะเลือก user หรือกรอก email (ตอนแก้ไขไม่ต้องเช็ค)
  const canSubmit =
    editingIdent !== null || form.userId !== "" || form.email.trim() !== "";

  const toggle = (key: string, on: boolean) =>
    setForm((prev) => ({
      ...prev,
      capabilities: on
        ? [...new Set([...prev.capabilities, key])]
        : prev.capabilities.filter((k) => k !== key),
    }));

  const toggleGroup = (keys: string[], on: boolean) =>
    setForm((prev) => ({
      ...prev,
      capabilities: on
        ? [...new Set([...prev.capabilities, ...keys])]
        : prev.capabilities.filter((k) => !keys.includes(k)),
    }));

  return (
    <div className="grid gap-4">
      <div className="flex items-center justify-between">
        <p className="text-muted-foreground text-sm">{t("access.subtitle")}</p>
        <Button size="sm" onClick={openAdd}>
          <PlusIcon />
          {t("access.addUser")}
        </Button>
      </div>

      {permissions.isPending ? (
        <Skeleton className="h-32 w-full" />
      ) : permissions.isError ? (
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
                {permissions.data.permissions.map((perm) => {
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
        <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              {editingIdent
                ? t("access.editTitle", { email: editingIdent })
                : t("access.addUser")}
            </DialogTitle>
            <DialogDescription>{t("access.accountRequired")}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (canSubmit && !upsert.isPending) upsert.mutate(form);
            }}
          >
            {editingIdent === null && (
              <div className="grid gap-2">
                <Label htmlFor="perm-user">{t("access.pickUser")}</Label>
                <Select
                  value={form.userId}
                  onValueChange={(v) =>
                    // เลือก user → เคลียร์ช่อง email (ใช้ user_id แทน)
                    setForm({ ...form, userId: v, email: "" })
                  }
                  disabled={pickableUsers.length === 0}
                >
                  <SelectTrigger id="perm-user">
                    <SelectValue
                      placeholder={
                        directory.isPending
                          ? t("common.loading")
                          : pickableUsers.length === 0
                            ? t("access.noPickable")
                            : t("access.pickUserPlaceholder")
                      }
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {pickableUsers.map((u) => (
                      <SelectItem key={u.id} value={u.id}>
                        {directoryLabel(u)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            {editingIdent === null && (
              <div className="grid gap-2">
                <Label htmlFor="perm-email">{t("access.orEmail")}</Label>
                <Input
                  id="perm-email"
                  type="email"
                  disabled={form.userId !== ""}
                  placeholder={t("access.emailPlaceholder")}
                  value={form.email}
                  onChange={(e) =>
                    // พิมพ์ email → ล้าง user ที่เลือกไว้
                    setForm({ ...form, email: e.target.value, userId: "" })
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
                  // owner ได้ทุก server-scoped cap โดยปริยาย — โชว์เปิดค้าง กดไม่ได้
                  isAdmin={form.role === "owner"}
                  capabilities={form.capabilities}
                  onToggle={toggle}
                  onToggleGroup={toggleGroup}
                />
              )}
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialogOpen(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={!canSubmit || upsert.isPending}>
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
            ? t("access.removeDesc", { email: userTitle(removeTarget) })
            : ""
        }
        confirmLabel={t("common.remove")}
        destructive
        pending={remove.isPending}
        onConfirm={() => {
          if (removeTarget) remove.mutate(removeTarget.user_id);
        }}
      />
    </div>
  );
}
