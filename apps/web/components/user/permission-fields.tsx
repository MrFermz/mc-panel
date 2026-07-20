"use client";

import * as React from "react";
import { en } from "@/lib/i18n/en";
import { useT, type TranslateFn, type TranslationKey } from "@/lib/i18n";
import { ROLE_LABEL_KEYS, ROLE_PRESETS, matchPreset } from "@/lib/user-roles";
import type { Capability } from "@/lib/types";
import { CheckIcon, MinusIcon } from "lucide-react";
import { cn } from "@/lib/utils";

// capability key มาจาก catalog ของ backend (dynamic) — หา i18n key แบบ runtime
// แล้ว fallback เป็น label/description อังกฤษจาก API ถ้ายังไม่ได้แปล
// (เพิ่ม capability ใหม่ = เพิ่ม `permAction.*` / `permDesc.*` ทั้ง en/th ดู CLAUDE.md)
function lookup(key: string): TranslationKey | null {
  return key in en ? (key as TranslationKey) : null;
}

export function permLabel(t: TranslateFn, cap: Capability): string {
  const tk = lookup(`permAction.${cap.action}`);
  return tk ? t(tk) : cap.label;
}

export function permDescription(t: TranslateFn, cap: Capability): string {
  const tk = lookup(`permDesc.${cap.key}`);
  return tk ? t(tk) : cap.description;
}

export function groupLabel(t: TranslateFn, group: string): string {
  const tk = lookup(`permGroup.${group}`);
  return tk ? t(tk) : group;
}

export interface CapabilityGroup {
  group: string;
  items: Capability[];
}

// จัดกลุ่มตามลำดับที่ catalog ส่งมา (backend คุมลำดับให้ UI แล้ว)
export function groupCapabilities(catalog: Capability[]): CapabilityGroup[] {
  const groups: CapabilityGroup[] = [];
  for (const cap of catalog) {
    const last = groups.find((g) => g.group === cap.group);
    if (last) last.items.push(cap);
    else groups.push({ group: cap.group, items: [cap] });
  }
  return groups;
}

export function FieldGroupLabel({ children }: { children: React.ReactNode }) {
  return (
    <p className="text-muted-foreground mb-2 text-xs font-medium tracking-wide uppercase">
      {children}
    </p>
  );
}

export function RolePresetPicker({
  isAdmin,
  capabilities,
  disabled,
  onSelect,
}: {
  isAdmin: boolean;
  capabilities: string[];
  disabled?: boolean;
  onSelect: (next: { isAdmin: boolean; capabilities: string[] }) => void;
}) {
  const t = useT();
  const selected = matchPreset(isAdmin, capabilities);
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
      {ROLE_PRESETS.map((preset) => {
        const active = selected === preset.key;
        return (
          <button
            key={preset.key}
            type="button"
            disabled={disabled}
            onClick={() =>
              onSelect({
                isAdmin: preset.isAdmin,
                capabilities: preset.capabilities,
              })
            }
            className={cn(
              "h-10 rounded-md border text-sm font-medium transition-colors",
              active
                ? "border-primary bg-primary/15 text-primary"
                : "hover:bg-accent hover:text-accent-foreground",
              disabled && "cursor-not-allowed opacity-50",
            )}
          >
            {t(ROLE_LABEL_KEYS[preset.key])}
          </button>
        );
      })}
    </div>
  );
}

// รายการสิทธิ์แบบจัดกลุ่มตาม feature — 1 แถว = 1 action (CRUD ของฟีเจอร์นั้น)
// **read-only ล้วน**: สิทธิ์มาจาก role preset ที่เลือกเท่านั้น (ไม่มี role custom แล้ว)
// ที่นี่คือหน้าต่างส่องว่า preset นั้นให้อะไรบ้าง ไม่ใช่ที่แก้ทีละข้อ
export function PermissionGroups({
  catalog,
  isAdmin,
  capabilities,
}: {
  catalog: Capability[];
  isAdmin: boolean;
  capabilities: string[];
}) {
  const t = useT();
  const groups = React.useMemo(() => groupCapabilities(catalog), [catalog]);

  if (groups.length === 0) {
    return (
      <p className="text-muted-foreground py-3 text-sm">
        {t("users.noCapabilities")}
      </p>
    );
  }

  return (
    <div className="grid gap-4">
      {groups.map(({ group, items }) => {
        const keys = items.map((c) => c.key);
        const on = keys.filter(
          (k) => isAdmin || capabilities.includes(k),
        ).length;
        return (
          <div key={group} className="rounded-lg border">
            <div className="flex items-center justify-between gap-3 border-b px-4 py-2.5">
              <span className="text-sm font-semibold">
                {groupLabel(t, group)}
              </span>
              <span className="text-muted-foreground text-xs">
                {t("users.accessCount", { count: on, total: keys.length })}
              </span>
            </div>
            <div className="divide-y px-4">
              {items.map((cap) => {
                // admin ได้ทุก capability โดยปริยาย ไม่ต้องมีใน capabilities[]
                const granted = isAdmin || capabilities.includes(cap.key);
                return (
                  <div
                    key={cap.key}
                    className="flex items-center justify-between gap-4 py-3"
                  >
                    <div
                      className={cn("grid gap-0.5", !granted && "opacity-50")}
                    >
                      <span className="text-sm font-medium">
                        {permLabel(t, cap)}
                      </span>
                      <span className="text-muted-foreground text-xs">
                        {permDescription(t, cap)}
                      </span>
                    </div>
                    {granted ? (
                      <CheckIcon
                        className="size-4 shrink-0 text-emerald-500"
                        aria-label={t("users.permGranted")}
                      />
                    ) : (
                      <MinusIcon
                        className="text-muted-foreground/50 size-4 shrink-0"
                        aria-label={t("users.permDenied")}
                      />
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        );
      })}
    </div>
  );
}
