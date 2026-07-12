import { create } from "zustand";

// global UI state สำหรับ modal ที่เปิดได้จากหลายที่ (sidebar, user menu)
// ephemeral ล้วน — ไม่ persist (ไม่ใช่ preference)
// หมายเหตุ: new/import server ย้ายเป็นหน้าเต็ม (/servers/new) แล้ว ไม่ใช้ store นี้
interface UiState {
  changePasswordOpen: boolean;
  openChangePassword: () => void;
  setChangePasswordOpen: (open: boolean) => void;
}

export const useUiStore = create<UiState>()((set) => ({
  changePasswordOpen: false,
  openChangePassword: () => set({ changePasswordOpen: true }),
  setChangePasswordOpen: (changePasswordOpen) => set({ changePasswordOpen }),
}));
