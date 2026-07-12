import type { User } from "@/lib/types";

// global RBAC keys (แยกจาก server_permissions) — sync กับ docs/api.md หัวข้อ Capabilities
export const CAPABILITY = {
  usersManage: "users.manage",
  nodesManage: "nodes.manage",
  serversCreate: "servers.create",
  serversViewAll: "servers.view_all",
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
