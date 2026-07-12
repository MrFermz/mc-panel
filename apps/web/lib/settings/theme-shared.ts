// pure helper/constant ของ theme ที่ "ไม่มี use client" — server component (root layout)
// เรียกได้ตรง ๆ ส่วน Provider/hook อยู่ใน theme.tsx (client)
export type Theme = "dark" | "light" | "system";
export type ResolvedTheme = "dark" | "light";

export const THEME_COOKIE = "mc_theme";
export const DEFAULT_THEME: Theme = "dark";

export function normalizeTheme(value: string | undefined | null): Theme {
  return value === "light" || value === "system" ? value : "dark";
}

// script ฝัง <head> ตอน theme=system: toggle .dark ตาม matchMedia ก่อน paint กัน FOUC
export const SYSTEM_THEME_SCRIPT =
  "(function(){try{var d=window.matchMedia('(prefers-color-scheme: dark)').matches;var c=document.documentElement.classList;if(d)c.add('dark');else c.remove('dark');}catch(e){}})();";
