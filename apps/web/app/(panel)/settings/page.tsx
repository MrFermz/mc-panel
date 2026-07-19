"use client";

import ServerSettings from "@/components/server/server-settings";
import { ServerPageShell } from "@/components/server/server-page-shell";

// server settings ของ active server (ย้ายออกมาจาก tab=settings ของ /servers/[id])
// เจ้าของ server เท่านั้น — ค่าตั้งส่วนตัวของ user อยู่ที่ /preferences
export default function ServerSettingsPage() {
  return (
    <ServerPageShell titleKey="tab.settings" need={(ctx) => ctx.isOwner}>
      {({ server }) => <ServerSettings server={server} />}
    </ServerPageShell>
  );
}
