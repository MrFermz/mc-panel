"use client";

import * as React from "react";
import { useIsMutating } from "@tanstack/react-query";
import { useT } from "@/lib/i18n";
import { LoadingOverlay } from "@/components/loading-overlay";

// mutation ที่มี overlay ของตัวเอง (มี label บอก phase) — ต้องไม่ให้ตัวกลางซ้อนทับ
// ตั้ง mutationKey ให้ตรงกับค่านี้เพื่อกันตัวเองออกจาก global overlay
export const LOCAL_OVERLAY_KEY = "local-overlay";

// หน่วงก่อนโชว์ overlay — งานที่จบเร็วกว่านี้ให้เห็นแค่ spinner ที่ปุ่มพอ
// ไม่งั้นจอจะกะพริบเทาหนึ่งเฟรมทุกครั้งที่กดอะไรก็ตาม
const SHOW_AFTER_MS = 350;

// overlay กลางของทั้งแอป: เกาะกับจำนวน mutation ที่ยังวิ่งอยู่ของ react-query
// (ทุกปุ่มที่ยิง API ผ่าน useMutation จึงได้ overlay อัตโนมัติ ไม่ต้องต่อสายที่ call site)
export function GlobalLoading() {
  const t = useT();
  const active = useIsMutating({
    predicate: (m) => m.options.mutationKey?.[0] !== LOCAL_OVERLAY_KEY,
  });
  const [show, setShow] = React.useState(false);

  React.useEffect(() => {
    if (active === 0) {
      setShow(false);
      return;
    }
    const id = window.setTimeout(() => setShow(true), SHOW_AFTER_MS);
    return () => window.clearTimeout(id);
  }, [active]);

  if (!show) return null;
  return <LoadingOverlay title={t("common.working")} />;
}
