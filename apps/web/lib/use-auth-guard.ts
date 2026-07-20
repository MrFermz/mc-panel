"use client";

import * as React from "react";
import { useMe } from "@/lib/use-me";
import type { User } from "@/lib/types";

// ทุก layout ที่ต้อง login ใช้ตัวนี้ร่วมกัน — /api/auth/me เป็น endpoint ที่ยกเว้น
// password_change_required เลยต้องเช็คแล้วบังคับ redirect เองที่ฝั่ง client
// คืน null = ยังโหลดไม่เสร็จ/ยังเข้าไม่ได้ (ผู้เรียกโชว์ skeleton แทน)
export function useAuthGuard(): User | null {
  const { data } = useMe();
  const user = data?.user;

  React.useEffect(() => {
    if (user?.must_change_password) {
      window.location.assign("/change-password");
    }
  }, [user?.must_change_password]);

  if (!user || user.must_change_password) return null;
  return user;
}
