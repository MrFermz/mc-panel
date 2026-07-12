"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  KeyRoundIcon,
  PlusIcon,
  SlidersHorizontalIcon,
  Trash2Icon,
} from "lucide-react";
import { apiGet, apiSend, deleteUser, ApiError } from "@/lib/api";
import {
  capabilitiesResponseSchema,
  createUserResponseSchema,
  resetPasswordResponseSchema,
  userResponseSchema,
  usersResponseSchema,
  type Capability,
  type User,
} from "@/lib/types";
import { formatDateTime } from "@/lib/format";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useMe } from "@/lib/use-me";
import { useT, type TranslateFn, type TranslationKey } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
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
import { SecretDialog } from "@/components/secret-dialog";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";

// map capability key จาก backend เป็นคำแปลฝั่ง web (อย่าพึ่ง label อังกฤษจาก API ตรง ๆ)
const CAP_LABEL_KEYS: Record<string, TranslationKey> = {
  "users.manage": "cap.users.manage.label",
  "nodes.manage": "cap.nodes.manage.label",
  "servers.create": "cap.servers.create.label",
  "servers.view_all": "cap.servers.view_all.label",
};

function capLabel(t: TranslateFn, key: string, fallback: string): string {
  const tk = CAP_LABEL_KEYS[key];
  return tk ? t(tk) : fallback;
}

// identifier ที่แสดงต่อ user — email มาก่อน, ถ้าไม่มี (user แบบ username-only) ตกไปที่ username
function userIdent(u: Pick<User, "email" | "username" | "display_name">): string {
  return u.email || u.username || u.display_name || "user";
}

// username: 3-64 ตัว [A-Za-z0-9_.-] — ตรงกับ backend
const USERNAME_RE = /^[A-Za-z0-9_.-]{3,64}$/;

