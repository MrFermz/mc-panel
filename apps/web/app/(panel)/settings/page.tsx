"use client";

import ServerSettings from "@/components/server/server-settings";
import { ServerPageShell } from "@/components/server/server-page-shell";
import { CAPABILITY } from "@/lib/capabilities";

// server settings ของ active server — การ์ดแต่ละใบ gate ด้วย cap ต่อ server:
// runtime (rename/RAM/port) = servers.edit, ลบ = servers.delete, properties = settings.view/edit
export default function ServerSettingsPage() {
  return (
    <ServerPageShell
      titleKey="tab.settings"
      need={(ctx) =>
        ctx.can(CAPABILITY.settingsView) ||
        ctx.can(CAPABILITY.serversEdit) ||
        ctx.can(CAPABILITY.serversDelete)
      }
    >
      {({ server, can }) => (
        <ServerSettings
          server={server}
          canEdit={can(CAPABILITY.serversEdit)}
          canDelete={can(CAPABILITY.serversDelete)}
          canViewProps={can(CAPABILITY.settingsView)}
          canEditProps={can(CAPABILITY.settingsEdit)}
        />
      )}
    </ServerPageShell>
  );
}
