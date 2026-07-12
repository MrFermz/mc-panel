"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { PencilIcon, PlusIcon, Trash2Icon } from "lucide-react";
import { apiGet, apiSendVoid, ApiError } from "@/lib/api";
import {
  permissionsResponseSchema,
  type Permission,
  type PermissionRole,
} from "@/lib/types";
import { useT, type TranslationKey } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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

interface FormState {
  email: string;
  role: PermissionRole;
  can_console_write: boolean;
  can_manage_files: boolean;
}

const emptyForm: FormState = {
  email: "",
  role: "viewer",
  can_console_write: false,
  can_manage_files: false,
};

function roleKey(role: PermissionRole): TranslationKey {
  if (role === "owner") return "access.roleOwner";
  if (role === "operator") return "access.roleOperator";
  return "access.roleViewer";
}

export default function AccessTab({ serverId }: { serverId: string }) {
  const t = useT();
  const queryClient = useQueryClient();
  const [dialogOpen, setDialogOpen] = React.useState(false);
  const [editingEmail, setEditingEmail] = React.useState<string | null>(null);
  const [form, setForm] = React.useState<FormState>(emptyForm);
  const [removeTarget, setRemoveTarget] = React.useState<Permission | null>(null);

  const permissions = useQuery({
    queryKey: ["servers", serverId, "permissions"],
    queryFn: () =>
      apiGet(`/api/servers/${serverId}/permissions`, permissionsResponseSchema),
  });

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["servers", serverId] });
  };

  const upsert = useMutation({
    mutationFn: (payload: FormState) =>
      apiSendVoid("POST", `/api/servers/${serverId}/permissions`, {
        ...payload,
        email: payload.email.trim(),
      }),
    onSuccess: () => {
      toast.success(editingEmail ? t("access.updated") : t("access.added"));
      setDialogOpen(false);
      invalidate();
    },
    onError: (err) => {
      if (err instanceof ApiError && err.code === "user_not_found") {
        toast.error(t("access.userNotFound"));
      } else {
        toast.error(err instanceof ApiError ? err.message : t("access.failedSave"));
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
      toast.error(err instanceof ApiError ? err.message : t("access.failedRemove"));
    },
  });

  const openAdd = () => {
    setEditingEmail(null);
    setForm(emptyForm);
    setDialogOpen(true);
  };

  const openEdit = (perm: Permission) => {
    setEditingEmail(perm.email);
    setForm({
      email: perm.email,
      role: perm.role,
      can_console_write: perm.can_console_write,
      can_manage_files: perm.can_manage_files,
    });
    setDialogOpen(true);
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

      {permissions.isPending ? (
        <Skeleton className="h-32 w-full" />
      ) : permissions.isError ? (
        <p className="text-destructive text-sm">{t("access.failedLoad")}</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("access.user")}</TableHead>
              <TableHead>{t("access.role")}</TableHead>
              <TableHead>{t("access.consoleWrite")}</TableHead>
              <TableHead>{t("access.manageFiles")}</TableHead>
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {permissions.data.permissions.map((perm) => (
              <TableRow key={perm.user_id}>
                <TableCell>
                  <div className="grid">
                    <span>{perm.display_name || perm.email}</span>
                    <span className="text-muted-foreground text-xs">
                      {perm.email}
                    </span>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant="secondary">{t(roleKey(perm.role))}</Badge>
                </TableCell>
                <TableCell>
                  {perm.can_console_write ? t("common.yes") : t("common.no")}
                </TableCell>
                <TableCell>
                  {perm.can_manage_files ? t("common.yes") : t("common.no")}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => openEdit(perm)}
                      aria-label={`${t("common.edit")} ${perm.email}`}
                    >
                      <PencilIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="text-destructive"
                      onClick={() => setRemoveTarget(perm)}
                      aria-label={`${t("common.remove")} ${perm.email}`}
                    >
                      <Trash2Icon />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editingEmail
                ? t("access.editTitle", { email: editingEmail })
                : t("access.addUser")}
            </DialogTitle>
            <DialogDescription>{t("access.accountRequired")}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (!upsert.isPending) upsert.mutate(form);
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="perm-email">{t("access.email")}</Label>
              <Input
                id="perm-email"
                type="email"
                required
                disabled={editingEmail !== null}
                value={form.email}
                onChange={(e) => setForm({ ...form, email: e.target.value })}
              />
            </div>
            <div className="grid gap-2">
              <Label>{t("access.role")}</Label>
              <Select
                value={form.role}
                onValueChange={(v) =>
                  setForm({ ...form, role: v as PermissionRole })
                }
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="owner">{t("access.roleOwner")}</SelectItem>
                  <SelectItem value="operator">
                    {t("access.roleOperator")}
                  </SelectItem>
                  <SelectItem value="viewer">{t("access.roleViewer")}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="perm-console"
                checked={form.can_console_write}
                onCheckedChange={(v) =>
                  setForm({ ...form, can_console_write: v === true })
                }
              />
              <Label htmlFor="perm-console" className="font-normal">
                {t("access.canConsole")}
              </Label>
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="perm-files"
                checked={form.can_manage_files}
                onCheckedChange={(v) =>
                  setForm({ ...form, can_manage_files: v === true })
                }
              />
              <Label htmlFor="perm-files" className="font-normal">
                {t("access.canFiles")}
              </Label>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setDialogOpen(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={upsert.isPending}>
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
            ? t("access.removeDesc", { email: removeTarget.email })
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
