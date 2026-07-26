"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

// สีประจำผู้เล่น — เลือกจาก hash ของชื่อ ให้คนเดิมได้สีเดิมเสมอทุกหน้า/ทุก session
// ใช้เป็น fallback ตอนไม่มี uuid หรือดึงรูปไม่ได้ (เกมไม่มีรูปให้ / โปรไฟล์ไม่มี texture)
// tuple แบบไม่ว่าง — บอก TS ว่า PALETTE[0] มีจริงเสมอ (noUncheckedIndexedAccess เปิดอยู่)
const PALETTE: [string, ...string[]] = [
  "bg-red-400 text-red-950",
  "bg-orange-400 text-orange-950",
  "bg-amber-400 text-amber-950",
  "bg-emerald-400 text-emerald-950",
  "bg-cyan-400 text-cyan-950",
  "bg-indigo-400 text-indigo-950",
  "bg-fuchsia-400 text-fuchsia-950",
  "bg-lime-400 text-lime-950",
];

function paletteFor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = (hash * 31 + name.charCodeAt(i)) | 0;
  }
  return PALETTE[Math.abs(hash) % PALETTE.length] ?? PALETTE[0];
}

export function PlayerAvatar({
  name,
  serverId,
  uuid,
  className,
}: {
  name: string;
  // ต้องมีทั้งคู่ถึงจะดึงรูปได้ (control-plane เป็นคนดึงจาก identity service ของเกมแล้วเสิร์ฟที่นี่)
  // uuid ว่าง = ผู้เล่นที่เห็นจาก console แต่ยังไม่มีในไฟล์ไหน → ตกไปใช้ตัวอักษรย่อ
  serverId?: string;
  uuid?: string;
  className?: string;
}) {
  const [failed, setFailed] = React.useState(false);
  const letter = name.trim().charAt(0).toUpperCase() || "?";

  const avatarUrl =
    serverId && uuid && !failed
      ? `/api/servers/${serverId}/players/${encodeURIComponent(uuid)}/avatar`
      : null;

  // reset สถานะ error เมื่อ uuid เปลี่ยน (row เดิมกลายเป็นคนใหม่ตอน list refetch)
  React.useEffect(() => setFailed(false), [uuid]);

  if (avatarUrl) {
    return (
      // next/image ไม่ช่วยอะไรที่นี่: รูปมาจาก API ของเราเองที่ต้องส่ง cookie ไปด้วย
      // และ crop มาเป็นหน้าเล็ก ๆ แล้ว — optimizer จะเป็นแค่ hop เพิ่ม
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={avatarUrl}
        alt=""
        aria-hidden
        loading="lazy"
        decoding="async"
        onError={() => setFailed(true)}
        className={cn(
          "size-8 shrink-0 rounded object-cover [image-rendering:pixelated]",
          className,
        )}
      />
    );
  }

  return (
    <div
      aria-hidden
      className={cn(
        "flex size-8 shrink-0 items-center justify-center rounded text-sm font-bold",
        paletteFor(name),
        className,
      )}
    >
      {letter}
    </div>
  );
}
