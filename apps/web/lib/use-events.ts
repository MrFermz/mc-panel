"use client";

import { useEffect, useRef } from "react";
import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { eventsWsUrl } from "@/lib/ws";
import {
  eventsServerMessageSchema,
  type EventsServerMessage,
  type Server,
  type Node,
} from "@/lib/types";
import { useStatsHistoryStore, type StatPoint } from "@/lib/settings/stats-history";

const MAX_BACKOFF_MS = 30_000;
const BASE_BACKOFF_MS = 1_000;

// shape ของ cache แต่ละ key ที่ event นี้ patch (ตรงกับ schema ใน lib/types.ts)
type ServersCache = { servers: Server[] } | undefined;
type ServerDetailCache = { server: Server } | undefined;
type NodesCache = { nodes: Node[] } | undefined;

type StatsHistoryApi = {
  push: (serverId: string, point: StatPoint) => void;
  reset: (serverId: string) => void;
};

function toStatPoint(stats: NonNullable<Server["stats"]>): StatPoint {
  return {
    t: new Date(stats.updated_at).getTime() || Date.now(),
    cpu: stats.cpu_percent,
    memUsed: stats.memory_used_mb,
    memLimit: stats.memory_limit_mb,
    netRx: stats.net_rx_bps,
    netTx: stats.net_tx_bps,
    diskR: stats.disk_read_bps,
    diskW: stats.disk_write_bps,
  };
}

// node metrics เข้ารูป StatPoint เดียวกับ instance — key ด้วย node.id (UUID ไม่ชนกับ server.id)
function toNodeStatPoint(node: Node): StatPoint {
  const ts = node.last_heartbeat_at
    ? new Date(node.last_heartbeat_at).getTime()
    : Date.now();
  return {
    t: ts || Date.now(),
    cpu: node.cpu_percent,
    memUsed: node.memory_used_mb,
    memLimit: node.memory_total_mb,
    netRx: node.net_rx_bps,
    netTx: node.net_tx_bps,
    diskUsed: node.disk_used_mb,
    diskTotal: node.disk_total_mb,
  };
}

function applyEvent(
  qc: QueryClient,
  stats: StatsHistoryApi,
  msg: EventsServerMessage,
) {
  switch (msg.type) {
    case "server_stats": {
      qc.setQueryData<ServersCache>(["servers"], (old) =>
        old
          ? {
              ...old,
              servers: old.servers.map((s) =>
                s.id === msg.server_id ? { ...s, stats: msg.stats } : s,
              ),
            }
          : old,
      );
      qc.setQueryData<ServerDetailCache>(["servers", msg.server_id], (old) =>
        old ? { ...old, server: { ...old.server, stats: msg.stats } } : old,
      );
      // ป้อน history ตรงจาก WS เพื่อให้กราฟ/popover อัปเดตแม้ไม่ได้เปิดหน้านั้นค้างไว้
      if (msg.stats) stats.push(msg.server_id, toStatPoint(msg.stats));
      else stats.reset(msg.server_id);
      break;
    }
    case "server_status": {
      qc.setQueryData<ServersCache>(["servers"], (old) =>
        old
          ? {
              ...old,
              servers: old.servers.map((s) =>
                s.id === msg.server_id ? { ...s, status: msg.status } : s,
              ),
            }
          : old,
      );
      qc.setQueryData<ServerDetailCache>(["servers", msg.server_id], (old) =>
        old ? { ...old, server: { ...old.server, status: msg.status } } : old,
      );
      // เมื่อ server ไม่ได้รันแล้ว เคลียร์ history เพื่อให้กราฟเริ่มใหม่รอบหน้า
      if (msg.status !== "running") stats.reset(msg.server_id);
      break;
    }
    case "node_stats": {
      qc.setQueryData<NodesCache>(["nodes"], (old) =>
        old
          ? {
              ...old,
              nodes: old.nodes.map((n) =>
                n.id === msg.node.id ? msg.node : n,
              ),
            }
          : old,
      );
      // ป้อน history ให้กราฟ per-node เหมือน server_stats — online เท่านั้นถึงมีค่าจริง
      if (msg.node.status === "online") stats.push(msg.node.id, toNodeStatPoint(msg.node));
      else stats.reset(msg.node.id);
      break;
    }
    case "server_jobs": {
      qc.invalidateQueries({ queryKey: ["servers", msg.server_id, "jobs"] });
      break;
    }
  }
}

// เปิด WS /ws/events หนึ่งเส้นตลอด session ของ panel แล้ว patch react-query cache ตาม event
// โครง reconnect (exponential backoff + StrictMode-safe identity) มิเรอร์จาก use-server-console
export function useEvents() {
  const queryClient = useQueryClient();
  const pushStats = useStatsHistoryStore((s) => s.push);
  const resetStats = useStatsHistoryStore((s) => s.reset);

  // เก็บ handler ใน ref เพื่อไม่ต้อง reconnect เมื่อ callback เปลี่ยน identity
  const applyRef = useRef<(msg: EventsServerMessage) => void>(() => {});
  applyRef.current = (msg) =>
    applyEvent(queryClient, { push: pushStats, reset: resetStats }, msg);

  const resyncRef = useRef<() => void>(() => {});
  resyncRef.current = () => {
    // reconnect แล้ว resync ข้อมูลที่อาจพลาดตอนหลุด — detail/jobs จะ refetch ตามเมื่อถูกดู
    queryClient.invalidateQueries({ queryKey: ["servers"] });
    queryClient.invalidateQueries({ queryKey: ["nodes"] });
  };

  useEffect(() => {
    let disposed = false;
    let attempt = 0;
    let timer: number | undefined;
    let socket: WebSocket | null = null;
    const wsRef = { current: null as WebSocket | null };

    const connect = () => {
      if (disposed) return;
      let ws: WebSocket;
      try {
        ws = new WebSocket(eventsWsUrl());
      } catch {
        scheduleReconnect();
        return;
      }
      socket = ws;
      wsRef.current = ws;

      ws.onopen = () => {
        if (wsRef.current !== ws) return;
        attempt = 0;
        resyncRef.current();
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
        const parsed = eventsServerMessageSchema.safeParse(raw);
        if (parsed.success) applyRef.current(parsed.data);
      };
      ws.onclose = () => {
        if (wsRef.current !== ws) return;
        wsRef.current = null;
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
  }, []);
}
