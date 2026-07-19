"use client";

import ServerFiles from "@/components/server/server-files";
import { ServerPageShell } from "@/components/server/server-page-shell";
import { CAPABILITY } from "@/lib/capabilities";

// file manager ของ active server — เข้าดูได้ด้วย files.view, เขียน/ลบ gate ด้วย files.write/delete
export default function FilesPage() {
  return (
    <ServerPageShell
      titleKey="tab.files"
      need={(ctx) => ctx.can(CAPABILITY.filesView)}
    >
      {({ server, can }) => (
        <ServerFiles
          serverId={server.id}
          canWrite={can(CAPABILITY.filesWrite)}
          canDelete={can(CAPABILITY.filesDelete)}
        />
      )}
    </ServerPageShell>
  );
}
