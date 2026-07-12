"use client";

import * as React from "react";
import { CheckIcon, CopyIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";

export function CopyButton({ value }: { value: string }) {
  const t = useT();
  const [copied, setCopied] = React.useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      // clipboard ใช้ไม่ได้ใน context ที่ไม่ secure — ให้ user copy เองจากช่องข้อความ
    }
  };

  return (
    <Button variant="outline" size="icon" onClick={copy} aria-label={t("common.copy")}>
      {copied ? <CheckIcon className="text-green-500" /> : <CopyIcon />}
    </Button>
  );
}
