// Project Zomboid game profile ฝั่ง web — ทุกอย่างที่ "เป็น PZ" ใน UI อยู่ที่ไฟล์นี้ที่เดียว
//
// เกมนี้ไม่มี allowlist ที่ panel เป็นเจ้าของ และไม่มี metric ประจำเกมที่อ่านจาก console ได้
// (ดู console.go ของ node-agent) — field ที่ไม่มีจึงเว้นว่างไว้ตามสัญญาของ GameProfile

import type { GameProfile } from "@/lib/games/types";

// ชื่อบัญชีของ PZ — เช็คคร่าว ๆ ฝั่ง client เท่านั้น ตัวจริงถูกเช็คซ้ำที่ backend
// (ชื่อถูกต่อเข้าไปในคำสั่ง console จึงห้ามมี whitespace)
const USERNAME_RE = /^[A-Za-z0-9_.-]{1,32}$/;

const SGR = {
  reset: "\x1b[0m",
  bold: "\x1b[1m",
  gray: "\x1b[90m",
  green: "\x1b[32m",
  brightGreen: "\x1b[92m",
} as const;

function highlightConsoleMessage(msg: string): string | null {
  // บรรทัดที่ PZ ประกาศว่าโหลด world เสร็จ พร้อมรับ connection
  if (/SERVER STARTED/.test(msg))
    return `${SGR.brightGreen}${SGR.bold}${msg}${SGR.reset}`;
  if (/ fully connected$/.test(msg)) return `${SGR.green}${msg}${SGR.reset}`;
  if (/ disconnected$/.test(msg)) return `${SGR.gray}${msg}${SGR.reset}`;
  return null;
}

export const zomboid: GameProfile = {
  id: "zomboid",
  label: "Project Zomboid",
  // dedicated server เป็น app ที่ Steam เปิดให้โหลดแบบ anonymous — ไม่มีอะไรให้กดยอมรับ
  licenseName: "",
  licenseUrl: "",
  // PZ เก็บรายชื่อผู้เล่นไว้ใน DB ของตัวเกม ไม่ใช่ไฟล์ที่ panel เขียนได้ — ไม่มี key ให้เปิด
  allowlistEnabledKey: "",
  metricLabel: "",
  metricUnsupportedHint: "",
  isValidPlayerName: (name) => USERNAME_RE.test(name),
  variantRequiresLicense: () => false,
  highlightConsoleMessage,
};
