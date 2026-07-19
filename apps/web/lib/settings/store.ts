import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";

export type ServerView = "grid" | "list";

// UI preference ฝั่ง client ล้วน — เก็บใน localStorage ได้ (ไม่ใช่ auth token)
// theme/lang อยู่ใน cookie เพราะต้องอ่านฝั่ง server กัน FOUC; ตัวนี้ client-only เลยใช้ localStorage
interface SettingsState {
  serverView: ServerView;
  setServerView: (view: ServerView) => void;
  // sidebar โหมดย่อ (ซ่อนออกนอกจอ + drawer เมื่อ hover ขอบซ้าย) vs เปิดค้าง (pinned)
  sidebarCollapsed: boolean;
  setSidebarCollapsed: (collapsed: boolean) => void;
  toggleSidebar: () => void;
  // หน้า detail: การ์ด live-resources เปิด/ปิด (จำค่าไว้ข้าม visit) — default เปิด
  detailResourcesOpen: boolean;
  setDetailResourcesOpen: (open: boolean) => void;
  // dashboard overview: server ที่เลือกดูภาพรวมอยู่ (null = ยังไม่เลือก → หน้าใช้ตัวแรก)
  dashboardServerId: string | null;
  setDashboardServerId: (id: string | null) => void;
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
      detailResourcesOpen: true,
      setDetailResourcesOpen: (detailResourcesOpen) =>
        set({ detailResourcesOpen }),
      dashboardServerId: null,
      setDashboardServerId: (dashboardServerId) => set({ dashboardServerId }),
    }),
    {
      name: "mc_settings",
      storage: createJSONStorage(() => localStorage),
    },
  ),
);
