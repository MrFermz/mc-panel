"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

// nav ของหน้า detail ต่อ user — สองแท็บนี้จัดการคนละชั้นของ RBAC:
// Permissions = capability ระดับ panel (ทำฟีเจอร์ไหนได้บ้าง)
// Server access = server_permissions (ทำกับ server ตัวไหนได้บ้าง)
export function UserDetailTabs({ userId }: { userId: string }) {
  const t = useT();
  const pathname = usePathname();

  const tabs = [
    {
      href: `/admin/users/${userId}/permissions`,
      label: t("users.permissions"),
    },
    { href: `/admin/users/${userId}/servers`, label: t("users.serverAccess") },
  ];

  return (
    <div className="flex gap-1 border-b">
      {tabs.map((tab) => {
        const active = pathname === tab.href;
        return (
          <Link
            key={tab.href}
            href={tab.href}
            className={cn(
              "-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors",
              active
                ? "border-primary text-foreground"
                : "text-muted-foreground hover:text-foreground border-transparent",
            )}
          >
            {tab.label}
          </Link>
        );
      })}
    </div>
  );
}
