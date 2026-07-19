"use client";

import {
  CrownIcon,
  EyeIcon,
  GavelIcon,
  MinusCircleIcon,
  ShieldIcon,
  SlidersHorizontalIcon,
  WrenchIcon,
  type LucideIcon,
} from "lucide-react";
import { useT, type TranslationKey } from "@/lib/i18n";
import type { PermissionRole } from "@/lib/types";
import { ROLE_LABEL_KEYS, type RoleKey } from "@/lib/user-roles";
import { cn } from "@/lib/utils";

// สี+ไอคอนต่อ role ถูกกำหนดที่นี่ที่เดียว — badge ของ role ต้องหน้าตาเหมือนกันทุกหน้า
// (สีสื่อระดับสิทธิ์: ม่วง/คราม = คุมทั้ง panel, เขียว/เหลือง = คุม server, ฟ้า/เทา = อ่านอย่างเดียว)
interface RoleStyle {
  icon: LucideIcon;
  className: string;
  labelKey: TranslationKey;
}

const PANEL_ROLE_STYLE: Record<RoleKey, RoleStyle> = {
  admin: {
    icon: ShieldIcon,
    className: "border-indigo-500/30 bg-indigo-500/15 text-indigo-300",
    labelKey: ROLE_LABEL_KEYS.admin,
  },
  operator: {
    icon: WrenchIcon,
    className: "border-emerald-500/30 bg-emerald-500/15 text-emerald-300",
    labelKey: ROLE_LABEL_KEYS.operator,
  },
  moderator: {
    icon: GavelIcon,
    className: "border-amber-500/30 bg-amber-500/15 text-amber-300",
    labelKey: ROLE_LABEL_KEYS.moderator,
  },
  viewer: {
    icon: EyeIcon,
    className: "border-sky-500/30 bg-sky-500/15 text-sky-300",
    labelKey: ROLE_LABEL_KEYS.viewer,
  },
  custom: {
    icon: SlidersHorizontalIcon,
    className: "border-violet-500/30 bg-violet-500/15 text-violet-300",
    labelKey: ROLE_LABEL_KEYS.custom,
  },
  none: {
    icon: MinusCircleIcon,
    className: "border-muted-foreground/20 bg-muted text-muted-foreground",
    labelKey: ROLE_LABEL_KEYS.none,
  },
};

// role ต่อ server (server_permissions) — คนละ vocabulary กับ panel role
// owner ได้มงกุฎเพราะเป็นสิทธิ์ที่ถอดไม่ได้ ไม่ใช่ preset ที่ตั้งเอง
const SERVER_ROLE_STYLE: Record<PermissionRole, RoleStyle> = {
  owner: {
    icon: CrownIcon,
    className: "border-amber-500/30 bg-amber-500/15 text-amber-300",
    labelKey: "access.roleOwner",
  },
  operator: {
    icon: WrenchIcon,
    className: "border-emerald-500/30 bg-emerald-500/15 text-emerald-300",
    labelKey: "access.roleOperator",
  },
  viewer: {
    icon: EyeIcon,
    className: "border-sky-500/30 bg-sky-500/15 text-sky-300",
    labelKey: "access.roleViewer",
  },
};

// size="sm" ใช้ตอนเป็น subtitle ใต้ username, "md" ตอนยืนเดี่ยวเป็น badge ในหัวข้อ/ตาราง
function RoleChip({
  style,
  size = "md",
  className,
}: {
  style: RoleStyle;
  size?: "sm" | "md";
  className?: string;
}) {
  const t = useT();
  const Icon = style.icon;
  return (
    <span
      className={cn(
        "inline-flex w-fit items-center gap-1 rounded-md border font-medium",
        size === "sm" ? "px-1.5 py-0 text-[11px]" : "px-2 py-0.5 text-xs",
        style.className,
        className,
      )}
    >
      <Icon className={size === "sm" ? "size-3" : "size-3.5"} />
      {t(style.labelKey)}
    </span>
  );
}

export function RoleBadge({
  role,
  size,
  className,
}: {
  role: RoleKey;
  size?: "sm" | "md";
  className?: string;
}) {
  return (
    <RoleChip style={PANEL_ROLE_STYLE[role]} size={size} className={className} />
  );
}

export function ServerRoleBadge({
  role,
  size,
  className,
}: {
  role: PermissionRole;
  size?: "sm" | "md";
  className?: string;
}) {
  return (
    <RoleChip
      style={SERVER_ROLE_STYLE[role]}
      size={size}
      className={className}
    />
  );
}
