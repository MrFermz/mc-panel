import type { TranslationKey } from "@/lib/i18n";

// ลำดับ step ของ wizard — index ตรงกับ state `step` ของหน้า
// step สุดท้ายเป็นตัวสั่งสร้างจริง: ก่อนหน้านั้นทุกอย่างเป็น draft ในหน้าเว็บล้วน
export const WIZARD_STEPS = [
  { key: "general", titleKey: "wizard.tabGeneral" },
  { key: "properties", titleKey: "wizard.tabProperties" },
  { key: "access", titleKey: "wizard.tabAccess" },
  { key: "players", titleKey: "wizard.tabPlayers" },
] as const satisfies ReadonlyArray<{ key: string; titleKey: TranslationKey }>;

export const LAST_STEP = WIZARD_STEPS.length - 1;

export type WizardMode = "new" | "import";
