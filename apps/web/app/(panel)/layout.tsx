"use client";

import * as React from "react";
import Link from "next/link";
import {
  BoxIcon,
  MenuIcon,
  PanelLeftCloseIcon,
  PanelLeftOpenIcon,
} from "lucide-react";
import { useMe } from "@/lib/use-me";
import type { User } from "@/lib/types";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useUiStore } from "@/lib/settings/ui-store";
import { useSettingsStore } from "@/lib/settings/store";
import {
  SidebarNav,
  mainItems,
  adminItems,
  visibleFor,
} from "@/components/layout/sidebar-nav";
import { EventsListener } from "@/components/layout/events-listener";
import { UserMenu } from "@/components/layout/user-menu";
import {
  BreadcrumbProvider,
  useBreadcrumbs,
} from "@/components/layout/breadcrumb-context";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { ChangePasswordDialog } from "@/components/change-password-dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

function MobileNav({ user }: { user: User }) {
  const t = useT();
  // มิเรอร์ desktop SidebarNav จาก source เดียวกัน (Dashboard + admin items) — ไม่มี "New Server"
  // (desktop จงใจไม่มี; สร้าง server ทำผ่านปุ่มในหน้า dashboard)
  const items = [...visibleFor(mainItems, user), ...visibleFor(adminItems, user)];
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="md:hidden"
          aria-label={t("nav.menu")}
        >
          <MenuIcon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        {items.map((item) => (
          <DropdownMenuItem key={item.href} asChild>
            <Link href={item.href}>{t(item.labelKey)}</Link>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function PanelBreadcrumbs() {
  const t = useT();
  const items = useBreadcrumbs();
  // root "Dashboard" นำหน้าเสมอ แล้วต่อด้วย trail ที่หน้าประกาศ
  const trail = [
    { label: t("breadcrumb.dashboard"), href: "/" },
    ...items,
  ];
  return (
    <Breadcrumb className="hidden min-w-0 sm:flex">
      <BreadcrumbList className="flex-nowrap">
        {trail.map((item, i) => {
          const last = i === trail.length - 1;
          return (
            <React.Fragment key={`${item.label}-${i}`}>
              <BreadcrumbItem className="min-w-0">
                {last || !item.href ? (
                  <BreadcrumbPage className="truncate">
                    {item.label}
                  </BreadcrumbPage>
                ) : (
                  <BreadcrumbLink asChild className="truncate">
                    <Link href={item.href}>{item.label}</Link>
                  </BreadcrumbLink>
                )}
              </BreadcrumbItem>
              {!last && <BreadcrumbSeparator />}
            </React.Fragment>
          );
        })}
      </BreadcrumbList>
    </Breadcrumb>
  );
}

function SidebarToggle({ collapsed }: { collapsed: boolean }) {
  const t = useT();
  const toggleSidebar = useSettingsStore((s) => s.toggleSidebar);
  return (
    <button
      type="button"
      onClick={toggleSidebar}
      aria-label={t(collapsed ? "nav.expandSidebar" : "nav.collapseSidebar")}
      aria-pressed={collapsed}
      className="text-muted-foreground hover:bg-accent/50 hover:text-foreground ml-auto flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors [&_svg]:size-4"
    >
      {collapsed ? <PanelLeftOpenIcon /> : <PanelLeftCloseIcon />}
    </button>
  );
}

export default function PanelLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const { data, isPending } = useMe();
  const user = data?.user;
  const changePasswordOpen = useUiStore((s) => s.changePasswordOpen);
  const setChangePasswordOpen = useUiStore((s) => s.setChangePasswordOpen);
  const collapsed = useSettingsStore((s) => s.sidebarCollapsed);

  React.useEffect(() => {
    // /api/auth/me เป็น endpoint ที่ยกเว้น password_change_required
    // เลยต้องเช็คแล้วบังคับ redirect เองที่นี่
    if (user?.must_change_password) {
      window.location.assign("/change-password");
    }
  }, [user?.must_change_password]);

  if (isPending || !user || user.must_change_password) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <div className="grid w-full max-w-md gap-3 p-6">
          <Skeleton className="h-8 w-40" />
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-24 w-full" />
        </div>
      </div>
    );
  }

  return (
    <BreadcrumbProvider>
    <EventsListener />
    <div className="flex min-h-screen">
      {/* spacer จองพื้นที่ layout ตามความกว้าง rail (aside เป็น fixed จึงต้องมีตัวนี้กันเนื้อหาโดนทับ) */}
      <div
        aria-hidden
        className={cn(
          "hidden shrink-0 transition-[width] duration-200 md:block",
          collapsed ? "w-16" : "w-56",
        )}
      />
      <aside
        className={cn(
          "group bg-card fixed inset-y-0 left-0 z-50 hidden h-screen flex-col border-r transition-[width] duration-200 md:flex",
          collapsed ? "w-16 hover:w-56 hover:shadow-xl" : "w-56",
        )}
      >
        <div className="flex h-14 items-center gap-2 border-b px-4">
          <BoxIcon
            className={cn(
              "size-5 shrink-0",
              collapsed && "hidden group-hover:block",
            )}
          />
          <Link
            href="/"
            className={cn(
              "truncate font-semibold",
              collapsed && "hidden group-hover:inline",
            )}
          >
            mc-panel
          </Link>
          <SidebarToggle collapsed={collapsed} />
        </div>
        <SidebarNav user={user} collapsed={collapsed} />
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="bg-background/80 sticky top-0 z-40 flex h-14 items-center justify-between border-b px-4 backdrop-blur">
          <div className="flex min-w-0 items-center gap-2">
            <MobileNav user={user} />
            <Link href="/" className="font-semibold md:hidden">
              mc-panel
            </Link>
            <PanelBreadcrumbs />
          </div>
          <UserMenu user={user} />
        </header>
        <main className="flex-1 p-4 md:p-6">{children}</main>
      </div>

      <ChangePasswordDialog
        open={changePasswordOpen}
        onOpenChange={setChangePasswordOpen}
      />
    </div>
    </BreadcrumbProvider>
  );
}
