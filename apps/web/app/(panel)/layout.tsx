"use client";

import * as React from "react";
import Link from "next/link";
import {
  BoxIcon,
  MenuIcon,
  PanelLeftCloseIcon,
  PanelLeftOpenIcon,
} from "lucide-react";
import type { User } from "@/lib/types";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useSettingsStore } from "@/lib/settings/store";
import {
  SidebarNav,
  mainItems,
  visibleFor,
} from "@/components/layout/sidebar-nav";
import { EventsListener } from "@/components/layout/events-listener";
import { UserMenu } from "@/components/layout/user-menu";
import { useAuthGuard } from "@/lib/use-auth-guard";
import {
  BreadcrumbProvider,
  usePageServer,
} from "@/components/layout/breadcrumb-context";
import { PageTitle } from "@/components/layout/page-title";
import { ServerHeaderControls } from "@/components/server/server-header-controls";
import { PageLoader } from "@/components/page-loader";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

function MobileNav({ user }: { user: User }) {
  const t = useT();
  // มิเรอร์ desktop SidebarNav จาก source เดียวกัน — admin ไม่อยู่ที่นี่ (อยู่ใน user menu)
  const items = visibleFor(mainItems, user);
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

// ปุ่มสั่งงานบน top bar — โชว์เฉพาะหน้าที่ผูก server ไว้และ user มีสิทธิ์สั่งงาน
function HeaderControls() {
  const pageServer = usePageServer();
  if (!pageServer || !pageServer.canOperate) return null;
  return <ServerHeaderControls server={pageServer.server} />;
}

function SidebarToggle({
  collapsed,
  className,
}: {
  collapsed: boolean;
  className?: string;
}) {
  const t = useT();
  const toggleSidebar = useSettingsStore((s) => s.toggleSidebar);
  return (
    <button
      type="button"
      onClick={toggleSidebar}
      aria-label={t(collapsed ? "nav.expandSidebar" : "nav.collapseSidebar")}
      aria-pressed={collapsed}
      className={cn(
        "text-muted-foreground hover:bg-accent/50 hover:text-foreground flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors [&_svg]:size-4",
        className,
      )}
    >
      {collapsed ? <PanelLeftOpenIcon /> : <PanelLeftCloseIcon />}
    </button>
  );
}

// โหมดย่อ = sidebar ออกไปนอกจอทั้งแผง แล้วกลับเข้ามาเป็น drawer เมื่อเมาส์แตะขอบซ้าย
// (เปิดค้างอยู่แล้ว = ไม่ต้องมี state นี้). ปิดแบบหน่วงเวลาเพราะ dropdown/select ใน sidebar
// render ผ่าน portal นอก <aside> — เมาส์ที่ย้ายไปกดเมนูนับเป็น mouseleave ทันที เลยต้อง
// เช็คว่ายังมี trigger ที่กางอยู่ไหมก่อนค่อยปิดจริง
function useHoverDrawer(enabled: boolean) {
  const [open, setOpen] = React.useState(false);
  const asideRef = React.useRef<HTMLElement | null>(null);
  const timer = React.useRef<number | null>(null);

  const cancelHide = React.useCallback(() => {
    if (timer.current !== null) {
      window.clearTimeout(timer.current);
      timer.current = null;
    }
  }, []);

  const show = React.useCallback(() => {
    cancelHide();
    setOpen(true);
  }, [cancelHide]);

  const scheduleHide = React.useCallback(() => {
    cancelHide();
    const tick = () => {
      timer.current = window.setTimeout(() => {
        if (asideRef.current?.querySelector('[aria-expanded="true"]')) {
          tick();
          return;
        }
        setOpen(false);
      }, 200);
    };
    tick();
  }, [cancelHide]);

  React.useEffect(() => cancelHide, [cancelHide]);
  // กลับไปโหมดเปิดค้าง = ทิ้ง state ของ drawer ไม่ให้ค้างมาโผล่รอบหน้า
  React.useEffect(() => {
    if (!enabled) {
      cancelHide();
      setOpen(false);
    }
  }, [enabled, cancelHide]);

  return { open: enabled && open, asideRef, show, scheduleHide, cancelHide };
}

export default function PanelLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const user = useAuthGuard();
  const collapsed = useSettingsStore((s) => s.sidebarCollapsed);
  const drawer = useHoverDrawer(collapsed);

  // useAuthGuard คืน null ทั้งตอนยังโหลดไม่เสร็จและตอนเข้าไม่ได้ — เคสหลัง middleware
  // เป็นคน redirect ที่นี่จึงเป็นสถานะ "รอ" ล้วน
  if (!user) return <PageLoader />;

  return (
    <BreadcrumbProvider>
    <EventsListener />
    <div className="flex min-h-screen">
      {/* spacer จองพื้นที่ layout ให้ aside (fixed) — ย่อแล้วกว้าง 0 คือคืนพื้นที่ให้เนื้อหาเต็ม */}
      <div
        aria-hidden
        className={cn(
          "hidden shrink-0 transition-[width] duration-200 md:block",
          collapsed ? "w-0" : "w-56",
        )}
      />
      {/* แถบรับ hover ที่ขอบซ้าย: จุดเดียวที่เรียก drawer กลับมาได้ตอน sidebar ออกไปนอกจอ */}
      {collapsed && (
        <div
          aria-hidden
          onMouseEnter={drawer.show}
          onMouseLeave={drawer.scheduleHide}
          className="fixed inset-y-0 left-0 z-50 hidden w-3 md:block"
        />
      )}
      <aside
        ref={drawer.asideRef}
        onMouseEnter={drawer.show}
        onMouseLeave={drawer.scheduleHide}
        className={cn(
          "bg-card fixed inset-y-0 left-0 z-50 hidden h-screen w-56 flex-col border-r transition-transform duration-200 md:flex",
          collapsed && !drawer.open && "-translate-x-full",
          collapsed && "shadow-xl",
        )}
      >
        <div className="flex h-14 items-center gap-2 border-b px-4">
          <BoxIcon className="size-5 shrink-0" />
          <Link href="/" className="truncate font-semibold">
            mc-panel
          </Link>
          <SidebarToggle collapsed={collapsed} className="ml-auto" />
        </div>
        <SidebarNav user={user} />
        {/* user menu ย้ายมาอยู่ล่างสุดของ navbar (desktop) — mobile ใช้ตัวใน top bar */}
        <div className="mt-auto border-t p-3">
          <UserMenu user={user} align="start" className="w-full justify-start" />
        </div>
      </aside>
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="bg-background/80 sticky top-0 z-40 flex h-14 items-center justify-between gap-2 border-b px-4 backdrop-blur">
          <div className="flex min-w-0 items-center gap-2">
            <MobileNav user={user} />
            <Link href="/" className="font-semibold md:hidden">
              mc-panel
            </Link>
            {/* ตอนย่อ sidebar หายไปทั้งแผง — ปุ่มกางต้องมีที่ยึดบน top bar ไม่งั้นเปิดคืนด้วย
                keyboard/touch ไม่ได้เลย (hover ขอบซ้ายใช้ได้แต่กับเมาส์) */}
            {collapsed && (
              <SidebarToggle collapsed className="-ml-1 hidden md:flex" />
            )}
            <PageTitle />
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <HeaderControls />
            <div className="md:hidden">
              <UserMenu user={user} />
            </div>
          </div>
        </header>
        <main className="flex-1 p-4 md:p-6">{children}</main>
      </div>
    </div>
    </BreadcrumbProvider>
  );
}
