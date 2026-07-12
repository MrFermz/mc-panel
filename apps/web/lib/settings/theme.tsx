"use client";

import * as React from "react";
import { writeCookie } from "@/lib/settings/cookie";
import {
  DEFAULT_THEME,
  SYSTEM_THEME_SCRIPT,
  THEME_COOKIE,
  normalizeTheme,
  type ResolvedTheme,
  type Theme,
} from "@/lib/settings/theme-shared";

// re-export เพื่อให้ client component เดิมที่ import จาก theme.tsx ใช้ต่อได้
export { DEFAULT_THEME, SYSTEM_THEME_SCRIPT, THEME_COOKIE, normalizeTheme };
export type { ResolvedTheme, Theme };

function systemPrefersDark(): boolean {
  if (typeof window === "undefined" || !window.matchMedia) return true;
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function resolveTheme(theme: Theme): ResolvedTheme {
  if (theme === "system") return systemPrefersDark() ? "dark" : "light";
  return theme;
}

function applyThemeClass(resolved: ResolvedTheme): void {
  if (typeof document === "undefined") return;
  document.documentElement.classList.toggle("dark", resolved === "dark");
}

interface ThemeContextValue {
  theme: Theme;
  resolvedTheme: ResolvedTheme;
  setTheme: (theme: Theme) => void;
}

const ThemeContext = React.createContext<ThemeContextValue | null>(null);

export function ThemeProvider({
  initialTheme,
  children,
}: {
  initialTheme: Theme;
  children: React.ReactNode;
}) {
  const [theme, setThemeState] = React.useState<Theme>(initialTheme);
  // initial resolved ต้องตรงกับ SSR: system ถือเป็น dark (default ของ app) แล้วแก้ใน effect
  const [resolvedTheme, setResolvedTheme] = React.useState<ResolvedTheme>(
    initialTheme === "light" ? "light" : "dark",
  );

  // sync resolved จริงหลัง mount (สำคัญกับ system ที่ต้องอ่าน matchMedia)
  React.useEffect(() => {
    setResolvedTheme(resolveTheme(theme));
    // ครั้งเดียวตอน mount — theme ถูกจัดการโดย SSR class + head script แล้ว
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ตาม OS theme เมื่อเลือก system
  React.useEffect(() => {
    if (theme !== "system" || typeof window === "undefined") return;
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const onChange = () => {
      const next: ResolvedTheme = mq.matches ? "dark" : "light";
      applyThemeClass(next);
      setResolvedTheme(next);
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, [theme]);

  const setTheme = React.useCallback((next: Theme) => {
    writeCookie(THEME_COOKIE, next);
    setThemeState(next);
    const resolved = resolveTheme(next);
    applyThemeClass(resolved);
    setResolvedTheme(resolved);
  }, []);

  const value = React.useMemo(
    () => ({ theme, resolvedTheme, setTheme }),
    [theme, resolvedTheme, setTheme],
  );

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
}

export function useTheme(): ThemeContextValue {
  const ctx = React.useContext(ThemeContext);
  if (!ctx) throw new Error("useTheme must be used within a ThemeProvider");
  return ctx;
}
