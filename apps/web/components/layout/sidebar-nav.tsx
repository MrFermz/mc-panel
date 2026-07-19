"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import {
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
import { apiGet } from "@/lib/api";
import type { User } from "@/lib/types";
import { serversResponseSchema } from "@/lib/types";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useT, type TranslationKey } from "@/lib/i18n";
import { useSettingsStore } from "@/lib/settings/store";
import { ServerSwitcher } from "@/components/server/server-switcher";

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
  { href: "/", labelKey: "nav.dashboard", icon: LayoutDashboardIcon, exact: true },
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

// quick-switcher เลือก server ที่ดูภาพรวมอยู่ (เก็บใน dashboardServerId) — เลือกแล้ว
// ตั้งเป็น active เลย ไม่เปลี่ยน route. /api/servers filter ตามสิทธิ์ให้แล้ว
// (owner/permission หรือ servers.view_all) และ share cache ["servers"] กับ dashboard/WS
function ServerQuickSwitch() {
  const t = useT();
  const dashboardServerId = useSettingsStore((s) => s.dashboardServerId);
  const setDashboardServerId = useSettingsStore((s) => s.setDashboardServerId);

  const { data } = useQuery({
    queryKey: ["servers"],
    queryFn: () => apiGet("/api/servers", serversResponseSchema),
  });
  const servers = data?.servers ?? [];

  // ค่าที่โชว์ = server ที่เลือกไว้ถ้ายังมีอยู่ ไม่งั้น fallback ตัวแรก (ตรงกับ dashboard)
  const value =
    dashboardServerId && servers.some((s) => s.id === dashboardServerId)
      ? dashboardServerId
      : servers[0]?.id ?? "";

  return (
    <div className="grid gap-1">
      <SectionLabel>{t("nav.servers")}</SectionLabel>
      {servers.length === 0 ? (
        // ยังไม่มี server ที่ต้องดูแล — โชว์ section ไว้เสมอแต่แจ้งสถานะแทน dropdown
        <p className="text-muted-foreground px-3 py-1 text-xs">
          {t("nav.noServers")}
        </p>
      ) : (
        <div className="px-1">
          <ServerSwitcher
            servers={servers}
            value={value}
            onSelect={setDashboardServerId}
          />
        </div>
      )}
    </div>
  );
}

export function SidebarNav({ user }: { user: NavUser }) {
  const t = useT();
  const admins = visibleFor(adminItems, user);
  // ลำดับ section: My Servers (บริบทที่ใช้บ่อยสุด) → General (เมนูระดับ panel เช่น Dashboard)
  // → Admin — เมนูใหม่ระดับ panel เพิ่มใน mainItems, เมนูผูกกับ server ที่เลือกไปไว้ใต้ switcher
  return (
    <nav className="flex flex-col gap-1 p-3">
      <ServerQuickSwitch />
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
