// GameProfile = ความรู้เฉพาะเกมทั้งหมดที่ **ฝั่ง web** ต้องใช้ รวมไว้ที่เดียวต่อเกม
//
// คู่ขนานกับ internal/games ของ control-plane/node-agent — ที่นี่เก็บเฉพาะสิ่งที่ backend
// บอกไม่ได้ผ่าน API: regex เช็คชื่อผู้เล่นฝั่ง client, การลงสีบรรทัด console
// และคำเรียกที่โผล่ในข้อความ i18n (ชื่อเกม/ชื่อ license/ชื่อ metric)
//
// **ห้ามเขียน switch ตามชื่อเกม/variant ใน component** — เรียกผ่าน useGameProfile() เสมอ

import type { TranslationKey } from "@/lib/i18n";

// step ของ wizard สร้าง server — เกมเป็นคนบอกว่าตัวเองมี step ไหนบ้าง (GameProfile.wizardSteps)
// เพราะบาง step ไม่มีความหมายกับบางเกม (เช่น allowlist ของเกมที่ panel เขียนรายชื่อไม่ได้)
// component ของแต่ละ step + ลำดับที่ยอมให้เรียง อยู่ที่ components/server/new-server/steps.ts
export type WizardStepKey = "general" | "properties" | "access" | "players";

export interface GameProfile {
  /** ต้องตรงกับ id ของ game definition ฝั่ง backend (servers.game) */
  id: string;
  /** ชื่อเกมที่โผล่ใน UI */
  label: string;
  /** คำอธิบายสั้น ๆ ที่โผล่บนการ์ดในหน้าเลือกเกม (i18n key — แปลได้ทั้ง en/th) */
  descriptionKey: TranslationKey;
  /** ภาพปกบนการ์ดเลือกเกม (path ใน public/) — วาดเอง ห้ามใช้ asset ของเกมจริง */
  coverSrc: string;

  /**
   * step ของ wizard สร้าง server ของเกมนี้ ตามลำดับที่ต้องเดิน — ต้องขึ้นต้นด้วย "general"
   * เสมอ (step บังคับกรอก) ที่เหลือข้ามได้ และ step สุดท้ายคือตัวสั่งสร้างจริง
   */
  wizardSteps: readonly WizardStepKey[];
  /** ชื่อข้อตกลงที่ user ต้องยอมรับก่อนสร้าง ("" = เกมนี้ไม่มี license ให้ยอมรับ) */
  licenseName: string;
  /** URL ของ license (ปล่อยว่าง = แสดงเป็น text ไม่ใช่ลิงก์) */
  licenseUrl: string;

  /** key ในไฟล์ config ที่เปิด/ปิด allowlist ("" = เกมนี้บังคับใช้เสมอ) */
  allowlistEnabledKey: string;

  /** ชื่อ metric ประจำเกมที่อ่านจาก console (stats.tick_rate) เช่น "TPS" */
  metricLabel: string;
  /** คำอธิบายตอน metric = 0 (variant นี้ไม่รองรับ ไม่ใช่ "ค่าเป็นศูนย์") */
  metricUnsupportedHint: string;

  /** เช็คชื่อผู้เล่นคร่าว ๆ ฝั่ง client — ตัวจริง verify ที่ backend กับ identity service */
  isValidPlayerName(name: string): boolean;

  /**
   * variant นี้ต้องยอมรับ license ไหม — ใช้เป็น fallback ตอน GET /api/meta/variants
   * ยังโหลดไม่เสร็จเท่านั้น (ค่าจริงมาจาก field requires_license ของ backend)
   */
  variantRequiresLicense(variant: string): boolean;

  /**
   * ลงสีเนื้อความของบรรทัด INFO ตามรูปแบบ log ของเกมนั้น (join/leave/chat/ready)
   * คืน null = ไม่มีอะไรพิเศษ ให้ใช้สีปกติ. ตัว timestamp/level parse เป็นของกลาง
   * อยู่ใน server-console.tsx
   */
  highlightConsoleMessage(message: string): string | null;
}
