"use client";

import * as React from "react";
import { CheckIcon } from "lucide-react";
import { useT } from "@/lib/i18n";
import type { WizardStep } from "@/components/server/new-server/steps";
import { cn } from "@/lib/utils";

// ตัวบอกลำดับ step — horizontal บน desktop (โชว์ชื่อ), compact บน mobile (โชว์เลข)
// รายการ step มาจากเกมที่เลือกไว้ ไม่ใช่ค่าคงที่ของ wizard
export function StepIndicator({
  steps,
  current,
  onSelect,
}: {
  steps: WizardStep[];
  current: number;
  onSelect: (step: number) => void;
}) {
  const t = useT();
  const last = steps.length - 1;
  return (
    <ol className="flex items-center">
      {steps.map((s, i) => {
        const done = i < current;
        const active = i === current;
        return (
          <React.Fragment key={s.key}>
            <li className="flex shrink-0 items-center gap-2">
              <button
                type="button"
                // ถอยกลับไปแก้ step ก่อนหน้าได้เสมอ — ยังไม่มีอะไรถูกสร้าง
                disabled={!done}
                onClick={() => onSelect(i)}
                className="flex items-center gap-2 disabled:cursor-default"
              >
                <span
                  className={cn(
                    "flex size-7 shrink-0 items-center justify-center rounded-full border text-xs font-medium transition-colors",
                    done && "border-primary bg-primary text-primary-foreground",
                    active && "border-primary text-primary",
                    !done && !active && "text-muted-foreground",
                  )}
                >
                  {done ? <CheckIcon className="size-4" /> : i + 1}
                </span>
                <span
                  className={cn(
                    "hidden text-sm font-medium sm:inline",
                    active ? "text-foreground" : "text-muted-foreground",
                  )}
                >
                  {t(s.titleKey)}
                </span>
              </button>
            </li>
            {i < last && (
              <span
                className={cn(
                  "mx-2 h-px flex-1 sm:mx-3",
                  done ? "bg-primary" : "bg-border",
                )}
              />
            )}
          </React.Fragment>
        );
      })}
    </ol>
  );
}
