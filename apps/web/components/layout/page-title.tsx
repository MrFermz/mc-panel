"use client";

import { useT } from "@/lib/i18n";
import {
  useBreadcrumbs,
  usePageServer,
} from "@/components/layout/breadcrumb-context";
import { StatusDot } from "@/components/status-badge";

// ชื่อหน้าปัจจุบัน = label ตัวท้ายของ trail ที่หน้าประกาศ (ไม่มี = Dashboard) — นำหน้าด้วย
// server ที่หน้าผูกไว้ (ถ้ามี) เป็น trail สั้น ๆ `● ชื่อ server / ชื่อหน้า`; หน้าระดับ panel
// (admin/*, profile, preferences) ไม่ผูก server จึงเหลือแค่ชื่อหน้า
export function PageTitle() {
  const t = useT();
  const items = useBreadcrumbs();
  const pageServer = usePageServer();
  const title = items.at(-1)?.label ?? t("nav.dashboard");
  return (
    <div className="flex min-w-0 items-center gap-2">
      {pageServer && (
        <>
          <StatusDot status={pageServer.server.status} />
          <span className="text-muted-foreground max-w-40 truncate text-base font-medium sm:max-w-64">
            {pageServer.server.name}
          </span>
          <span className="text-muted-foreground/50" aria-hidden>
            /
          </span>
        </>
      )}
      <h1 className="truncate text-base font-semibold">{title}</h1>
    </div>
  );
}
