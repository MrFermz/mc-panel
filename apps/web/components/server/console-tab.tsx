"use client";

import * as React from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { useServerConsole, type ConsoleEvent } from "@/lib/use-server-console";
import { useConsoleHistoryStore } from "@/lib/console-history-store";
import { useTheme, type ResolvedTheme } from "@/lib/settings/theme";
import { useT } from "@/lib/i18n";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { SendHorizontalIcon } from "lucide-react";

// สีจูนให้เข้ากับ token ของ globals.css (xterm ไม่รู้จัก CSS variables) — มีทั้ง dark/light
const XTERM_THEMES: Record<
  ResolvedTheme,
  { background: string; foreground: string; cursor: string; selectionBackground: string }
> = {
  dark: {
    background: "#101012",
    foreground: "#d4d4d8",
    cursor: "#d4d4d8",
    selectionBackground: "#3f3f46",
  },
  light: {
    background: "#ffffff",
    foreground: "#27272a",
    cursor: "#27272a",
    selectionBackground: "#d4d4d8",
  },
};

export default function ConsoleTab({
  serverId,
  canWrite,
}: {
  serverId: string;
  canWrite: boolean;
}) {
  const queryClient = useQueryClient();
  const t = useT();
  const { resolvedTheme } = useTheme();
  const xtermTheme = XTERM_THEMES[resolvedTheme];
  const containerRef = React.useRef<HTMLDivElement | null>(null);
  const termRef = React.useRef<Terminal | null>(null);
  // บรรทัดที่มาก่อน terminal พร้อม (WS เปิดเร็วกว่า effect แรกได้) เก็บพักไว้ก่อน
  const pendingRef = React.useRef<string[]>([]);

  const [input, setInput] = React.useState("");
  const historyIndexRef = React.useRef<number | null>(null);
  const history = useConsoleHistoryStore(
    (s) => s.history[serverId] ?? EMPTY_HISTORY,
  );
  const pushHistory = useConsoleHistoryStore((s) => s.push);

  React.useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const term = new Terminal({
      convertEol: true,
      disableStdin: true,
      cursorBlink: false,
      fontSize: 12.5,
      fontFamily:
        "ui-monospace, SFMono-Regular, Menlo, Consolas, 'Liberation Mono', monospace",
      scrollback: 5_000,
      theme: XTERM_THEMES[resolvedTheme],
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(container);
    fit.fit();
    termRef.current = term;

    if (pendingRef.current.length > 0) {
      for (const line of pendingRef.current) term.writeln(line);
      pendingRef.current = [];
    }

    const observer = new ResizeObserver(() => {
      try {
        fit.fit();
      } catch {
        // fit พังได้ตอน container ถูกซ่อน (สลับ tab) — ปล่อยผ่าน
      }
    });
    observer.observe(container);

    return () => {
      observer.disconnect();
      term.dispose();
      termRef.current = null;
    };
    // สร้าง terminal ครั้งเดียว — theme sync แยกใน effect ด้านล่าง
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ปรับสี terminal ตาม theme ปัจจุบัน (dark/light) โดยไม่ต้องสร้างใหม่
  React.useEffect(() => {
    if (termRef.current) {
      termRef.current.options.theme = XTERM_THEMES[resolvedTheme];
    }
  }, [resolvedTheme]);

  const onEvent = React.useCallback(
    (event: ConsoleEvent) => {
      switch (event.type) {
        case "open":
          // server ส่ง history ใหม่ทุกครั้งที่ต่อ — เคลียร์ของเก่ากัน log ซ้ำหลัง reconnect
          termRef.current?.clear();
          pendingRef.current = [];
          break;
        case "lines": {
          const term = termRef.current;
          if (term) {
            for (const line of event.lines) term.writeln(line);
          } else {
            pendingRef.current.push(...event.lines);
          }
          break;
        }
        case "status":
          queryClient.invalidateQueries({ queryKey: ["servers", serverId] });
          queryClient.invalidateQueries({ queryKey: ["servers"] });
          break;
        case "error":
          toast.error(event.message);
          break;
      }
    },
    [queryClient, serverId],
  );

  const { connected, sendCommand } = useServerConsole(serverId, onEvent);

  const submit = () => {
    const command = input.trim();
    if (command === "") return;
    if (!sendCommand(command)) {
      toast.error(t("console.notConnected"));
      return;
    }
    pushHistory(serverId, command);
    setInput("");
    historyIndexRef.current = null;
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      submit();
      return;
    }
    if (e.key === "ArrowUp") {
      e.preventDefault();
      if (history.length === 0) return;
      const idx =
        historyIndexRef.current === null
          ? history.length - 1
          : Math.max(0, historyIndexRef.current - 1);
      historyIndexRef.current = idx;
      setInput(history[idx] ?? "");
      return;
    }
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (historyIndexRef.current === null) return;
      const idx = historyIndexRef.current + 1;
      if (idx >= history.length) {
        historyIndexRef.current = null;
        setInput("");
      } else {
        historyIndexRef.current = idx;
        setInput(history[idx] ?? "");
      }
    }
  };

  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between">
        <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
          <span
            className={cn(
              "size-2 rounded-full",
              connected ? "bg-green-400" : "bg-red-400 animate-pulse",
            )}
          />
          {connected ? t("console.connected") : t("console.disconnected")}
        </span>
      </div>
      <div
        ref={containerRef}
        className="h-[28rem] w-full overflow-hidden rounded-md border p-2"
        style={{ backgroundColor: xtermTheme.background }}
      />
      {canWrite && (
        <div className="flex gap-2">
          <Input
            value={input}
            onChange={(e) => {
              setInput(e.target.value);
              historyIndexRef.current = null;
            }}
            onKeyDown={onKeyDown}
            disabled={!connected}
            placeholder={
              connected ? t("console.placeholder") : t("console.waiting")
            }
            className="font-mono text-sm"
            autoComplete="off"
            spellCheck={false}
          />
          <Button
            variant="secondary"
            size="icon"
            onClick={submit}
            disabled={!connected || input.trim() === ""}
            aria-label={t("console.send")}
          >
            <SendHorizontalIcon />
          </Button>
        </div>
      )}
    </div>
  );
}

const EMPTY_HISTORY: string[] = [];
