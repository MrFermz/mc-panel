import { CAPABILITY } from "@/lib/capabilities";
import type { TranslationKey } from "@/lib/i18n";
import type { User } from "@/lib/types";

// role preset = ทางลัดฝั่ง UI สำหรับติ๊ก capability ชุดที่ใช้บ่อย
// backend ไม่รู้จัก "role" — เก็บแค่ is_admin + capabilities ตามเดิม
export type RoleKey = "admin" | "operator" | "moderator" | "viewer" | "custom" | "none";

export interface RolePreset {
  key: Exclude<RoleKey, "custom" | "none">;
  isAdmin: boolean;
  capabilities: string[];
}

// operator = ดูแลเซิร์ฟเวอร์ได้เต็ม (ไม่รวมจัดการ user/node)
// moderator = คุมเกม (คอนโซล + ผู้เล่น) แต่แก้ไฟล์/ตั้งค่าไม่ได้
// viewer = อ่านอย่างเดียว
export const ROLE_PRESETS: RolePreset[] = [
  { key: "admin", isAdmin: true, capabilities: [] },
  {
    key: "operator",
    isAdmin: false,
    capabilities: [
      CAPABILITY.serversViewAll,
      CAPABILITY.serversCreate,
      CAPABILITY.serversEdit,
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
      CAPABILITY.accessView,
    ],
  },
  {
    key: "moderator",
    isAdmin: false,
    capabilities: [
      CAPABILITY.serversPower,
      CAPABILITY.consoleView,
      CAPABILITY.consoleWrite,
      CAPABILITY.playersView,
      CAPABILITY.playersManage,
      CAPABILITY.playersModerate,
      CAPABILITY.settingsView,
    ],
  },
  {
    key: "viewer",
    isAdmin: false,
    capabilities: [
      CAPABILITY.consoleView,
      CAPABILITY.filesView,
      CAPABILITY.playersView,
      CAPABILITY.settingsView,
      CAPABILITY.accessView,
    ],
  },
];

export const ROLE_LABEL_KEYS: Record<RoleKey, TranslationKey> = {
  admin: "users.role.admin",
  operator: "users.role.operator",
  moderator: "users.role.moderator",
  viewer: "users.role.viewer",
  custom: "users.role.custom",
  none: "users.role.none",
};

function sameSet(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((k) => b.includes(k));
}

// role ที่แสดง = preset ที่ตรงเป๊ะ ไม่งั้น custom (มีสิทธิ์แต่ไม่ตรง preset) / none (ไม่มีเลย)
export function matchPreset(isAdmin: boolean, capabilities: string[]): RoleKey {
  if (isAdmin) return "admin";
  const match = ROLE_PRESETS.find(
    (p) => !p.isAdmin && sameSet(p.capabilities, capabilities),
  );
  if (match) return match.key;
  return capabilities.length === 0 ? "none" : "custom";
}

export function detectRole(
  user: Pick<User, "is_admin" | "capabilities">,
): RoleKey {
  return matchPreset(user.is_admin, user.capabilities);
}
