// Minecraft game profile ฝั่ง web — ทุกอย่างที่ "เป็น Minecraft" ใน UI อยู่ที่ไฟล์นี้ที่เดียว
// (กติกาชื่อผู้เล่น, การลงสี log, ชื่อ EULA/metric)
//
// เพิ่มเกมใหม่ = เพิ่มไฟล์แบบเดียวกันนี้แล้วลงทะเบียนใน lib/games/index.ts

import type { GameProfile } from "@/lib/games/types";

// ชื่อผู้เล่น Minecraft — เช็คคร่าว ๆ ฝั่ง client เท่านั้น ตัวจริงถูก verify กับ Mojang ที่ backend
const USERNAME_RE = /^[A-Za-z0-9_]{3,16}$/;

// velocity เป็น proxy ไม่รัน server jar ของ Mojang — ไม่มี EULA ให้ยอมรับ
const VARIANTS_WITHOUT_LICENSE = new Set(["velocity"]);

// ---------- การลงสีบรรทัด console ----------
// ตัว timestamp/level parse เป็นของกลาง (server-console.tsx) — ที่นี่คือรูปแบบข้อความของ MC

const SGR = {
  reset: "\x1b[0m",
  bold: "\x1b[1m",
  gray: "\x1b[90m",
  green: "\x1b[32m",
  brightGreen: "\x1b[92m",
  cyan: "\x1b[36m",
} as const;

function highlightConsoleMessage(msg: string): string | null {
  if (/ (joined|joined the game)$/.test(msg))
    return `${SGR.green}${msg}${SGR.reset}`;
  if (/ left the game$/.test(msg)) return `${SGR.gray}${msg}${SGR.reset}`;
  if (/^Done \(/.test(msg))
    return `${SGR.brightGreen}${SGR.bold}${msg}${SGR.reset}`;
  const chat = /^<([^>]+)>\s([\s\S]*)$/.exec(msg);
  if (chat) return `${SGR.cyan}<${chat[1]}>${SGR.reset} ${chat[2]}`;
  return null;
}

export const minecraft: GameProfile = {
  id: "minecraft",
  label: "Minecraft",
  licenseName: "Minecraft EULA",
  licenseUrl: "https://www.minecraft.net/en-us/eula",
  // ต้อง white-list=true ใน server.properties ถึงจะ enforce จริง
  allowlistEnabledKey: "white-list",
  metricLabel: "TPS",
  metricUnsupportedHint: "Only Paper/Spigot report TPS",
  isValidPlayerName: (name) => USERNAME_RE.test(name),
  variantRequiresLicense: (variant) => !VARIANTS_WITHOUT_LICENSE.has(variant),
  highlightConsoleMessage,
};
