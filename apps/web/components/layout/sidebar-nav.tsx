"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  ArrowLeftIcon,
  FolderIcon,
  HardDriveIcon,
  KeyIcon,
  LayoutDashboardIcon,
  ScrollTextIcon,
  ServerIcon,
  SettingsIcon,
  TerminalIcon,
  UsersIcon,
} from "lucide-react";
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
// เมนูที่ทำงานกับ "active server" (เลือกจาก switcher) — ไม่ผูก id ใน URL
export const mainItems: NavItem[] = [
  { href: "/dashboard", labelKey: "nav.dashboard", icon: LayoutDashboardIcon },
  { href: "/console", labelKey: "tab.console", icon: TerminalIcon },
  { href: "/players", labelKey: "tab.players", icon: UsersIcon },
  { href: "/files", labelKey: "tab.files", icon: FolderIcon },
  { href: "/access", labelKey: "tab.access", icon: KeyIcon },
  { href: "/logs", labelKey: "nav.logs", icon: ScrollTextIcon },
  { href: "/settings", labelKey: "tab.settings", icon: SettingsIcon },
];

export const adminItems: NavItem[] = [
  {
    href: "/admin/servers",
    labelKey: "nav.allServers",
    icon: ServerIcon,
    capability: CAPABILITY.serversViewAll,
  },
  {
    href: "/admin/users",
    labelKey: "nav.users",
    icon: UsersIcon,
    capability: CAPABILITY.usersView,
  },
  {
    href: "/admin/nodes",
    labelKey: "nav.nodes",
    icon: HardDriveIcon,
    capability: CAPABILITY.nodesView,
  },
];

export function visibleFor(items: NavItem[], user: NavUser): NavItem[] {
  return items.filter(
    (item) => !item.capability || hasCapability(user, item.capability),
  );
}

function SectionLabel({
  children,
  className,
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "text-muted-foreground mb-1 px-3 text-xs font-semibold tracking-wider uppercase",
        className,
      )}
    >
      {children}
    </div>
  );
}

function NavLink({ item }: { item: NavItem }) {
  const pathname = usePathname();
  const t = useT();
  const active = item.exact
    ? pathname === item.href
    : pathname.startsWith(item.href);
  const Icon = item.icon;
  return (
    <Link
      href={item.href}
      className={cn(
        "flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors",
        active
          ? "bg-accent text-accent-foreground"
          : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
      )}
    >
      <Icon className="size-4 shrink-0" />
      <span className="truncate">{t(item.labelKey)}</span>
    </Link>
  );
}

// เปลี่ยน server ทำที่หน้า list (`/`) ที่เดียว — sidebar จึงเหลือแค่ทางกลับออกไป
// (ชื่อ/สถานะของ server ที่กำลังจัดการอยู่บน top bar หน้า title แล้ว)
function BackToServers() {
  const t = useT();
  return (
    <Link
      href="/"
      className="text-muted-foreground hover:bg-accent/50 hover:text-foreground flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors"
    >
      <ArrowLeftIcon className="size-4 shrink-0" />
      <span className="truncate">{t("nav.backToServers")}</span>
    </Link>
  );
}

export function SidebarNav({ user }: { user: NavUser }) {
  const t = useT();
  const admins = visibleFor(adminItems, user);
  // ลำดับ section: ปุ่ม server ปัจจุบัน/กลับไปหน้า list → General (เมนูที่ทำงานกับ active server)
  // → Admin
  return (
    <nav className="flex flex-col gap-1 p-3">
      <BackToServers />
      <SectionLabel className="mt-4">{t("nav.general")}</SectionLabel>
      {visibleFor(mainItems, user).map((item) => (
        <NavLink key={item.href} item={item} />
      ))}
      {admins.length > 0 && (
        <>
          <SectionLabel className="mt-4">{t("nav.admin")}</SectionLabel>
          {admins.map((item) => (
            <NavLink key={item.href} item={item} />
          ))}
        </>
      )}
    </nav>
  );
}
