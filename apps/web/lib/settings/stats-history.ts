import { create } from "zustand";

// จำนวนจุดสูงสุดต่อ server (rolling window) — ~40 จุด @ refetch 5s = ~3.3 นาที
const MAX_POINTS = 40;

export interface StatPoint {
  t: number; // epoch ms ของ stats.updated_at
  cpu: number; // percent
  memUsed: number; // MB
  memLimit: number; // MB
}

// history ของ resource stats ต่อ server เก็บใน memory เท่านั้น (ephemeral เหมือน console history)
interface StatsHistoryState {
  history: Record<string, StatPoint[]>;
  push: (serverId: string, point: StatPoint) => void;
  reset: (serverId: string) => void;
}

export const useStatsHistoryStore = create<StatsHistoryState>()((set) => ({
  history: {},
  push: (serverId, point) =>
    set((state) => {
      const prev = state.history[serverId] ?? [];
      // กัน append ซ้ำเมื่อ stats.updated_at เดิม (refetch แต่ค่ายังไม่ขยับ)
      if (prev.length > 0 && prev[prev.length - 1]?.t === point.t) return state;
      const next = [...prev, point].slice(-MAX_POINTS);
      return { history: { ...state.history, [serverId]: next } };
    }),
  reset: (serverId) =>
    set((state) => {
      if (!(serverId in state.history)) return state;
      const next = { ...state.history };
      delete next[serverId];
      return { history: next };
    }),
}));
