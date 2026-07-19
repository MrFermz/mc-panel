"use client";

import ServerFiles from "@/components/server/server-files";
import { ServerPageShell } from "@/components/server/server-page-shell";

// file manager ของ active server (ย้ายออกมาจาก tab=files ของ /servers/[id])
export default function FilesPage() {
  return (
    <ServerPageShell titleKey="tab.files" need={(ctx) => ctx.canManageFiles}>
      {({ server }) => <ServerFiles serverId={server.id} />}
    </ServerPageShell>
  );
}
