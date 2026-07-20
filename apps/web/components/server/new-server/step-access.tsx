"use client";

import ServerAccess from "@/components/server/server-access";
import type { Permission } from "@/lib/types";

// step 3 — สิทธิ์เข้าถึง (ข้ามได้) ใช้ ServerAccess ตัวเดียวกับหน้า Access ของ server จริง
// ในโหมด draft: เก็บลง state แล้ว apply หลังสร้างเสร็จ แถวของคนสร้างล็อกไว้
export function StepAccess({
  draft,
  onChange,
  selfUserId,
}: {
  draft: Permission[];
  onChange: (next: Permission[]) => void;
  selfUserId?: string;
}) {
  return (
    <ServerAccess
      draft={draft}
      onDraftChange={onChange}
      lockedUserId={selfUserId}
    />
  );
}
