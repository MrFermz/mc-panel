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
// ANSI palette ต้องกำหนดเองทั้งชุด เพราะ colorizeLine ยิง SGR มาตรฐาน (31-36/90-93) มา
// แล้ว xterm map index → สีจาก theme ตอน render — สลับ theme บรรทัดเก่าจึงเปลี่ยนสีตามให้เอง
type XtermTheme = {
  background: string;
  foreground: string;
  cursor: string;
  selectionBackground: string;
  red: string;
  green: string;
  yellow: string;
  blue: string;
  magenta: string;
  cyan: string;
  brightBlack: string;
  brightRed: string;
  brightGreen: string;
  brightYellow: string;
};

const XTERM_THEMES: Record<ResolvedTheme, XtermTheme> = {
  dark: {
    background: "#101012",
    foreground: "#d4d4d8",
    cursor: "#d4d4d8",
    selectionBackground: "#3f3f46",
    red: "#f87171",
    green: "#4ade80",
    yellow: "#fbbf24",
    blue: "#60a5fa",
    magenta: "#c084fc",
    cyan: "#22d3ee",
    brightBlack: "#71717a",
    brightRed: "#fca5a5",
    brightGreen: "#86efac",
    brightYellow: "#fde68a",
  },
  light: {
    background: "#ffffff",
    foreground: "#27272a",
    cursor: "#27272a",
    selectionBackground: "#d4d4d8",
    red: "#dc2626",
    green: "#16a34a",
    yellow: "#b45309",
    blue: "#2563eb",
    magenta: "#9333ea",
    cyan: "#0891b2",
    brightBlack: "#71717a",
    brightRed: "#b91c1c",
    brightGreen: "#15803d",
    brightYellow: "#a16207",
  },
};

// ---- ANSI colorizer: ทำให้แต่ละบรรทัดใน console อ่านออกทันทีว่าเกิดอะไร ----
// ยิง SGR มาตรฐานเท่านั้น (สีจริงมาจาก palette ใน theme) → รองรับ dark/light + recolor ตอนสลับ theme
const SGR = {
  reset: "\x1b[0m",
  bold: "\x1b[1m",
  dim: "\x1b[2m",
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  magenta: "\x1b[35m",
  cyan: "\x1b[36m",
  gray: "\x1b[90m",
  brightRed: "\x1b[91m",
  brightGreen: "\x1b[92m",
} as const;

// [12:34:56] [Server thread/INFO]: message  — จับ timestamp / thread(level) / message
const LOG_LINE_RE = /^(\[\d{1,2}:\d{2}:\d{2}\])\s*(?:\[([^\]]+)\])?\s*:?\s?([\s\S]*)$/;

function levelSGR(level: string): string {
  switch (level) {
    case "ERROR":
    case "SEVERE":
    case "FATAL":
      return SGR.brightRed;
    case "WARN":
    case "WARNING":
      return SGR.yellow;
    case "DEBUG":
    case "TRACE":
      return SGR.gray;
    default:
      return ""; // INFO = สี foreground ปกติ
  }
}

// สีของ message เฉพาะบรรทัด INFO (WARN/ERROR ครอบทั้งบรรทัดไปแล้ว)
function colorizeInfoMessage(msg: string): string {
  if (/ (joined|joined the game)$/.test(msg))
    return `${SGR.green}${msg}${SGR.reset}`;
  if (/ left the game$/.test(msg)) return `${SGR.gray}${msg}${SGR.reset}`;
  if (/^Done \(/.test(msg))
    return `${SGR.brightGreen}${SGR.bold}${msg}${SGR.reset}`;
  const chat = /^<([^>]+)>\s([\s\S]*)$/.exec(msg);
  if (chat) return `${SGR.cyan}<${chat[1]}>${SGR.reset} ${chat[2]}`;
  return msg;
}

function colorizeLine(raw: string): string {
  // system line จาก agent (crash cleanup ฯลฯ) — เด่นแยกจาก log ของ server
  if (raw.startsWith("[mc-panel]"))
    return `${SGR.magenta}${SGR.bold}${raw}${SGR.reset}`;

  const m = LOG_LINE_RE.exec(raw);
  if (!m) return raw; // format แปลก = ปล่อยดิบ ไม่เดา
  const [, ts, thread, msg = ""] = m;
  const level = (thread?.split("/").pop() ?? "").toUpperCase();
  const lc = levelSGR(level);

  const tsPart = `${SGR.gray}${ts}${SGR.reset}`;
  const threadPart = thread ? ` ${SGR.dim}[${thread}]${SGR.reset}` : "";
  const sep = `${SGR.gray}:${SGR.reset}`;
  const msgPart = lc
    ? `${lc}${msg}${SGR.reset}`
    : colorizeInfoMessage(msg);
  return `${tsPart}${threadPart}${sep} ${msgPart}`;
}

function writeLine(term: Terminal, raw: string) {
  term.writeln(colorizeLine(raw));
}

export default function ServerConsole({
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
    termRef.current = term;

    // FitAddon.proposeDimensions() อ่าน _core._renderService.dimensions โดยไม่เช็คว่า
    // renderer พร้อมหรือยัง — ถ้า container ยังไม่มีขนาด/terminal ถูก dispose ไปแล้ว
    // (StrictMode remount, สลับหน้า) จะโยน "reading 'dimensions'" ทิ้ง ต้องกันทุกจุดที่เรียก fit
    const safeFit = () => {
      if (!termRef.current) return;
      if (container.clientWidth === 0 || container.clientHeight === 0) return;
      try {
        fit.fit();
      } catch {
        // renderer ยังไม่พร้อม — ปล่อยให้ ResizeObserver รอบถัดไป fit ให้เอง
      }
    };
    // รอ layout รอบแรกให้ container มีขนาดจริงก่อนค่อย fit
    const raf = requestAnimationFrame(safeFit);

    if (pendingRef.current.length > 0) {
      for (const line of pendingRef.current) writeLine(term, line);
      pendingRef.current = [];
    }

    const observer = new ResizeObserver(safeFit);
    observer.observe(container);

    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
      termRef.current = null;
      term.dispose();
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
            for (const line of event.lines) writeLine(term, line);
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
