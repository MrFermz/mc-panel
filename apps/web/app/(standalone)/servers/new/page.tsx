"use client";

import * as React from "react";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { Skeleton } from "@/components/ui/skeleton";
import { GameCard } from "@/components/server/new-server/game-card";
import { useMetaGames } from "@/components/server/new-server/use-meta-games";

// หน้าแรกของการสร้าง server: เลือกเกมก่อน แล้วค่อยเข้า wizard ของเกมนั้น
// (/servers/new/{game}) — ลำดับ step และฟอร์มของแต่ละเกมไม่เหมือนกัน จึงเลือกก่อนเริ่มกรอก
export default function NewServerGamePage() {
  const t = useT();
  const me = useMe().data?.user;
  const { games, pending, error } = useMetaGames();

  useSetBreadcrumbs(
    React.useMemo(() => [{ label: t("wizard.newBreadcrumb") }], [t]),
  );

  // กันเข้าตรง URL — ปุ่มที่พามาที่นี่ซ่อนตาม servers.create อยู่แล้ว
  if (me && !hasCapability(me, CAPABILITY.serversCreate)) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>
    );
  }

  return (
    <div className="grid gap-6">
      <div className="grid gap-1">
        <h1 className="text-xl font-semibold">{t("gamePicker.title")}</h1>
        <p className="text-muted-foreground text-sm">
          {t("gamePicker.subtitle")}
        </p>
      </div>

      {pending ? (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-64 w-full rounded-xl" />
          ))}
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">{t("gamePicker.failedLoad")}</p>
      ) : games.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t("gamePicker.empty")}</p>
      ) : (
        <>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {games.map((game) => (
              <GameCard key={game.id} game={game} />
            ))}
          </div>
          {/* มีสิทธิ์สร้าง server แต่ยังไม่มีเกมไหนให้เลย — บอกว่าต้องไปขอสิทธิ์อะไร */}
          {games.every((g) => !g.can_create) && (
            <p className="text-muted-foreground text-sm">
              {t("gamePicker.noGameAccess")}
            </p>
          )}
        </>
      )}
    </div>
  );
}
