// registry ของ game profile ฝั่ง web — คู่ขนานกับ games.NewRegistry(...) ของ backend
//
// component **ห้าม import lib/games/minecraft ตรง ๆ** — เรียก gameProfile(server.game)
// หรือ useGameProfile() เสมอ เพื่อให้เพิ่มเกมที่สองแล้วไม่ต้องไล่แก้ทีละหน้า

import { minecraft } from "@/lib/games/minecraft";
import type { GameProfile } from "@/lib/games/types";

export type { DetectedInstance, GameProfile } from "@/lib/games/types";

const PROFILES: Record<string, GameProfile> = {
  [minecraft.id]: minecraft,
};

// DEFAULT_GAME_ID = เกมที่ใช้เมื่อไม่รู้ว่า instance เป็นเกมอะไร (ต้องตรงกับ games.DefaultID
// ของ control-plane). เฟสนี้มีเกมเดียว ฟอร์มสร้าง server จึงยังไม่มี UI ให้เลือกเกม —
// มีเกมที่สองเมื่อไรต้องกลายเป็น state ของ wizard + ดึงรายการจาก GET /api/meta/games
export const DEFAULT_GAME_ID = minecraft.id;

// gameProfile คืน profile ของเกมนั้น — id ที่ไม่รู้จัก (backend ใหม่กว่า web) ตกไปใช้ default
// แทนที่จะพัง เพราะ UI ส่วนใหญ่ยังใช้งานได้โดยไม่ต้องรู้จักเกม
export function gameProfile(gameId?: string | null): GameProfile {
  return (gameId ? PROFILES[gameId] : undefined) ?? minecraft;
}
