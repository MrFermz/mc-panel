"use client";

import { Loader2Icon } from "lucide-react";

// overlay ที่บล็อกทั้งหน้าระหว่างงานที่ "ห้ามแตะอะไรจนกว่าจะจบ" — ใช้เฉพาะงานที่ยิงหลาย
// request ต่อกันหรือรอ job บนโหนด (งาน request เดียวใช้ spinner ที่ปุ่ม/ConfirmDialog พอ
// ไม่ต้องบล็อกทั้งหน้า). progress = null → โชว์แค่ spinner ไม่มีแถบ
export function LoadingOverlay({
  title,
  description,
  progress = null,
}: {
  title: string;
  description?: string;
  progress?: number | null;
}) {
  return (
    <div
      role="status"
      aria-live="polite"
      aria-busy
      className="bg-background/80 fixed inset-0 z-[100] flex items-center justify-center backdrop-blur-sm"
    >
      <div className="bg-card mx-4 grid w-full max-w-sm gap-4 rounded-lg border p-6 shadow-xl">
        <div className="flex items-center gap-2">
          <Loader2Icon className="text-primary size-5 animate-spin" />
          <p className="font-medium">{title}</p>
        </div>
        {progress !== null && (
          <>
            <div className="bg-muted h-2 overflow-hidden rounded-full">
              <div
                className="bg-primary h-full transition-all"
                style={{ width: `${progress}%` }}
              />
            </div>
            <p className="text-muted-foreground text-sm tabular-nums">
              {progress}%
            </p>
          </>
        )}
        {description && (
          <p className="text-muted-foreground text-xs">{description}</p>
        )}
      </div>
    </div>
  );
}
