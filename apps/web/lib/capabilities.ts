import type { User } from "@/lib/types";

// global RBAC keys (แยกจาก server_permissions) — sync กับ catalog ใน
// control-plane `internal/httpapi/capabilities.go` + docs/api.md
// key = "{group}.{action}" ทุกตัว
export const CAPABILITY = {
  usersView: "users.view",
  usersCreate: "users.create",
  usersEdit: "users.edit",
  usersDelete: "users.delete",
  usersResetPassword: "users.reset_password",

  nodesView: "nodes.view",
  nodesCreate: "nodes.create",
  nodesDelete: "nodes.delete",

  serversViewAll: "servers.view_all",
  serversCreate: "servers.create",
  serversEdit: "servers.edit",
  serversDelete: "servers.delete",
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
