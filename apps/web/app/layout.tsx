import type { Metadata } from "next";
import { cookies } from "next/headers";
import { Providers } from "./providers";
import {
  DEFAULT_THEME,
  SYSTEM_THEME_SCRIPT,
  THEME_COOKIE,
  normalizeTheme,
} from "@/lib/settings/theme-shared";
import { LANG_COOKIE, normalizeLocale } from "@/lib/i18n/config";
import "./globals.css";

export const metadata: Metadata = {
  title: "mc-panel",
  description: "Minecraft server management panel",
};

export default async function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  // อ่าน preference จาก cookie ฝั่ง server เพื่อเซ็ต class/lang ตั้งแต่ SSR (กัน FOUC)
  const cookieStore = await cookies();
  const theme = normalizeTheme(cookieStore.get(THEME_COOKIE)?.value ?? DEFAULT_THEME);
  const locale = normalizeLocale(cookieStore.get(LANG_COOKIE)?.value);

  // dark = มี class ตั้งแต่ SSR, light = ไม่มี class, system = ให้ head script ตัดสินก่อน paint
  const htmlClass = theme === "dark" ? "dark" : undefined;

  return (
    <html lang={locale} className={htmlClass} suppressHydrationWarning>
      <head>
        {theme === "system" && (
          <script dangerouslySetInnerHTML={{ __html: SYSTEM_THEME_SCRIPT }} />
        )}
      </head>
      <body className="min-h-screen bg-background text-foreground antialiased">
        <Providers theme={theme} locale={locale}>
          {children}
        </Providers>
      </body>
    </html>
  );
}
