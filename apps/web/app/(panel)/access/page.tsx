"use client";

import ServerAccess from "@/components/server/server-access";
import { ServerPageShell } from "@/components/server/server-page-shell";

// สิทธิ์ต่อ server ของ active server (ย้ายออกมาจาก tab=access ของ /servers/[id])
// เจ้าของ server เท่านั้น — คนอื่นเห็น noAccess ผ่าน gate ของ shell
export default function AccessPage() {
  return (
    <ServerPageShell titleKey="tab.access" need={(ctx) => ctx.isOwner}>
      {({ server }) => <ServerAccess serverId={server.id} />}
    </ServerPageShell>
  );
}
