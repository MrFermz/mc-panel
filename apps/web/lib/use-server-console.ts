"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { consoleWsUrl } from "@/lib/ws";
import { consoleServerMessageSchema, type ConsoleServerMessage } from "@/lib/types";

export type ConsoleEvent =
  | { type: "open" }
  | ConsoleServerMessage;

const MAX_BACKOFF_MS = 30_000;
const BASE_BACKOFF_MS = 1_000;

// ต่อ WebSocket console ของ server หนึ่งตัว พร้อม reconnect แบบ exponential backoff
// onEvent ถูกเก็บใน ref เพื่อไม่ต้อง reconnect ทุกครั้งที่ callback เปลี่ยน identity
export function useServerConsole(
  serverId: string,
  onEvent: (event: ConsoleEvent) => void,
) {
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const onEventRef = useRef(onEvent);
  onEventRef.current = onEvent;

  useEffect(() => {
    let disposed = false;
    let attempt = 0;
    let timer: number | undefined;
    // socket ของ effect นี้เอง — ใช้ปิดตอน cleanup ตรง ๆ ไม่พึ่ง wsRef ที่ effect ใหม่ (StrictMode) อาจเขียนทับแล้ว
    let socket: WebSocket | null = null;

    const connect = () => {
      if (disposed) return;
      let ws: WebSocket;
      try {
        ws = new WebSocket(consoleWsUrl(serverId));
      } catch {
        scheduleReconnect();
        return;
      }
      socket = ws;
      wsRef.current = ws;

      ws.onopen = () => {
        if (wsRef.current !== ws) return;
        attempt = 0;
        setConnected(true);
        onEventRef.current({ type: "open" });
      };
      ws.onmessage = (ev: MessageEvent) => {
        if (wsRef.current !== ws) return;
        if (typeof ev.data !== "string") return;
        let raw: unknown;
        try {
          raw = JSON.parse(ev.data);
        } catch {
          return;
        }
        const parsed = consoleServerMessageSchema.safeParse(raw);
        if (parsed.success) {
          onEventRef.current(parsed.data);
        }
      };
      ws.onclose = () => {
        // เช็ค identity กัน onclose ของ socket เก่า (StrictMode/reconnect) null ทับ wsRef ของ socket ใหม่
        if (wsRef.current !== ws) return;
        wsRef.current = null;
        setConnected(false);
        scheduleReconnect();
      };
    };

    const scheduleReconnect = () => {
      if (disposed) return;
      const delay = Math.min(MAX_BACKOFF_MS, BASE_BACKOFF_MS * 2 ** attempt);
      attempt += 1;
      timer = window.setTimeout(connect, delay);
    };

    connect();

    return () => {
      disposed = true;
      if (timer !== undefined) window.clearTimeout(timer);
      socket?.close();
      if (wsRef.current === socket) wsRef.current = null;
    };
  }, [serverId]);

  const sendCommand = useCallback((command: string): boolean => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ type: "input", command }));
      return true;
    }
    return false;
  }, []);

  return { connected, sendCommand };
}
