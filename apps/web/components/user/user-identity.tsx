"use client";

import * as React from "react";
import Link from "next/link";
import type { PermissionRole } from "@/lib/types";
import type { RoleKey } from "@/lib/user-roles";
import { userIdent, userTitle } from "@/lib/user-display";
import { cn } from "@/lib/utils";
import { UserAvatar } from "@/components/user/user-avatar";
import { RoleBadge, ServerRoleBadge } from "@/components/user/role-badge";

const SIZE = {
  sm: { avatar: "size-8 rounded-lg text-xs", title: "text-sm" },
  md: { avatar: "size-9 rounded-lg text-sm", title: "text-sm" },
  lg: { avatar: "size-12 rounded-xl text-lg", title: "font-semibold" },
};

// identity block มาตรฐานของ user: avatar + username เป็นหลัก + role เป็น subtitle
// ใช้ตัวนี้ทุกที่ที่โชว์ user เพื่อให้เห็น role ทันทีโดยไม่ต้องเปิดหน้า permission
// (panelRole = สิทธิ์ระดับ panel, serverRole = สิทธิ์ต่อ server — ส่งมาตัวเดียวตาม context)
export function UserIdentity({
  user,
  panelRole,
  serverRole,
  size = "md",
  trailing,
  href,
  className,
}: {
  user: {
    id?: string;
    username?: string | null;
    display_name?: string | null;
    avatar_url?: string | null;
  };
  panelRole?: RoleKey;
  serverRole?: PermissionRole;
  size?: keyof typeof SIZE;
  trailing?: React.ReactNode;
  // href = ทำให้ชื่อกดเข้าหน้า detail ได้ (แนวเดียวกับชื่อ server ใน /admin/servers) —
  // ปุ่ม action ที่เหลือย้ายไปอยู่ใน more menu หมดแล้ว ชื่อจึงต้องเป็นทางเข้าหลัก
  href?: string;
  className?: string;
}) {
  const title = userTitle(user);
  const ident = userIdent(user);
  const s = SIZE[size];
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <UserAvatar
        seed={user.id ?? title}
        name={title}
        src={user.avatar_url}
        className={s.avatar}
      />
      <div className="grid gap-0.5">
        {/* username อยู่ใน title attr — subtitle ถูกจองไว้ให้ role แล้ว */}
        <span
          className={cn("flex items-center gap-2 font-medium", s.title)}
          title={ident !== title ? ident : undefined}
        >
          {href ? (
            <Link href={href} className="hover:underline">
              {title}
            </Link>
          ) : (
            title
          )}
          {trailing}
        </span>
        {panelRole && <RoleBadge role={panelRole} size="sm" />}
        {serverRole && <ServerRoleBadge role={serverRole} size="sm" />}
      </div>
    </div>
  );
}
