"use client";

import ServerPlayers from "@/components/server/server-players";
import { ServerPageShell } from "@/components/server/server-page-shell";
import { CAPABILITY } from "@/lib/capabilities";

// จัดการผู้เล่นของ active server — ดูได้ด้วย players.view, จัดการ whitelist = players.manage,
// op/kick/ban = players.moderate
export default function PlayersPage() {
  return (
    <ServerPageShell
      titleKey="tab.players"
      need={(ctx) => ctx.can(CAPABILITY.playersView)}
    >
      {({ server, can }) => (
        <ServerPlayers
          serverId={server.id}
          isRunning={server.status === "running"}
          onlineNames={server.stats?.online_players ?? []}
          canManage={can(CAPABILITY.playersManage)}
          canModerate={can(CAPABILITY.playersModerate)}
        />
      )}
    </ServerPageShell>
  );
}