function CapabilityCheckboxes({
  catalog,
  selected,
  onToggle,
  idPrefix,
}: {
  catalog: Capability[];
  selected: string[];
  onToggle: (key: string, on: boolean) => void;
  idPrefix: string;
}) {
  const t = useT();
  if (catalog.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("users.noCapabilities")}
      </p>
    );
  }
  return (
    <div className="grid gap-3">
      {catalog.map((cap) => {
        const id = `${idPrefix}-${cap.key}`;
        return (
          <div key={cap.key} className="flex items-start gap-2">
            <Checkbox
              id={id}
              checked={selected.includes(cap.key)}
              onCheckedChange={(v) => onToggle(cap.key, v === true)}
            />
            <div className="grid gap-0.5">
              <Label htmlFor={id} className="font-normal">
                {capLabel(t, cap.key, cap.label)}
              </Label>
              <span className="text-muted-foreground text-xs">
                {cap.description}
              </span>
            </div>
          </div>
        );
      })}
    </div>
  );
}

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
  const canManage = hasCapability(me, CAPABILITY.usersManage);

  const [createOpen, setCreateOpen] = React.useState(false);
  const [email, setEmail] = React.useState("");
  const [username, setUsername] = React.useState("");
  const [displayName, setDisplayName] = React.useState("");
  const [isAdmin, setIsAdmin] = React.useState(false);
  const [createCaps, setCreateCaps] = React.useState<string[]>([]);
  const [editTarget, setEditTarget] = React.useState<User | null>(null);
  const [editCaps, setEditCaps] = React.useState<string[]>([]);
  const [secret, setSecret] = React.useState<{
    title: string;
    password: string;
  } | null>(null);
  const [resetTarget, setResetTarget] = React.useState<User | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<User | null>(null);

  // filter bar — search debounce 300ms กัน refetch ถี่ทุก keystroke
  const [searchInput, setSearchInput] = React.useState("");
  const [search, setSearch] = React.useState("");
  const [role, setRole] = React.useState<"all" | "admin" | "user">("all");
  const [status, setStatus] = React.useState<"all" | "active" | "inactive">(
    "all",
  );

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
    enabled: canManage,
  });

  const caps = useQuery({
    queryKey: ["meta", "capabilities"],
    queryFn: () => apiGet("/api/meta/capabilities", capabilitiesResponseSchema),
    enabled: canManage,
    staleTime: 300_000,
  });
  const catalog = React.useMemo(
    () => caps.data?.capabilities ?? [],
    [caps.data],
  );
  const capLabels = React.useMemo(() => {
    const map = new Map<string, string>();
    for (const c of catalog) map.set(c.key, c.label);
    return map;
  }, [catalog]);

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ["users"] });

  const toggle =
    (setter: React.Dispatch<React.SetStateAction<string[]>>) =>
    (key: string, on: boolean) =>
      setter((prev) =>
        on ? [...new Set([...prev, key])] : prev.filter((k) => k !== key),
      );

  const createUser = useMutation({
    mutationFn: () =>
      apiSend(
        "POST",
        "/api/users",
        {
          // ส่งทั้งคู่เสมอ — backend ต้องมีอย่างน้อยหนึ่ง, string ว่าง = "ไม่ระบุ"
          email: email.trim(),
          username: username.trim(),
          display_name: displayName.trim(),
          is_admin: isAdmin,
          capabilities: createCaps,
        },
        createUserResponseSchema,
      ),
    onSuccess: (data) => {
      setCreateOpen(false);
      setEmail("");
      setUsername("");
      setDisplayName("");
      setIsAdmin(false);
      setCreateCaps([]);
      setSecret({
        title: t("users.initialPasswordFor", { email: userIdent(data.user) }),
        password: data.initial_password,
      });
      invalidate();
    },
    onError: (err) => {
      if (err instanceof ApiError && err.code === "identifier_required") {
        toast.error(t("users.identifierRequired"));
        return;
      }
      if (err instanceof ApiError && err.code === "email_exists") {
        toast.error(t("users.emailExists"));
        return;
      }
      if (err instanceof ApiError && err.code === "username_exists") {
        toast.error(t("users.usernameExists"));
        return;
      }
      toast.error(err instanceof ApiError ? err.message : t("users.failedCreate"));
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
      toast.error(err instanceof ApiError ? err.message : t("users.failedDelete"));
    },
  });

  const patchUser = useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: Partial<
        Pick<User, "display_name" | "is_admin" | "is_active" | "capabilities">
      >;
    }) => apiSend("PATCH", `/api/users/${id}`, body, userResponseSchema),
    onSuccess: () => {
      toast.success(t("users.updated"));
      invalidate();
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : t("users.failedUpdate"));
    },
  });

  const saveCapabilities = useMutation({
    mutationFn: ({ id, capabilities }: { id: string; capabilities: string[] }) =>
      apiSend(
        "PATCH",
        `/api/users/${id}`,
        { capabilities },
        userResponseSchema,
      ),
    onSuccess: () => {
      toast.success(t("users.capsUpdated"));
      setEditTarget(null);
      invalidate();
    },
    onError: (err) => {
      toast.error(
        err instanceof ApiError ? err.message : t("users.failedCaps"),
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
          email: target ? userIdent(target) : "user",
        }),
        password: data.initial_password,
      });
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : t("users.failedReset"));
    },
  });

  const openEdit = (user: User) => {
    setEditTarget(user);
    setEditCaps(user.capabilities);
  };

  // ต้องมี email (มี "@") หรือ username (ตรง pattern) อย่างน้อยหนึ่ง ถึงจะ submit ได้
  const emailValid = email.trim().includes("@");
  const usernameValid = USERNAME_RE.test(username.trim());
  const canCreate = emailValid || usernameValid;

  // กันเข้าตรง URL — menu ซ่อนอยู่แล้วแต่ต้องกันซ้ำ
  if (me && !canManage) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>
    );
  }

  return (
    <div className="grid gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">{t("users.title")}</h1>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <PlusIcon />
          {t("users.create")}
        </Button>
      </div>

      <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
        <Input
          className="w-full sm:max-w-xs"
          placeholder={t("users.filterSearch")}
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
        />
        <Select
          value={role}
          onValueChange={(v) => setRole(v as typeof role)}
        >
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
            <SelectItem value="inactive">{t("users.statusInactive")}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {users.isPending ? (
        <Skeleton className="h-40 w-full" />
      ) : users.isError ? (
        <p className="text-destructive text-sm">{t("users.failedLoad")}</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("users.colUser")}</TableHead>
              <TableHead>{t("users.colAdmin")}</TableHead>
              <TableHead>{t("users.colActive")}</TableHead>
              <TableHead>{t("users.colCapabilities")}</TableHead>
              <TableHead>{t("users.colCreated")}</TableHead>
              <TableHead className="w-64" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.data.users.map((user) => {
              const self = user.id === me?.id;
              return (
                <TableRow key={user.id}>
                  <TableCell>
                    <div className="grid">
                      <span className="flex items-center gap-2">
                        {user.display_name || user.email || user.username}
                        {self && (
                          <Badge variant="secondary">{t("users.you")}</Badge>
                        )}
                        {user.must_change_password && (
                          <Badge
                            variant="outline"
                            className="border-yellow-500/30 bg-yellow-500/15 text-yellow-400"
                          >
                            {t("users.pendingPassword")}
                          </Badge>
                        )}
                      </span>
                      <span className="text-muted-foreground text-xs">
                        {user.email && <span>{user.email}</span>}
                        {user.email && user.username && " · "}
                        {user.username && (
                          <span className="font-mono">@{user.username}</span>
                        )}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={user.is_admin}
                      disabled={self || patchUser.isPending}
                      onCheckedChange={(v) =>
                        patchUser.mutate({ id: user.id, body: { is_admin: v } })
                      }
                      aria-label={t("users.toggleAdmin", { email: user.email })}
                    />
                  </TableCell>
                  <TableCell>
                    <Switch
                      checked={user.is_active}
                      disabled={self || patchUser.isPending}
                      onCheckedChange={(v) =>
                        patchUser.mutate({ id: user.id, body: { is_active: v } })
                      }
                      aria-label={t("users.toggleActive", { email: user.email })}
                    />
                  </TableCell>
                  <TableCell>
                    {user.is_admin ? (
                      <span className="text-muted-foreground text-xs">
                        {t("users.allAdmin")}
                      </span>
                    ) : user.capabilities.length === 0 ? (
                      <span className="text-muted-foreground text-xs">—</span>
                    ) : (
                      <div className="flex max-w-56 flex-wrap gap-1">
                        {user.capabilities.map((key) => (
                          <Badge key={key} variant="secondary">
                            {capLabel(t, key, capLabels.get(key) ?? key)}
                          </Badge>
                        ))}
                      </div>
                    )}
                  </TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {formatDateTime(user.created_at)}
                  </TableCell>
                  <TableCell>
                    <div className="flex justify-end gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={user.is_admin}
                        onClick={() => openEdit(user)}
                      >
                        <SlidersHorizontalIcon />
                        {t("users.capabilitiesBtn")}
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => setResetTarget(user)}
                      >
                        <KeyRoundIcon />
                        {t("users.resetPassword")}
                      </Button>
                      {!self && (
                        <Button
                          variant="outline"
                          size="icon"
                          className="text-destructive"
                          onClick={() => setDeleteTarget(user)}
                          aria-label={`${t("users.delete")} ${userIdent(user)}`}
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
      )}

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("users.createTitle")}</DialogTitle>
            <DialogDescription>{t("users.createDesc")}</DialogDescription>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (!createUser.isPending && canCreate) createUser.mutate();
            }}
          >
            <p className="text-muted-foreground text-sm">
              {t("users.identifierHint")}
            </p>
            <div className="grid gap-2">
              <Label htmlFor="u-email">{t("users.emailOptional")}</Label>
              <Input
                id="u-email"
                type="email"
                autoComplete="off"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="u-username">{t("users.usernameOptional")}</Label>
              <Input
                id="u-username"
                minLength={3}
                maxLength={64}
                pattern="[a-zA-Z0-9_.\-]+"
                autoComplete="off"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="u-name">{t("users.displayName")}</Label>
              <Input
                id="u-name"
                maxLength={100}
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
              />
            </div>
            <div className="flex items-center gap-2">
              <Checkbox
                id="u-admin"
                checked={isAdmin}
                onCheckedChange={(v) => setIsAdmin(v === true)}
              />
              <Label htmlFor="u-admin" className="font-normal">
                {t("users.administrator")}
              </Label>
            </div>
            {!isAdmin && (
              <div className="grid gap-2">
                <Label>{t("users.capabilities")}</Label>
                <CapabilityCheckboxes
                  catalog={catalog}
                  selected={createCaps}
                  onToggle={toggle(setCreateCaps)}
                  idPrefix="create-cap"
                />
              </div>
            )}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setCreateOpen(false)}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={createUser.isPending || !canCreate}>
                {createUser.isPending
                  ? t("users.creating")
                  : t("users.create.submit")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={editTarget !== null}
        onOpenChange={(open) => {
          if (!open) setEditTarget(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t("users.capabilitiesFor", {
                name: editTarget?.display_name || editTarget?.email || "",
              })}
            </DialogTitle>
            <DialogDescription>{t("users.capabilitiesDesc")}</DialogDescription>
          </DialogHeader>
          <CapabilityCheckboxes
            catalog={catalog}
            selected={editCaps}
            onToggle={toggle(setEditCaps)}
            idPrefix="edit-cap"
          />
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setEditTarget(null)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              disabled={saveCapabilities.isPending}
              onClick={() => {
                if (editTarget)
                  saveCapabilities.mutate({
                    id: editTarget.id,
                    capabilities: editCaps,
                  });
              }}
            >
              {saveCapabilities.isPending ? t("common.saving") : t("common.save")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={resetTarget !== null}
        onOpenChange={(open) => {
          if (!open) setResetTarget(null);
        }}
        title={t("users.resetTitle", {
          email: resetTarget ? userIdent(resetTarget) : "",
        })}
        description={t("users.resetDesc")}
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
          email: deleteTarget ? userIdent(deleteTarget) : "",
        })}
        description={t("users.deleteConfirm")}
        confirmLabel={t("users.delete")}
        destructive
        pending={removeUser.isPending}
        onConfirm={() => {
          if (deleteTarget) removeUser.mutate(deleteTarget.id);
        }}
      />

      <SecretDialog
        open={secret !== null}
        onOpenChange={(open) => {
          if (!open) setSecret(null);
        }}
        title={secret?.title ?? ""}
        description={t("users.sharePassword")}
        secret={secret?.password ?? ""}
      />
    </div>
  );
}
