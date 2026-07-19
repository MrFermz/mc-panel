import { CAPABILITY } from "@/lib/capabilities";
import type { TranslationKey } from "@/lib/i18n";

// per-server role preset = ทางลัดฝั่ง UI สำหรับ grant capability ต่อ server ชุดที่ใช้บ่อย
// backend เก็บแค่ role (owner|member) + capabilities[] — ไม่รู้จัก "preset"
// owner   = superuser ต่อ server (ได้ทุก server-scoped cap โดยปริยาย + จัดการ access list)
// operator = ดูแล server ได้เต็ม ยกเว้นแก้โครงสร้าง (rename/RAM/port) กับลบ server
// moderator = คุมเกม (คอนโซล + ผู้เล่น) แต่แก้ไฟล์ไม่ได้
// viewer  = อ่านอย่างเดียว (view ทุกแท็บ)
export type ServerRoleKey =
  | "owner"
  | "operator"
  | "moderator"
  | "viewer"
  | "custom";

export interface ServerRolePreset {
  key: Exclude<ServerRoleKey, "custom">;
  role: "owner" | "member";
  capabilities: string[];
}

export const SERVER_ROLE_PRESETS: ServerRolePreset[] = [
  { key: "owner", role: "owner", capabilities: [] },
  {
    key: "operator",
    role: "member",
    capabilities: [
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
    ],
  },
  {
    key: "moderator",
    role: "member",
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
    role: "member",
    capabilities: [
      CAPABILITY.consoleView,
      CAPABILITY.filesView,
      CAPABILITY.playersView,
      CAPABILITY.settingsView,
    ],
  },
];

export const SERVER_ROLE_LABEL_KEYS: Record<ServerRoleKey, TranslationKey> = {
  owner: "access.roleOwner",
  operator: "access.roleOperator",
  moderator: "access.roleModerator",
  viewer: "access.roleViewer",
  custom: "access.roleCustom",
};

function sameSet(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((k) => b.includes(k));
}

// role ที่แสดง = preset ที่ตรงเป๊ะ ไม่งั้น custom (grant ราย cap ที่ไม่ตรง preset ไหน)
export function matchServerPreset(
  role: string,
  capabilities: string[],
): ServerRoleKey {
  if (role === "owner") return "owner";
  const match = SERVER_ROLE_PRESETS.find(
    (p) => p.role === "member" && sameSet(p.capabilities, capabilities),
  );
  return match ? match.key : "custom";
}
