"use client";

import * as React from "react";
import { en, type Dictionary, type TranslationKey } from "@/lib/i18n/en";
import { th } from "@/lib/i18n/th";
import { writeCookie } from "@/lib/settings/cookie";

export type { TranslationKey } from "@/lib/i18n/en";

import {
  DEFAULT_LOCALE,
  LANG_COOKIE,
  LOCALES,
  normalizeLocale,
  type Locale,
} from "@/lib/i18n/config";

// re-export เพื่อให้ client component เดิมที่ import จาก @/lib/i18n ใช้ต่อได้
export { DEFAULT_LOCALE, LANG_COOKIE, LOCALES, normalizeLocale };
export type { Locale };

const dictionaries: Record<Locale, Dictionary> = { en, th };

export type Vars = Record<string, string | number>;

// interpolation แบบง่าย: แทน {name} ด้วย vars.name
function interpolate(template: string, vars?: Vars): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (match, key: string) =>
    key in vars ? String(vars[key]) : match,
  );
}

export type TranslateFn = (key: TranslationKey, vars?: Vars) => string;

interface I18nContextValue {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: TranslateFn;
}

const I18nContext = React.createContext<I18nContextValue | null>(null);

export function I18nProvider({
  initialLocale,
  children,
}: {
  initialLocale: Locale;
  children: React.ReactNode;
}) {
  const [locale, setLocaleState] = React.useState<Locale>(initialLocale);

  const setLocale = React.useCallback((next: Locale) => {
    writeCookie(LANG_COOKIE, next);
    setLocaleState(next);
    // อัปเดต <html lang> ให้ตรง โดยไม่ต้อง reload
    if (typeof document !== "undefined") {
      document.documentElement.lang = next;
    }
  }, []);

  const t = React.useCallback<TranslateFn>(
    (key, vars) => {
      const dict = dictionaries[locale];
      return interpolate(dict[key] ?? en[key] ?? key, vars);
    },
    [locale],
  );

  const value = React.useMemo(
    () => ({ locale, setLocale, t }),
    [locale, setLocale, t],
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

function useI18n(): I18nContextValue {
  const ctx = React.useContext(I18nContext);
  if (!ctx) throw new Error("useI18n must be used within an I18nProvider");
  return ctx;
}

export function useT(): TranslateFn {
  return useI18n().t;
}

export function useLocale(): [Locale, (locale: Locale) => void] {
  const { locale, setLocale } = useI18n();
  return [locale, setLocale];
}
