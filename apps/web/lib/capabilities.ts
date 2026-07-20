import type { User } from "@/lib/types";

// global RBAC keys (แยกจาก server_permissions) — sync กับ catalog ใน
// control-plane `internal/httpapi/capabilities.go` + docs/api.md
// key = "{group}.{action}" ทุกตัว
export const CAPABILITY = {
  usersView: "users.view",
  usersCreate: "users.create",
  usersEdit: "users.edit",
  usersDelete: "users.delete",
  usersRestore: "users.restore",
  usersResetPassword: "users.reset_password",

  nodesView: "nodes.view",
  nodesCreate: "nodes.create",
  nodesDelete: "nodes.delete",

  serversViewAll: "servers.view_all",
  serversCreate: "servers.create",
  serversEdit: "servers.edit",
  serversDelete: "servers.delete",
  serversRestore: "servers.restore",
  serversPurge: "servers.purge",
  serversPower: "servers.power",

  consoleView: "console.view",
  consoleWrite: "console.write",

  filesView: "files.view",
  filesWrite: "files.write",
  filesDelete: "files.delete",

  playersView: "players.view",
  playersManage: "players.manage",
  playersModerate: "players.moderate",

  settingsView: "settings.view",
  settingsEdit: "settings.edit",

  accessView: "access.view",
  accessManage: "access.manage",
} as const;

export type CapabilityKey = (typeof CAPABILITY)[keyof typeof CAPABILITY];

// effective capability = is_admin ครอบทุกอย่าง ไม่งั้นเช็คใน list
export function hasCapability(
  user: Pick<User, "is_admin" | "capabilities"> | null | undefined,
  key: string,
): boolean {
  if (!user) return false;
  return user.is_admin || user.capabilities.includes(key);
}

// server-scoped capability = cap ที่ grant ได้ในระดับ server (ชั้น access) — ต้องตรงกับ
// serverScopedCaps ใน control-plane `internal/httpapi/capabilities.go`
export const SERVER_SCOPED_CAPABILITIES = [
  CAPABILITY.serversEdit,
  CAPABILITY.serversDelete,
  CAPABILITY.serversRestore,
  CAPABILITY.serversPurge,
  CAPABILITY.serversPower,
  CAPABILITY.consoleView,
  CAPABILITY.consoleWrite,
  CAPABILITY.filesView,
  CAPABILITY.filesWrite,
  CAPABILITY.filesDelete,
  CAPABILITY.playersView,
  CAPABILITY.playersManage,
  CAPABILITY.playersModerate,
  CAPABILITY.settingsView,
  CAPABILITY.settingsEdit,
] as const;

// effective per-server cap = enforce 2 ชั้นแบบ AND ให้ตรงกับ effectiveServerCap ฝั่ง backend:
// admin ครอบทุกอย่าง; ไม่งั้นต้องมี global cap (เพดาน) AND (owner หรือ grant ต่อ server มี cap)
export function effectiveServerCap(
  user: Pick<User, "is_admin" | "capabilities"> | null | undefined,
  perm: { role: string; capabilities: string[] } | undefined | null,
  cap: string,
): boolean {
  if (!user) return false;
  if (user.is_admin) return true;
  if (!user.capabilities.includes(cap)) return false;
  if (!perm) return false;
  if (perm.role === "owner") return true;
  return perm.capabilities.includes(cap);
}
