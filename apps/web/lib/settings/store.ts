import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";

export type ServerView = "grid" | "list";

// UI preference ฝั่ง client ล้วน — เก็บใน localStorage ได้ (ไม่ใช่ auth token)
// theme/lang อยู่ใน cookie เพราะต้องอ่านฝั่ง server กัน FOUC; ตัวนี้ client-only เลยใช้ localStorage
interface SettingsState {
  serverView: ServerView;
  setServerView: (view: ServerView) => void;
  // sidebar โหมดย่อ (collapsed rail + auto-drawer เมื่อ hover) vs เปิดค้าง (pinned)
  sidebarCollapsed: boolean;
  setSidebarCollapsed: (collapsed: boolean) => void;
  toggleSidebar: () => void;
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set) => ({
      serverView: "grid",
      setServerView: (serverView) => set({ serverView }),
      sidebarCollapsed: false,
      setSidebarCollapsed: (sidebarCollapsed) => set({ sidebarCollapsed }),
      toggleSidebar: () =>
        set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
    }),
    {
      name: "mc_settings",
      storage: createJSONStorage(() => localStorage),
    },
  ),
);
