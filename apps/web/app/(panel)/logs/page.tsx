"use client";

import ServerJobs from "@/components/server/server-jobs";
import { ServerPageShell } from "@/components/server/server-page-shell";

// ประวัติ job (create/start/stop/kill/delete) ของ active server
// ย้ายออกมาจาก tab=jobs ของ /servers/[id]
export default function LogsPage() {
  return (
    <ServerPageShell titleKey="nav.logs">
      {({ server }) => <ServerJobs serverId={server.id} />}
    </ServerPageShell>
  );
}
