"use client";

import * as React from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeftIcon, PlusIcon, Trash2Icon } from "lucide-react";
import {
  apiGet,
  assignUserServer,
  listUserServers,
  unassignUserServer,
  ApiError,
} from "@/lib/api";
import {
  capabilitiesResponseSchema,
  serversResponseSchema,
  userResponseSchema,
  type ServerPermission,
} from "@/lib/types";
import {
  CAPABILITY,
  SERVER_SCOPED_CAPABILITIES,
  hasCapability,
} from "@/lib/capabilities";
import { matchPreset } from "@/lib/user-roles";
import {
  SERVER_ROLE_LABEL_KEYS,
  SERVER_ROLE_PRESETS,
  matchServerPreset,
  type ServerRolePreset,
} from "@/lib/server-roles";
import { userIdent, userTitle } from "@/lib/user-display";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { UserIdentity } from "@/components/user/user-identity";
import { UserDetailTabs } from "@/components/user/user-detail-tabs";
import {
  FieldGroupLabel,
  PermissionGroups,
} from "@/components/user/permission-fields";

const DEFAULT_PRESET: ServerRolePreset = SERVER_ROLE_PRESETS.find(
  (p) => p.key === "viewer",
)!;

interface FormState {
  serverId: string;
  presetKey: ServerRolePreset["key"];
}

