"use client";

import * as React from "react";
import { toast } from "sonner";
import { Trash2Icon } from "lucide-react";
import { useT } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { gameProfile } from "@/lib/games";

// step 4 — allowlist (ข้ามได้) และเป็น step ที่มีปุ่มสร้างจริง
// server ยังไม่ถูกสร้าง จึงอ่านไฟล์ state ของเกมไม่ได้เลย เก็บได้แค่รายชื่อที่จะ
// allowlist แล้ว apply หลังสร้างเสร็จ (ตัว live อยู่ที่ server-players.tsx)
// กติกาชื่อผู้เล่นมาจาก game profile — ตัวจริงถูก verify กับ identity service ที่ backend
export function StepPlayers({
  value,
  onChange,
  game,
  allowlistEnabled,
  onEnableAllowlist,
}: {
  value: string[];
  onChange: (next: string[]) => void;
  game?: string;
  // = config draft key ของ allowlist (game profile เป็นคนบอก key) — ไม่เปิดก็เพิ่มชื่อได้
  // แต่เกมจะไม่บังคับใช้
  allowlistEnabled: boolean;
  onEnableAllowlist: () => void;
}) {
  const t = useT();
  const profile = gameProfile(game);
  const [username, setUsername] = React.useState("");

  const add = () => {
    const name = username.trim();
    if (!profile.isValidPlayerName(name)) {
      toast.error(t("players.errInvalid"));
      return;
    }
    if (value.some((n) => n.toLowerCase() === name.toLowerCase())) {
      toast.error(t("players.errExists"));
      return;
    }
    onChange([...value, name]);
    setUsername("");
  };

  return (
    <div className="grid gap-4">
      {!allowlistEnabled ? (
        <div className="border-destructive/40 bg-destructive/5 grid gap-2 rounded-md border p-3 text-sm sm:flex sm:items-center sm:justify-between">
          <div className="grid gap-1">
            <p className="font-medium">{t("players.allowlistOff")}</p>
            <p className="text-muted-foreground text-xs">
              {t("wizard.playersDraftHint")}
            </p>
          </div>
          <Button size="sm" onClick={onEnableAllowlist}>
            {t("players.enableAllowlist")}
          </Button>
        </div>
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("players.allowlistOn")}{" "}
          <span className="text-xs">{t("wizard.playersDraftHint")}</span>
        </p>
      )}

      <form
        className="flex flex-wrap gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          add();
        }}
      >
        <Input
          className="w-full sm:max-w-xs"
          maxLength={16}
          autoCapitalize="none"
          spellCheck={false}
          placeholder={t("players.addPlaceholder")}
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <Button type="submit" disabled={username.trim() === ""}>
          {t("players.add")}
        </Button>
      </form>

      {value.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t("players.empty")}</p>
      ) : (
        <ul className="grid gap-1">
          {value.map((name) => (
            <li
              key={name.toLowerCase()}
              className="flex items-center justify-between gap-2 rounded-md border px-3 py-2 text-sm"
            >
              <span className="truncate font-medium">{name}</span>
              <Button
                variant="ghost"
                size="icon"
                className="text-destructive"
                aria-label={`${t("common.remove")} ${name}`}
                onClick={() =>
                  onChange(value.filter((n) => n !== name))
                }
              >
                <Trash2Icon />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
