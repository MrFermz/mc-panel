"use client";

import * as React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import { ApiError } from "@/lib/api";
import { ThemeProvider, useTheme, type Theme } from "@/lib/settings/theme";
import { I18nProvider, type Locale } from "@/lib/i18n";

function AppToaster() {
  // sonner ไม่รู้จัก CSS variable — ป้อน theme ที่ resolve แล้วให้ตรงกับ light/dark ปัจจุบัน
  const { resolvedTheme } = useTheme();
  return <Toaster theme={resolvedTheme} richColors position="bottom-right" />;
}

export function Providers({
  children,
  theme,
  locale,
}: {
  children: React.ReactNode;
  theme: Theme;
  locale: Locale;
}) {
  const [queryClient] = React.useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            // 4xx เป็น error จริง (สิทธิ์/ข้อมูลผิด) retry ไปก็ไม่หาย
            retry: (failureCount, error) => {
              if (error instanceof ApiError && error.status < 500) return false;
              return failureCount < 2;
            },
            staleTime: 5_000,
          },
        },
      }),
  );

  return (
    <ThemeProvider initialTheme={theme}>
      <I18nProvider initialLocale={locale}>
        <QueryClientProvider client={queryClient}>
          {children}
          <AppToaster />
        </QueryClientProvider>
      </I18nProvider>
    </ThemeProvider>
  );
}
