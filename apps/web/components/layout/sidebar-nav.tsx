"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { LayoutDashboardIcon, ServerIcon, UsersIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import type { User } from "@/lib/types";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useT, type TranslationKey } from "@/lib/i18n";

export type NavUser = Pick<User, "is_admin" | "capabilities">;

export interface NavItem {
  href: string;
  labelKey: TranslationKey;
  icon: React.ComponentType<{ className?: string }>;
  exact?: boolean;
  // undefined = แสดงเสมอ, ไม่งั้น gate ด้วย effective capability
  capability?: string;
}

// source of truth เดียวของเมนู nav — desktop sidebar + mobile drawer map จากชุดเดียวกัน (ห้าม drift)
export const mainItems: NavItem[] = [
  { href: "/", labelKey: "nav.dashboard", icon: LayoutDashboardIcon, exact: true },
];

export const adminItems: NavItem[] = [
  {
    href: "/admin/users",
    labelKey: "nav.users",
    icon: UsersIcon,
    capability: CAPABILITY.usersManage,
  },
  {
    href: "/admin/nodes",
    labelKey: "nav.nodes",
    icon: ServerIcon,
    capability: CAPABILITY.nodesManage,
  },
];

export function visibleFor(items: NavItem[], user: NavUser): NavItem[] {
  return items.filter(
    (item) => !item.capability || hasCapability(user, item.capability),
  );
}

// เมื่อ collapsed: label ซ่อนไว้จนกว่า aside จะ hover/focus (group บน aside คุมการกาง)
function labelClass(collapsed: boolean): string {
  return cn(
    "truncate",
    collapsed && "hidden group-hover:inline",
  );
}

function NavLink({
  item,
  collapsed,
}: {
  item: NavItem;
  collapsed: boolean;
}) {
  const pathname = usePathname();
  const t = useT();
  const active = item.exact
    ? pathname === item.href
    : pathname.startsWith(item.href);
  const Icon = item.icon;
  return (
    <Link
      href={item.href}
      title={collapsed ? t(item.labelKey) : undefined}
      className={cn(
        "flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors",
        active
          ? "bg-accent text-accent-foreground"
          : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
      )}
    >
      <Icon className="size-4 shrink-0" />
      <span className={labelClass(collapsed)}>{t(item.labelKey)}</span>
    </Link>
  );
}

export function SidebarNav({
  user,
  collapsed = false,
}: {
  user: NavUser;
  collapsed?: boolean;
}) {
  const t = useT();
  const admins = visibleFor(adminItems, user);
  return (
    <nav className="flex flex-col gap-1 p-3">
      {visibleFor(mainItems, user).map((item) => (
        <NavLink key={item.href} item={item} collapsed={collapsed} />
      ))}
      {admins.length > 0 && (
        <>
          <div
            className={cn(
              "text-muted-foreground mt-4 mb-1 px-3 text-xs font-semibold tracking-wider uppercase",
              collapsed && "hidden group-hover:block",
            )}
          >
            {t("nav.admin")}
          </div>
          {admins.map((item) => (
            <NavLink key={item.href} item={item} collapsed={collapsed} />
          ))}
        </>
      )}
    </nav>
  );
}
