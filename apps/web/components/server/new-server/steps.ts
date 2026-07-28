import type { TranslationKey } from "@/lib/i18n";
import type { GameProfile, WizardStepKey } from "@/lib/games";

// catalog ของ step ที่ wizard รู้จัก — **ลำดับจริงเป็นของแต่ละเกม** (GameProfile.wizardSteps)
// ที่นี่บอกแค่ว่าแต่ละ step ชื่ออะไร เพิ่ม step ใหม่ = เพิ่มไฟล์ step-*.tsx + แถวที่นี่
// แล้วใส่ key ลงใน wizardSteps ของเกมที่ต้องใช้
export const STEP_TITLE_KEYS: Record<WizardStepKey, TranslationKey> = {
  general: "wizard.tabGeneral",
  properties: "wizard.tabProperties",
  access: "wizard.tabAccess",
  players: "wizard.tabPlayers",
};

export interface WizardStep {
  key: WizardStepKey;
  titleKey: TranslationKey;
}

// step ของเกมหนึ่ง ตามลำดับที่ profile กำหนด — step สุดท้ายคือตัวสั่งสร้างจริง
export function wizardSteps(profile: GameProfile): WizardStep[] {
  return profile.wizardSteps.map((key) => ({
    key,
    titleKey: STEP_TITLE_KEYS[key],
  }));
}
