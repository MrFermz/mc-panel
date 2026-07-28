"use client";

import Link from "next/link";
import { LockIcon } from "lucide-react";
import { useT } from "@/lib/i18n";
import { knownGameProfile } from "@/lib/games";
import type { MetaGame } from "@/lib/types";
import { cn } from "@/lib/utils";

// การ์ด 1 ใบ = 1 เกมในหน้าเลือกเกม (ปก + ชื่อ + คำอธิบายสั้น)
// เกมที่ user ไม่มีสิทธิ์สร้าง (can_create=false) ยังเห็นการ์ดอยู่แต่กดไม่ได้ —
// บอกไปตรง ๆ ว่าไม่มีสิทธิ์ ดีกว่าซ่อนจนดูเหมือนระบบไม่รองรับเกมนั้น
export function GameCard({ game }: { game: MetaGame }) {
  const t = useT();
  const profile = knownGameProfile(game.id);
  const locked = !game.can_create;

  const body = (
    <>
      <div className="bg-muted relative aspect-video w-full overflow-hidden">
        {profile ? (
          // eslint-disable-next-line @next/next/no-img-element -- ปกเป็น SVG static ใน public/ ไม่ต้อง optimize
          <img
            src={profile.coverSrc}
            alt=""
            className={cn(
              "size-full object-cover transition-transform duration-300",
              !locked && "group-hover:scale-105",
              locked && "grayscale",
            )}
          />
        ) : (
          <div className="text-muted-foreground flex size-full items-center justify-center text-3xl font-semibold">
            {game.label.slice(0, 1)}
          </div>
        )}
        {locked && (
          <span className="bg-background/85 text-muted-foreground absolute top-2 right-2 flex items-center gap-1 rounded-full px-2 py-1 text-xs font-medium">
            <LockIcon className="size-3" />
            {t("gamePicker.locked")}
          </span>
        )}
      </div>
      <div className="grid gap-1 p-4">
        <span className="font-semibold">{game.label}</span>
        <span className="text-muted-foreground line-clamp-2 text-sm">
          {profile ? t(profile.descriptionKey) : t("gamePicker.unknownGame")}
        </span>
        <span className="text-muted-foreground mt-1 text-xs">
          {locked
            ? t("gamePicker.lockedHint")
            : t("gamePicker.minMemory", { mb: game.min_memory_mb })}
        </span>
      </div>
    </>
  );

  const shell =
    "group bg-card text-card-foreground overflow-hidden rounded-xl border text-left transition-colors";

  if (locked) {
    return (
      <div aria-disabled className={cn(shell, "opacity-60")}>
        {body}
      </div>
    );
  }

  return (
    <Link
      href={`/servers/new/${game.id}`}
      className={cn(
        shell,
        "hover:border-primary focus-visible:ring-ring block focus-visible:ring-2 focus-visible:outline-none",
      )}
    >
      {body}
    </Link>
  );
}
