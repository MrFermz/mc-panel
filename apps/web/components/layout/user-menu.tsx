"use client";

import Link from "next/link";
import {
  ChevronDownIcon,
  KeyRoundIcon,
  LogOutIcon,
  SettingsIcon,
  UserIcon,
} from "lucide-react";
import { apiSendVoid } from "@/lib/api";
import type { User } from "@/lib/types";
import { useT } from "@/lib/i18n";
import { useUiStore } from "@/lib/settings/ui-store";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export function UserMenu({ user }: { user: User }) {
  const t = useT();
  const openChangePassword = useUiStore((s) => s.openChangePassword);
  const logout = async () => {
    try {
      await apiSendVoid("POST", "/api/auth/logout");
    } finally {
      window.location.assign("/login");
    }
  };

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="gap-2">
          <UserIcon className="size-4" />
          <span className="max-w-40 truncate">
            {user.display_name || user.email}
          </span>
          <ChevronDownIcon className="size-3.5 opacity-60" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel>
          <div className="grid gap-0.5">
            <span>{user.display_name || user.email}</span>
            <span className="text-muted-foreground text-xs font-normal">
              {user.email}
            </span>
          </div>
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link href="/settings">
            <SettingsIcon />
            {t("userMenu.settings")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem onSelect={openChangePassword}>
          <KeyRoundIcon />
          {t("userMenu.changePassword")}
        </DropdownMenuItem>
        <DropdownMenuItem variant="destructive" onSelect={logout}>
          <LogOutIcon />
          {t("userMenu.logout")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