// assign server ให้ user จากฝั่ง user — เป็นข้อมูลชุดเดียวกับแท็บ Access ต่อ server
// (server_permissions) แค่มองกลับด้าน: ที่นี่ตรึง user แล้วเลือก server
export default function UserServersPage() {
  const t = useT();
  const params = useParams<{ id: string }>();
  const userId = params.id;
  const queryClient = useQueryClient();

  const { data: meData } = useMe();
  const me = meData?.user;
  const canView = hasCapability(me, CAPABILITY.usersView);
  const canViewAccess = hasCapability(me, CAPABILITY.accessView);
  const canManageAccess = hasCapability(me, CAPABILITY.accessManage);
  const canViewAllServers = hasCapability(me, CAPABILITY.serversViewAll);

  useSetBreadcrumbs(
    React.useMemo(
      () => [
        { label: t("nav.admin") },
        { label: t("users.title"), href: "/admin/users" },
        { label: t("users.serverAccess") },
      ],
      [t],
    ),
  );

  const [dialogOpen, setDialogOpen] = React.useState(false);
  const [form, setForm] = React.useState<FormState>({
    serverId: "",
    presetKey: DEFAULT_PRESET.key,
  });
  const [removeTarget, setRemoveTarget] =
    React.useState<ServerPermission | null>(null);

  const user = useQuery({
    queryKey: ["users", userId],
    queryFn: () => apiGet(`/api/users/${userId}`, userResponseSchema),
    enabled: canView,
  });

  const grants = useQuery({
    queryKey: ["users", userId, "servers"],
    queryFn: () => listUserServers(userId),
    enabled: canViewAccess,
  });

  // รายชื่อ server ให้เลือก — scope=all ต้องมี servers.view_all ไม่งั้นตกไปใช้ของตัวเอง
  // (backend ยังเช็คซ้ำว่าเป็น owner ของ server นั้นจริงตอน POST)
  const servers = useQuery({
    queryKey: ["servers", { scope: canViewAllServers ? "all" : "mine" }],
    queryFn: () =>
      apiGet(
        canViewAllServers ? "/api/servers?scope=all" : "/api/servers",
        serversResponseSchema,
      ),
    enabled: canManageAccess,
  });

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

  // server ที่ยังไม่ได้ assign และไม่ได้อยู่ในถังขยะ
  const pickableServers = React.useMemo(() => {
    const already = new Set(
      (grants.data?.permissions ?? []).map((p) => p.server_id),
    );
    return (servers.data?.servers ?? []).filter(
      (s) => !s.deleted_at && !already.has(s.id),
    );
  }, [servers.data, grants.data]);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: ["users", userId, "servers"] });
    // แท็บ Access ของ server นั้นอ่านข้อมูลชุดเดียวกัน
    queryClient.invalidateQueries({ queryKey: ["servers"] });
  };

  const assign = useMutation({
    mutationFn: (payload: FormState) => {
      const preset =
        SERVER_ROLE_PRESETS.find((p) => p.key === payload.presetKey) ??
        DEFAULT_PRESET;
      return assignUserServer(userId, {
        server_id: payload.serverId,
        role: preset.role,
        capabilities: preset.capabilities,
      });
    },
    onSuccess: () => {
      toast.success(t("access.added"));
      setDialogOpen(false);
      invalidate();
    },
    onError: (err) => {
      toast.error(
        err instanceof ApiError ? err.message : t("access.failedSave"),
      );
    },
  });

  const unassign = useMutation({
    mutationFn: (serverId: string) => unassignUserServer(userId, serverId),
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

  if (me && !canView) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>
    );
  }

  if (user.isPending) return <Skeleton className="h-96 w-full" />;
  if (user.isError || !user.data?.user) {
    return <p className="text-destructive text-sm">{t("users.failedLoad")}</p>;
  }
  const loaded = user.data.user;

  const selectedPreset =
    SERVER_ROLE_PRESETS.find((p) => p.key === form.presetKey) ?? DEFAULT_PRESET;

  const openAdd = () => {
    setForm({ serverId: "", presetKey: DEFAULT_PRESET.key });
    setDialogOpen(true);
  };

  return (
    <div className="grid gap-4">
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
            panelRole={matchPreset(loaded.is_admin, loaded.capabilities)}
            size="lg"
            className="mr-auto"
          />
          {userIdent(loaded) !== userTitle(loaded) && (
            <span className="text-muted-foreground text-xs">
              {userIdent(loaded)}
            </span>
          )}
        </CardContent>
      </Card>

      <UserDetailTabs userId={userId} />

      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">
          {loaded.is_admin
            ? t("users.serverAccessAdminHint")
            : t("users.serverAccessSubtitle")}
        </p>
        {canManageAccess && (
          <Button size="sm" onClick={openAdd}>
            <PlusIcon />
            {t("users.assignServer")}
          </Button>
        )}
      </div>

      {!canViewAccess ? (
        <p className="text-muted-foreground text-sm">
          {t("users.needAccessView")}
        </p>
      ) : grants.isPending ? (
        <Skeleton className="h-40 w-full" />
      ) : grants.isError ? (
        <p className="text-destructive text-sm">{t("access.failedLoad")}</p>
      ) : grants.data.permissions.length === 0 ? (
        <Card className="py-10">
          <CardContent className="text-muted-foreground text-center text-sm">
            {t("users.noServerAccess")}
          </CardContent>
        </Card>
      ) : (
        <Card className="py-0">
          <CardContent className="overflow-x-auto px-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("users.colServer")}</TableHead>
                  <TableHead>{t("access.accessLevel")}</TableHead>
                  <TableHead className="w-16" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {grants.data.permissions.map((perm) => {
                  const preset = matchServerPreset(
                    perm.role,
                    perm.capabilities,
                  );
                  return (
                    <TableRow key={perm.server_id}>
                      <TableCell>
                        <span className="flex items-center gap-2 text-sm font-medium">
                          <span
                            className={cn(
                              "size-2 shrink-0 rounded-full",
                              perm.server_status === "running"
                                ? "bg-emerald-500"
                                : "bg-muted-foreground/40",
                            )}
                          />
                          {perm.server_name}
                        </span>
                      </TableCell>
                      <TableCell className="text-sm">
                        {t(SERVER_ROLE_LABEL_KEYS[preset])}
                        {perm.role === "member" && (
                          <span className="text-muted-foreground">
                            {" · "}
                            {t("access.capsCount", {
                              count: perm.capabilities.length,
                            })}
                          </span>
                        )}
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end">
                          {canManageAccess && (
                            <Button
                              variant="ghost"
                              size="icon"
                              className="text-destructive"
                              onClick={() => setRemoveTarget(perm)}
                              aria-label={`${t("common.remove")} ${perm.server_name}`}
                            >
                              <Trash2Icon />
                            </Button>
                          )}
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
        <DialogContent className="sm:max-w-2xl">
          <form
            className="contents"
            onSubmit={(e) => {
              e.preventDefault();
              if (!form.serverId || assign.isPending) return;
              assign.mutate(form);
            }}
          >
            <DialogHeader>
              <DialogTitle className="text-base">
                {t("users.assignServerTitle", { name: userTitle(loaded) })}
              </DialogTitle>
              <DialogDescription className="text-xs">
                {t("users.assignServerDesc")}
              </DialogDescription>
            </DialogHeader>

            <DialogBody className="gap-6">
              <div className="grid gap-2">
                <Label htmlFor="assign-server">{t("users.colServer")}</Label>
                <Select
                  value={form.serverId}
                  onValueChange={(v) => setForm({ ...form, serverId: v })}
                  disabled={pickableServers.length === 0}
                >
                  <SelectTrigger id="assign-server" className="w-full">
                    <SelectValue
                      placeholder={
                        servers.isPending
                          ? t("common.loading")
                          : pickableServers.length === 0
                            ? t("users.noPickableServer")
                            : t("users.pickServerPlaceholder")
                      }
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {pickableServers.map((s) => (
                      <SelectItem key={s.id} value={s.id}>
                        {s.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div>
                <FieldGroupLabel>{t("access.rolePreset")}</FieldGroupLabel>
                <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                  {SERVER_ROLE_PRESETS.map((preset) => (
                    <button
                      key={preset.key}
                      type="button"
                      onClick={() =>
                        setForm({ ...form, presetKey: preset.key })
                      }
                      className={cn(
                        "h-10 rounded-md border text-sm font-medium transition-colors",
                        form.presetKey === preset.key
                          ? "border-primary bg-primary/15 text-primary"
                          : "hover:bg-accent hover:text-accent-foreground",
                      )}
                    >
                      {t(SERVER_ROLE_LABEL_KEYS[preset.key])}
                    </button>
                  ))}
                </div>
                <p className="text-muted-foreground mt-2 text-xs">
                  {selectedPreset.role === "owner"
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
                    isAdmin={selectedPreset.role === "owner"}
                    capabilities={selectedPreset.capabilities}
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
                loading={assign.isPending}
                disabled={!form.serverId}
              >
                {assign.isPending ? t("common.saving") : t("users.assign")}
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
            ? t("users.unassignDesc", {
                name: userTitle(loaded),
                server: removeTarget.server_name,
              })
            : ""
        }
        confirmLabel={t("common.remove")}
        destructive
        pending={unassign.isPending}
        onConfirm={() => {
          if (removeTarget) unassign.mutate(removeTarget.server_id);
        }}
      />
    </div>
  );
}
