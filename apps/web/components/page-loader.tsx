"use client";

import { Loader2Icon } from "lucide-react";
import { useT } from "@/lib/i18n";

// สถานะ "หน้ายังไม่พร้อมให้ใช้" เต็มจอ — auth guard ที่ยังไม่รู้ว่า user เป็นใคร,
// Suspense boundary ของหน้าที่ต้องรอ client hook. คนละเรื่องกับ LoadingOverlay
// ซึ่งคลุมทับเนื้อหาที่ render อยู่แล้วระหว่างงานที่กำลังทำ
export function PageLoader({ label }: { label?: string }) {
  const t = useT();
  return (
    <div
      role="status"
      aria-live="polite"
      className="text-muted-foreground flex min-h-screen flex-col items-center justify-center gap-3"
    >
      <Loader2Icon className="size-6 animate-spin" />
      <p className="text-sm">{label ?? t("common.loading")}</p>
    </div>
  );
}
