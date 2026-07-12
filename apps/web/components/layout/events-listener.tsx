"use client";

import { useEvents } from "@/lib/use-events";

// เปิด WS /ws/events หนึ่งเส้นตลอด session ของ panel (mount ที่ layout ครั้งเดียว ไม่ใช่ต่อหน้า)
export function EventsListener() {
  useEvents();
  return null;
}
