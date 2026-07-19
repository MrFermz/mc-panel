"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

// สีคงที่ต่อ user — hash จาก id (ไม่ใช่ index ในตาราง) เพื่อให้สีไม่สลับตอน filter/sort
const PALETTE = [
  "bg-amber-400 text-amber-950",
  "bg-emerald-400 text-emerald-950",
  "bg-cyan-400 text-cyan-950",
  "bg-violet-400 text-violet-950",
  "bg-rose-400 text-rose-950",
  "bg-sky-400 text-sky-950",
];

function paletteFor(seed: string): string {
  let hash = 0;
  for (let i = 0; i < seed.length; i++) hash = (hash * 31 + seed.charCodeAt(i)) | 0;
  return (
    PALETTE[Math.abs(hash) % PALETTE.length] ?? "bg-muted text-foreground"
  );
}

export function UserAvatar({
  seed,
  name,
  src,
  className,
}: {
  seed: string;
  name: string;
  // src = /api/users/{id}/avatar?v=... (มี cache-buster ใน URL แล้ว) — ไม่มีรูป = ตัวอักษรย่อ
  src?: string | null;
  className?: string;
}) {
  const initial = name.trim().charAt(0).toUpperCase() || "?";
  const [failed, setFailed] = React.useState(false);
  // รูปเปลี่ยน = ให้โอกาสโหลดใหม่ (ไม่งั้นค้างเป็นตัวอักษรย่อหลังอัปโหลดทับรูปที่เคยพัง)
  React.useEffect(() => setFailed(false), [src]);

  if (src && !failed) {
    return (
      // next/image ไม่ช่วยอะไรที่นี่: รูปมาจาก API ของเราเองที่ต้องส่ง cookie ไปด้วย
      // และย่อมาให้แล้วในขนาดจิ๋ว — optimizer จะเป็นแค่ hop เพิ่ม
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={src}
        alt=""
        aria-hidden
        onError={() => setFailed(true)}
        className={cn(
          "size-9 shrink-0 rounded-lg object-cover",
          className,
        )}
      />
    );
  }
  return (
    <span
      aria-hidden
      className={cn(
        "flex size-9 shrink-0 items-center justify-center rounded-lg text-sm font-semibold",
        paletteFor(seed),
        className,
      )}
    >
      {initial}
    </span>
  );
}
