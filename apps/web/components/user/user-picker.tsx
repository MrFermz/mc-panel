"use client";

import * as React from "react";
import { CheckIcon, ChevronsUpDownIcon, SearchIcon } from "lucide-react";
import type { DirectoryUser } from "@/lib/types";
import { useT } from "@/lib/i18n";
import { userIdent, userTitle } from "@/lib/user-display";
import { UserIdentity } from "@/components/user/user-identity";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";

// combobox เลือก user แบบค้นหาได้ — สร้างจาก Popover + Input ที่มีอยู่แล้ว
// (ไม่เพิ่ม dependency cmdk เพื่อ component เดียว ตามกฎ dependency ของ repo)
export function UserPicker({
  users,
  value,
  onSelect,
  disabled = false,
  placeholder,
  emptyLabel,
  id,
}: {
  users: DirectoryUser[];
  // "" = ยังไม่ได้เลือก
  value: string;
  onSelect: (userId: string) => void;
  disabled?: boolean;
  placeholder?: string;
  // ข้อความตอนไม่มี user ให้เลือกเลย (ต่างจาก "หาไม่เจอ" ตอนพิมพ์ค้นหา)
  emptyLabel?: string;
  id?: string;
}) {
  const t = useT();
  const [open, setOpen] = React.useState(false);
  const [search, setSearch] = React.useState("");
  // index ที่ไฮไลต์อยู่สำหรับเดินด้วยลูกศร
  const [active, setActive] = React.useState(0);
  const listRef = React.useRef<HTMLDivElement>(null);

  const selected = users.find((u) => u.id === value);

  const matches = React.useMemo(() => {
    const q = search.trim().toLowerCase();
    if (q === "") return users;
    return users.filter(
      (u) =>
        u.username.toLowerCase().includes(q) ||
        (u.display_name ?? "").toLowerCase().includes(q),
    );
  }, [users, search]);

  // เปิดใหม่ทุกครั้งเริ่มจากรายการบนสุดเสมอ ไม่ค้างจากรอบก่อน
  React.useEffect(() => {
    if (open) {
      setSearch("");
      setActive(0);
    }
  }, [open]);

  React.useEffect(() => {
    setActive(0);
  }, [search]);

  // เลื่อนรายการที่ไฮไลต์ให้อยู่ในสายตาเวลาเดินด้วยลูกศร
  React.useEffect(() => {
    listRef.current
      ?.querySelector(`[data-index="${active}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [active]);

  const pick = (userId: string) => {
    onSelect(userId);
    setOpen(false);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((i) => Math.min(i + 1, matches.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((i) => Math.max(i - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const target = matches[active];
      if (target) pick(target.id);
    }
  };

  const noUsers = users.length === 0;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled || noUsers}
          className="h-auto w-full justify-between py-2 font-normal"
        >
          {selected ? (
            <UserIdentity user={selected} size="sm" />
          ) : (
            <span className="text-muted-foreground">
              {noUsers
                ? (emptyLabel ?? t("access.noPickable"))
                : (placeholder ?? t("access.pickUserPlaceholder"))}
            </span>
          )}
          <ChevronsUpDownIcon className="opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-[var(--radix-popover-trigger-width)] p-0"
        onKeyDown={onKeyDown}
      >
        <div className="relative border-b">
          <SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
          <Input
            autoFocus
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("access.searchUser")}
            className="border-0 pl-8 shadow-none focus-visible:ring-0"
          />
        </div>
        <div ref={listRef} role="listbox" className="max-h-64 overflow-y-auto p-1">
          {matches.length === 0 ? (
            <p className="text-muted-foreground p-3 text-center text-sm">
              {t("access.noUserMatch")}
            </p>
          ) : (
            matches.map((u, i) => (
              <button
                key={u.id}
                type="button"
                role="option"
                aria-selected={u.id === value}
                data-index={i}
                onMouseEnter={() => setActive(i)}
                onClick={() => pick(u.id)}
                className={cn(
                  "flex w-full items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-left",
                  i === active && "bg-accent text-accent-foreground",
                )}
              >
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">
                    {userTitle(u)}
                  </span>
                  {userIdent(u) !== userTitle(u) && (
                    <span className="text-muted-foreground block truncate text-xs">
                      {userIdent(u)}
                    </span>
                  )}
                </span>
                {u.id === value && <CheckIcon className="size-4 shrink-0" />}
              </button>
            ))
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
