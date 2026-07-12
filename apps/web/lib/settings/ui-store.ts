import { create } from "zustand";

// global UI state สำหรับ modal ที่เปิดได้จากหลายที่ (sidebar, dashboard, user menu)
// ephemeral ล้วน — ไม่ persist (ไม่ใช่ preference)
interface UiState {
  newServerOpen: boolean;
  importServerOpen: boolean;
  changePasswordOpen: boolean;
  openNewServer: () => void;
  setNewServerOpen: (open: boolean) => void;
  openImportServer: () => void;
  setImportServerOpen: (open: boolean) => void;
  openChangePassword: () => void;
  setChangePasswordOpen: (open: boolean) => void;
}

export const useUiStore = create<UiState>()((set) => ({
  newServerOpen: false,
  importServerOpen: false,
  changePasswordOpen: false,
  openNewServer: () => set({ newServerOpen: true }),
  setNewServerOpen: (newServerOpen) => set({ newServerOpen }),
  openImportServer: () => set({ importServerOpen: true }),
  setImportServerOpen: (importServerOpen) => set({ importServerOpen }),
  openChangePassword: () => set({ changePasswordOpen: true }),
  setChangePasswordOpen: (changePasswordOpen) => set({ changePasswordOpen }),
}));
