"use client";

import ServerPlayers from "@/components/server/server-players";
import { ServerPageShell } from "@/components/server/server-page-shell";

// จัดการผู้เล่นของ active server (ย้ายออกมาจาก tab=players ของ /servers/[id])
// สิทธิ์เท่า file manager — whitelist rebuild เขียนไฟล์ผ่าน agent เหมือนกัน
export default function PlayersPage() {
  return (
    <ServerPageShell titleKey="tab.players" need={(ctx) => ctx.canManageFiles}>
      {({ server }) => (
        <ServerPlayers
          serverId={server.id}
          isRunning={server.status === "running"}
          onlineNames={server.stats?.online_players ?? []}
        />
      )}
    </ServerPageShell>
  );
}
