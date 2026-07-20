"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import Link from "next/link";
import {
  KeyRoundIcon,
  MoreHorizontalIcon,
  PauseCircleIcon,
  PlayCircleIcon,
  PlusIcon,
  RotateCcwIcon,
  ServerIcon,
  ShieldIcon,
  Trash2Icon,
} from "lucide-react";
import {
  apiGet,
  apiSend,
  apiSendVoid,
  deleteUser,
  restoreUser,
  ApiError,
} from "@/lib/api";
import {
  capabilitiesResponseSchema,
  createUserResponseSchema,
  resetPasswordResponseSchema,
  userResponseSchema,
  usersResponseSchema,
  type User,
} from "@/lib/types";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { detectRole } from "@/lib/user-roles";
import { userIdent } from "@/lib/user-display";
import { formatDateTime } from "@/lib/format";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { SecretDialog } from "@/components/secret-dialog";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { UserIdentity } from "@/components/user/user-identity";
import {
  CreateUserDialog,
  type CreateUserBody,
} from "@/components/user/create-user-dialog";

export default function AdminUsersPage() {
  const t = useT();
  useSetBreadcrumbs(
    React.useMemo(
      () => [{ label: t("nav.admin") }, { label: t("users.title") }],
      [t],
    ),
  );
  const queryClient = useQueryClient();
  const { data: meData } = useMe();
  const me = meData?.user;
  // สิทธิ์ต่อ action — ปุ่มที่ใช้ไม่ได้ให้ซ่อน ไม่ใช่ให้กดแล้วเจอ 403
  const canView = hasCapability(me, CAPABILITY.usersView);
  const canCreateUser = hasCapability(me, CAPABILITY.usersCreate);
  const canEditUser = hasCapability(me, CAPABILITY.usersEdit);
  const canDeleteUser = hasCapability(me, CAPABILITY.usersDelete);
  const canRestoreUser = hasCapability(me, CAPABILITY.usersRestore);
  const canResetPassword = hasCapability(me, CAPABILITY.usersResetPassword);

  const [createOpen, setCreateOpen] = React.useState(false);
  const [secret, setSecret] = React.useState<{
    title: string;
    password: string;
    // รีเซ็ตรหัสผ่านของตัวเอง = token_version ถูก bump เซสชันนี้ตายไปแล้ว —
    // ปิด dialog เมื่อไหร่ต้องเด้งออกทันที ไม่ปล่อยให้คลิกต่อแล้วเจอ 401 รัว ๆ
    logoutOnClose?: boolean;
  } | null>(null);
  const [resetTarget, setResetTarget] = React.useState<User | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<User | null>(null);
  const [restoreTarget, setRestoreTarget] = React.useState<User | null>(null);

  // filter bar — search debounce 300ms กัน refetch ถี่ทุก keystroke
  const [searchInput, setSearchInput] = React.useState("");
  const [search, setSearch] = React.useState("");
  const [role, setRole] = React.useState<"all" | "admin" | "user">("all");
  // status=deleted = ถังขยะ (ที่เดียวที่เห็นบัญชีที่ถูกลบ — soft delete เก็บแถวไว้)
  const [status, setStatus] = React.useState<
    "all" | "active" | "inactive" | "deleted"
  >("all");
  const trash = status === "deleted";

  React.useEffect(() => {
    const id = setTimeout(() => setSearch(searchInput.trim()), 300);
    return () => clearTimeout(id);
  }, [searchInput]);

  const users = useQuery({
    queryKey: ["users", { search, role, status }],
    queryFn: () => {
      const params = new URLSearchParams();
      if (search) params.set("search", search);
      if (role !== "all") params.set("role", role);
      if (status !== "all") params.set("status", status);
      const qs = params.toString();
      return apiGet(`/api/users${qs ? `?${qs}` : ""}`, usersResponseSchema);
    },
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

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["users"] });

  const logout = async () => {
    try {
      await apiSendVoid("POST", "/api/auth/logout");
    } finally {
      window.location.assign("/login");
    }
  };

  const createUser = useMutation({
    mutationFn: (body: CreateUserBody) =>
      apiSend("POST", "/api/users", body, createUserResponseSchema),
    onSuccess: (data) => {
      setCreateOpen(false);
      setSecret({
        title: t("users.initialPasswordFor", { name: userIdent(data.user) }),
        password: data.initial_password,
      });
      invalidate();
    },
    onError: (err) => {
      if (err instanceof ApiError && err.code === "invalid_username") {
        toast.error(t("users.invalidUsername"));
        return;
      }
      if (err instanceof ApiError && err.code === "username_exists") {
        toast.error(t("users.usernameExists"));
        return;
      }
      if (err instanceof ApiError && err.code === "username_reserved") {
        toast.error(t("users.usernameReserved"));
        return;
      }
      toast.error(
        err instanceof ApiError ? err.message : t("users.failedCreate"),
      );
    },
  });

  const removeUser = useMutation({
    mutationFn: (id: string) => deleteUser(id),
    onSuccess: () => {
      toast.success(t("users.deleted"));
      setDeleteTarget(null);
      invalidate();
    },
    onError: (err) => {
      if (err instanceof ApiError && err.code === "cannot_delete_self") {
        toast.error(t("users.cannotDeleteSelf"));
        return;
      }
      toast.error(
        err instanceof ApiError ? err.message : t("users.failedDelete"),
      );
    },
  });

  const bringBackUser = useMutation({
    mutationFn: (id: string) => restoreUser(id),
    onSuccess: () => {
      toast.success(t("users.restored"));
      setRestoreTarget(null);
      invalidate();
    },
    onError: (err) => {
      // username ถูกคนอื่นใช้ไประหว่างที่บัญชีนี้อยู่ในถังขยะ
      if (err instanceof ApiError && err.code === "username_exists") {
        toast.error(t("users.restoreUsernameTaken"));
        return;
      }
      toast.error(
        err instanceof ApiError ? err.message : t("users.failedRestore"),
      );
    },
  });

  const patchUser = useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: Partial<Pick<User, "is_admin" | "is_active" | "capabilities">>;
    }) => apiSend("PATCH", `/api/users/${id}`, body, userResponseSchema),
    onSuccess: () => {
      toast.success(t("users.updated"));
      invalidate();
    },
    onError: (err) => {
      toast.error(
        err instanceof ApiError ? err.message : t("users.failedUpdate"),
      );
    },
  });

  const resetPassword = useMutation({
    mutationFn: (id: string) =>
      apiSend(
        "POST",
        `/api/users/${id}/reset-password`,
        undefined,
        resetPasswordResponseSchema,
      ),
    onSuccess: (data, id) => {
      const target = users.data?.users.find((u) => u.id === id);
      setResetTarget(null);
      setSecret({
        title: t("users.newPasswordFor", {
          name: target ? userIdent(target) : "user",
        }),
        password: data.initial_password,
        logoutOnClose: id === me?.id,
      });
    },
    onError: (err) => {
      toast.error(
        err instanceof ApiError ? err.message : t("users.failedReset"),
      );
    },
  });

  // กันเข้าตรง URL — menu ซ่อนอยู่แล้วแต่ต้องกันซ้ำ
  if (me && !canView) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>
    );
  }

  const userList = users.data?.users ?? [];

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-muted-foreground text-sm">{t("users.subtitle")}</p>
        {canCreateUser && !trash && (
          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <PlusIcon />
            {t("users.create")}
          </Button>
        )}
      </div>

      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
        <Input
          className="w-full sm:max-w-xs"
          placeholder={t("users.filterSearch")}
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
        />
        <Select value={role} onValueChange={(v) => setRole(v as typeof role)}>
          <SelectTrigger
            className="w-full sm:w-40"
            aria-label={t("users.filterRole")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("users.roleAll")}</SelectItem>
            <SelectItem value="admin">{t("users.roleAdmin")}</SelectItem>
            <SelectItem value="user">{t("users.roleUser")}</SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={status}
          onValueChange={(v) => setStatus(v as typeof status)}
        >
          <SelectTrigger
            className="w-full sm:w-40"
            aria-label={t("users.filterStatus")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t("users.statusAll")}</SelectItem>
            <SelectItem value="active">{t("users.statusActive")}</SelectItem>
            <SelectItem value="inactive">
              {t("users.statusInactive")}
            </SelectItem>
            <SelectItem value="deleted">{t("users.statusDeleted")}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {users.isPending ? (
        <Skeleton className="h-64 w-full" />
      ) : users.isError ? (
        <p className="text-destructive text-sm">{t("users.failedLoad")}</p>
      ) : userList.length === 0 ? (
        <Card className="py-10">
          <CardContent className="text-muted-foreground text-center text-sm">
            {trash ? t("users.trashEmpty") : t("users.empty")}
          </CardContent>
        </Card>
      ) : (
        <Card className="py-0">
          <CardContent className="overflow-x-auto px-0">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("users.colUser")}</TableHead>
                  <TableHead>{t("users.colAccess")}</TableHead>
                  <TableHead>{t("users.colStatus")}</TableHead>
                  <TableHead className="text-right">
                    {t("users.colActions")}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {userList.map((user) => {
                  const self = user.id === me?.id;
                  const roleKey = detectRole(user);
                  return (
                    <TableRow key={user.id}>
                      <TableCell>
                        <UserIdentity
                          user={user}
                          panelRole={roleKey}
                          // ในถังขยะไม่มีหน้า detail ให้เข้า (GET /users/{id} ตอบ 404)
                          href={
                            trash
                              ? undefined
                              : `/admin/users/${user.id}/permissions`
                          }
                          trailing={
                            <>
                              {self && (
                                <span className="text-muted-foreground font-normal">
                                  ({t("users.you")})
                                </span>
                              )}
                              {user.must_change_password && (
                                <Badge
                                  variant="outline"
                                  className="border-yellow-500/30 bg-yellow-500/15 text-yellow-400"
                                >
                                  {t("users.pendingPassword")}
                                </Badge>
                              )}
                            </>
                          }
                        />
                      </TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {user.is_admin
                          ? t("users.accessAll")
                          : t("users.accessCount", {
                              count: user.capabilities.length,
                              total: catalog.length,
                            })}
                      </TableCell>
                      <TableCell>
                        <span className="flex items-center gap-2 text-sm">
                          <span
                            className={cn(
                              "size-2 rounded-full",
                              user.deleted_at
                                ? "bg-destructive"
                                : user.is_active
                                  ? "bg-emerald-500"
                                  : "bg-amber-500",
                            )}
                          />
                          {user.deleted_at
                            ? t("users.deletedOn", {
                                date: formatDateTime(user.deleted_at),
                              })
                            : user.is_active
                              ? t("users.statusActive")
                              : t("users.statusSuspended")}
                        </span>
                      </TableCell>
                      <TableCell>
                        <div className="flex justify-end">
                          {/* บัญชีในถังขยะแก้อะไรไม่ได้ — กู้กลับมาก่อนถึงจะจัดการต่อได้
                              จึงเหลือปุ่มเดียวไม่ต้องมี menu */}
                          {trash ? (
                            canRestoreUser && (
                              <Button
                                variant="outline"
                                size="sm"
                                disabled={bringBackUser.isPending}
                                onClick={() => setRestoreTarget(user)}
                              >
                                <RotateCcwIcon />
                                {t("users.restore")}
                              </Button>
                            )
                          ) : (
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  className="size-8"
                                  aria-label={t("users.actionsFor", {
                                    name: userIdent(user),
                                  })}
                                >
                                  <MoreHorizontalIcon />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="w-60">
                                <DropdownMenuLabel className="text-muted-foreground text-xs font-normal">
                                  {userIdent(user)}
                                </DropdownMenuLabel>
                                <DropdownMenuSeparator />
                                <DropdownMenuItem asChild>
                                  <Link
                                    href={`/admin/users/${user.id}/permissions`}
                                  >
                                    <ShieldIcon />
                                    {t("users.menuPermissions")}
                                  </Link>
                                </DropdownMenuItem>
                                <DropdownMenuItem asChild>
                                  <Link
                                    href={`/admin/users/${user.id}/servers`}
                                  >
                                    <ServerIcon />
                                    {t("users.menuServerAccess")}
                                  </Link>
                                </DropdownMenuItem>
                                {(canResetPassword || canEditUser) && (
                                  <DropdownMenuSeparator />
                                )}
                                {canResetPassword && (
                                  <DropdownMenuItem
                                    onSelect={() => setResetTarget(user)}
                                  >
                                    <KeyRoundIcon />
                                    {t("users.menuResetPassword")}
                                  </DropdownMenuItem>
                                )}
                                {canEditUser && (
                                  // ระงับ/ลบตัวเองไม่ได้ (กันล็อกตัวเองออกจากระบบ) —
                                  // ขึ้นเป็น disabled ให้เห็นว่ามีเมนูนี้อยู่ ดีกว่าซ่อนเงียบ ๆ
                                  <DropdownMenuItem
                                    disabled={self || patchUser.isPending}
                                    onSelect={() =>
                                      patchUser.mutate({
                                        id: user.id,
                                        body: { is_active: !user.is_active },
                                      })
                                    }
                                  >
                                    {user.is_active ? (
                                      <PauseCircleIcon />
                                    ) : (
                                      <PlayCircleIcon />
                                    )}
                                    {user.is_active
                                      ? t("users.menuSuspend")
                                      : t("users.menuActivate")}
                                  </DropdownMenuItem>
                                )}
                                {canDeleteUser && (
                                  <>
                                    <DropdownMenuSeparator />
                                    <DropdownMenuItem
                                      variant="destructive"
                                      disabled={self}
                                      onSelect={() => setDeleteTarget(user)}
                                    >
                                      <Trash2Icon />
                                      {t("users.menuDelete")}
                                    </DropdownMenuItem>
                                  </>
                                )}
                              </DropdownMenuContent>
                            </DropdownMenu>
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

      <CreateUserDialog
        open={createOpen}
        catalog={catalog}
        pending={createUser.isPending}
        onOpenChange={setCreateOpen}
        onSubmit={(body) => createUser.mutate(body)}
      />

      <ConfirmDialog
        open={resetTarget !== null}
        onOpenChange={(open) => {
          if (!open) setResetTarget(null);
        }}
        title={t("users.resetTitle", {
          name: resetTarget ? userIdent(resetTarget) : "",
        })}
        description={
          resetTarget?.id === me?.id
            ? t("users.resetSelfDesc")
            : t("users.resetDesc")
        }
        confirmLabel={t("users.resetConfirm")}
        destructive
        pending={resetPassword.isPending}
        onConfirm={() => {
          if (resetTarget) resetPassword.mutate(resetTarget.id);
        }}
      />

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("users.deleteTitle", {
          name: deleteTarget ? userIdent(deleteTarget) : "",
        })}
        description={t("users.deleteConfirm")}
        confirmLabel={t("users.menuDelete")}
        destructive
        pending={removeUser.isPending}
        onConfirm={() => {
          if (deleteTarget) removeUser.mutate(deleteTarget.id);
        }}
      />

      <ConfirmDialog
        open={restoreTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRestoreTarget(null);
        }}
        title={t("users.restoreTitle", {
          name: restoreTarget ? userIdent(restoreTarget) : "",
        })}
        description={t("users.restoreDesc")}
        confirmLabel={t("users.restore")}
        pending={bringBackUser.isPending}
        onConfirm={() => {
          if (restoreTarget) bringBackUser.mutate(restoreTarget.id);
        }}
      />

      <SecretDialog
        open={secret !== null}
        onOpenChange={(open) => {
          if (open) return;
          if (secret?.logoutOnClose) {
            logout();
            return;
          }
          setSecret(null);
        }}
        title={secret?.title ?? ""}
        description={
          secret?.logoutOnClose
            ? t("users.selfResetDesc")
            : t("users.sharePassword")
        }
        secret={secret?.password ?? ""}
      />
    </div>
  );
}
