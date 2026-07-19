"use client";

import Link from "next/link";
import {
  ChevronDownIcon,
  LogOutIcon,
  SettingsIcon,
  UserIcon,
} from "lucide-react";
import { apiSendVoid } from "@/lib/api";
import type { User } from "@/lib/types";
import { useT } from "@/lib/i18n";
import { userTitle } from "@/lib/user-display";
import { detectRole } from "@/lib/user-roles";
import { UserIdentity } from "@/components/user/user-identity";
import { UserAvatar } from "@/components/user/user-avatar";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

export function UserMenu({
  user,
  className,
  align = "end",
}: {
  user: User;
  className?: string;
  align?: "start" | "end";
}) {
  const t = useT();
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
        <Button variant="ghost" className={cn("gap-2", className)}>
          <UserAvatar
            seed={user.id}
            name={userTitle(user)}
            src={user.avatar_url}
            className="size-6 rounded-md text-[0.625rem]"
          />
          <span className="max-w-40 truncate">{userTitle(user)}</span>
          <ChevronDownIcon className="ml-auto size-3.5 shrink-0 opacity-60" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align={align} className="w-56">
        <DropdownMenuLabel className="font-normal">
          <UserIdentity user={user} panelRole={detectRole(user)} size="sm" />
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link href="/profile">
            <UserIcon />
            {t("userMenu.profile")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link href="/preferences">
            <SettingsIcon />
            {t("userMenu.preferences")}
          </Link>
        </DropdownMenuItem>
        <DropdownMenuItem variant="destructive" onSelect={logout}>
          <LogOutIcon />
          {t("userMenu.logout")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
