"use client";

import { cn } from "@/lib/utils";

// สีประจำผู้เล่น — เลือกจาก hash ของชื่อ ให้คนเดิมได้สีเดิมเสมอทุกหน้า/ทุก session
// (ไม่ใช้ skin renderer ภายนอกเพราะต้องมี uuid และเพิ่ม host ที่ต้องเชื่อใจ)
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

export function PlayerHead({
  name,
  className,
}: {
  name: string;
  className?: string;
}) {
  const letter = name.trim().charAt(0).toUpperCase() || "?";
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
