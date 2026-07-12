"use client";

import { Button } from "@/components/ui/button";

// ค่ายอดนิยม (MB) — free entry ยังทำได้ที่ช่อง input, chip พวกนี้เป็นทางลัดเฉย ๆ
const MEMORY_PRESETS = [1024, 2048, 4096, 8192, 16384] as const;

export function MemoryPresets({
  value,
  onChange,
  disabled = false,
}: {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const current = Number(value);
  return (
    <div className="flex flex-wrap gap-2">
      {MEMORY_PRESETS.map((mb) => (
        <Button
          key={mb}
          type="button"
          size="sm"
          disabled={disabled}
          variant={current === mb ? "default" : "outline"}
          onClick={() => onChange(String(mb))}
        >
          {mb / 1024} GB
        </Button>
      ))}
    </div>
  );
}
