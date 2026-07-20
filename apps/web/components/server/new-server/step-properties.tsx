"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { getPropertiesCatalog } from "@/lib/api";
import { useT } from "@/lib/i18n";
import { PropertiesFields } from "@/components/server/server-settings";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

// draft ของ server.properties เก็บเฉพาะ key ที่ต่างจาก default — ตอน apply จึงส่งไปเท่าที่
// เปลี่ยนจริง (ตอนนั้นไฟล์ยังไม่มี MC จะเขียนที่เหลือเองตอน start แรก)
export function useCatalogDefaults() {
  const query = useQuery({
    queryKey: ["meta", "properties"],
    queryFn: () => getPropertiesCatalog(),
    staleTime: 300_000,
  });
  const defaults = React.useMemo(() => query.data?.values ?? {}, [query.data]);
  return { query, defaults };
}

// เทียบ draft กับ default แล้วเหลือเฉพาะที่ต่าง
export function changedFrom(
  defaults: Record<string, string>,
  draft: Record<string, string>,
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(draft)) {
    if (defaults[k] !== v) out[k] = v;
  }
  return out;
}

// step 2 — server.properties (ข้ามได้)
export function StepProperties({
  draft,
  onChange,
}: {
  draft: Record<string, string>;
  onChange: (key: string, value: string) => void;
}) {
  const t = useT();
  const { query, defaults } = useCatalogDefaults();

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("sset.propsTitle")}</CardTitle>
        <CardDescription>{t("wizard.propsDraftHint")}</CardDescription>
      </CardHeader>
      <CardContent>
        {query.isPending ? (
          <Skeleton className="h-40 w-full" />
        ) : query.isError ? (
          <p className="text-destructive text-sm">{t("sset.failedSave")}</p>
        ) : (
          <PropertiesFields
            fields={query.data.fields}
            values={{ ...defaults, ...draft }}
            onChange={onChange}
          />
        )}
      </CardContent>
    </Card>
  );
}
