"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { BoxIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useAuthGuard } from "@/lib/use-auth-guard";
import { adminItems, visibleFor } from "@/components/layout/sidebar-nav";
import { EventsListener } from "@/components/layout/events-listener";
import { UserMenu } from "@/components/layout/user-menu";
import { PageTitle } from "@/components/layout/page-title";
import { BreadcrumbProvider } from "@/components/layout/breadcrumb-context";
import { Skeleton } from "@/components/ui/skeleton";

// หน้าระดับ panel ที่ไม่ผูกกับ "active server" (admin/*, /servers/new, /profile, /preferences)
// อยู่นอก route group (panel) จึงไม่มี sidebar — nav ของ admin เป็นแถบแนวนอนใต้ top bar แทน
export default function StandaloneLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  const user = useAuthGuard();
  const pathname = usePathname();
  const t = useT();

  if (!user) {
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

  const admins = visibleFor(adminItems, user);
  const showAdminNav = pathname.startsWith("/admin") && admins.length > 0;

  return (
    <BreadcrumbProvider>
      <EventsListener />
      <div className="flex min-h-screen flex-col">
        <header className="bg-background/80 sticky top-0 z-40 border-b backdrop-blur">
          <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-3 px-4 md:px-6">
            <Link href="/" className="flex shrink-0 items-center gap-2">
              <BoxIcon className="size-5 shrink-0" />
              <span className="hidden font-semibold sm:inline">mc-panel</span>
            </Link>
            <span className="text-muted-foreground/50" aria-hidden>
              /
            </span>
            <PageTitle />
            <div className="ml-auto shrink-0">
              <UserMenu user={user} />
            </div>
          </div>
          {showAdminNav && (
            <nav className="mx-auto flex w-full max-w-6xl gap-1 overflow-x-auto px-4 pb-2 md:px-6">
              {admins.map((item) => {
                const active = pathname.startsWith(item.href);
                const Icon = item.icon;
                return (
                  <Link
                    key={item.href}
                    href={item.href}
                    className={cn(
                      "flex shrink-0 items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                      active
                        ? "bg-accent text-accent-foreground"
                        : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
                    )}
                  >
                    <Icon className="size-4 shrink-0" />
                    {t(item.labelKey)}
                  </Link>
                );
              })}
            </nav>
          )}
        </header>
        <main className="mx-auto w-full max-w-6xl flex-1 p-4 md:p-6">
          {children}
        </main>
      </div>
    </BreadcrumbProvider>
  );
}
