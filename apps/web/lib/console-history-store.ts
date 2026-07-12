import { create } from "zustand";

const MAX_HISTORY = 100;

// history ของคำสั่ง console ต่อ server เก็บใน memory เท่านั้น (ห้าม localStorage —
// นโยบายของ app นี้คือไม่เก็บอะไรฝั่ง browser storage เลย)
interface ConsoleHistoryState {
  history: Record<string, string[]>;
  push: (serverId: string, command: string) => void;
}

export const useConsoleHistoryStore = create<ConsoleHistoryState>()((set) => ({
  history: {},
  push: (serverId, command) =>
    set((state) => {
      const prev = state.history[serverId] ?? [];
      if (prev[prev.length - 1] === command) return state;
      const next = [...prev, command].slice(-MAX_HISTORY);
      return { history: { ...state.history, [serverId]: next } };
    }),
}));
