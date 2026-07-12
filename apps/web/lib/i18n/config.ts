// pure config ของ i18n ที่ "ไม่มี use client" — server component (root layout) เรียกได้
// ส่วน Provider/hook อยู่ใน index.tsx (client)
export type Locale = "en" | "th";

export const LOCALES: Locale[] = ["en", "th"];
export const DEFAULT_LOCALE: Locale = "en";
export const LANG_COOKIE = "mc_lang";

export function normalizeLocale(value: string | undefined | null): Locale {
  return value === "th" ? "th" : "en";
}
