"use client";

import * as React from "react";
import { useRouter } from "next/navigation";

// การ์ด/แถวทั้งใบกดแล้วเข้า detail — คืน props ให้ element เป็น button ที่ accessible
// (ปุ่มข้างในต้อง stopPropagation เอง; keyboard เช็ค target === currentTarget กันซ้อน)
export function useServerNav(id: string) {
  const router = useRouter();
  const go = React.useCallback(
    () => router.push(`/servers/${id}`),
    [router, id],
  );
  return {
    role: "button" as const,
    tabIndex: 0,
    onClick: go,
    onKeyDown: (e: React.KeyboardEvent) => {
      if (e.target !== e.currentTarget) return;
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        go();
      }
    },
  };
}
