"use client";

import dynamic from "next/dynamic";
import { LiveResources } from "@/components/server/live-resources";
import { ServerPageShell } from "@/components/server/server-page-shell";
import { CAPABILITY } from "@/lib/capabilities";
import { Skeleton } from "@/components/ui/skeleton";

// @xterm/xterm แตะ window ตั้งแต่ import — ต้องปิด SSR
const ServerConsole = dynamic(() => import("@/components/server/server-console"), {
  ssr: false,
  loading: () => <Skeleton className="h-[28rem] w-full" />,
});

export default function ConsolePage() {
  return (
    <ServerPageShell
      titleKey="tab.console"
      need={(ctx) => ctx.can(CAPABILITY.consoleView)}
    >
      {({ server, canConsoleWrite }) => (
        <div className="grid gap-4">
          <ServerConsole serverId={server.id} canWrite={canConsoleWrite} />
          {/* live resources อยู่ล่างสุดของหน้า console */}
          <LiveResources server={server} />
        </div>
      )}
    </ServerPageShell>
  );
}
