"use client";

import * as React from "react";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useT, useLocale, type Locale } from "@/lib/i18n";
import { useTheme, type Theme } from "@/lib/settings/theme";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";

// ค่าตั้งส่วนตัวของ user (ธีม/ภาษา) — คนละหน้ากับ /settings ที่เป็น server settings
export default function PreferencesPage() {
  const t = useT();
  useSetBreadcrumbs(
    React.useMemo(() => [{ label: t("preferences.title") }], [t]),
  );
  const { theme, setTheme } = useTheme();
  const [locale, setLocale] = useLocale();

  return (
    <div className="grid gap-6">
      <div className="grid gap-1">
        <h1 className="text-xl font-semibold">{t("preferences.title")}</h1>
        <p className="text-muted-foreground text-sm">
          {t("preferences.subtitle")}
        </p>
      </div>

      <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>{t("settings.appearance")}</CardTitle>
            <CardDescription>{t("settings.appearanceDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2">
            <Label htmlFor="theme-select">{t("settings.theme")}</Label>
            <Select
              value={theme}
              onValueChange={(v) => setTheme(v as Theme)}
            >
              <SelectTrigger id="theme-select" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="dark">{t("settings.themeDark")}</SelectItem>
                <SelectItem value="light">{t("settings.themeLight")}</SelectItem>
                <SelectItem value="system">{t("settings.themeSystem")}</SelectItem>
              </SelectContent>
            </Select>
            <p className="text-muted-foreground text-xs">
              {t("settings.themeHint")}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t("settings.language")}</CardTitle>
            <CardDescription>{t("settings.languageDesc")}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2">
            <Label htmlFor="lang-select">{t("settings.language")}</Label>
            <Select
              value={locale}
              onValueChange={(v) => setLocale(v as Locale)}
            >
              <SelectTrigger id="lang-select" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="en">{t("settings.langEnglish")}</SelectItem>
                <SelectItem value="th">{t("settings.langThai")}</SelectItem>
              </SelectContent>
            </Select>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
