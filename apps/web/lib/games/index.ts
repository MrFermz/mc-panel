// registry ของ game profile ฝั่ง web — คู่ขนานกับ games.NewRegistry(...) ของ backend
//
// component **ห้าม import lib/games/minecraft ตรง ๆ** — เรียก gameProfile(server.game)
// หรือ useGameProfile() เสมอ เพื่อให้เพิ่มเกมที่สองแล้วไม่ต้องไล่แก้ทีละหน้า

import { minecraft } from "@/lib/games/minecraft";
import { zomboid } from "@/lib/games/zomboid";
import type { GameProfile } from "@/lib/games/types";

export type { GameProfile, WizardStepKey } from "@/lib/games/types";

const PROFILES: Record<string, GameProfile> = {
  [minecraft.id]: minecraft,
  [zomboid.id]: zomboid,
};

// DEFAULT_GAME_ID = เกมที่ใช้เมื่อไม่รู้ว่า instance เป็นเกมอะไร (ต้องตรงกับ games.DefaultID
// ของ control-plane) — เป็น **fallback เท่านั้น** ไม่ใช่ "เกมที่ wizard สร้าง":
// ฟอร์มสร้าง server ดึงรายการจริงจาก GET /api/meta/games แล้วให้ user เลือก
export const DEFAULT_GAME_ID = minecraft.id;

// gameProfile คืน profile ของเกมนั้น — id ที่ไม่รู้จัก (backend ใหม่กว่า web) ตกไปใช้ default
// แทนที่จะพัง เพราะ UI ส่วนใหญ่ยังใช้งานได้โดยไม่ต้องรู้จักเกม
export function gameProfile(gameId?: string | null): GameProfile {
  return (gameId ? PROFILES[gameId] : undefined) ?? minecraft;
}

// knownGameProfile คืน undefined เมื่อ web ยังไม่รู้จักเกมนั้น — ใช้ที่ซึ่ง fallback เงียบ ๆ
// เป็นข้อมูลผิด (หน้าเลือกเกมจะโชว์ปก/คำอธิบายของ Minecraft ให้เกมอื่น) และที่ซึ่งต้องรู้ว่า
// เดินฟอร์มของเกมนี้ได้จริงไหม
export function knownGameProfile(gameId: string): GameProfile | undefined {
  return PROFILES[gameId];
}
