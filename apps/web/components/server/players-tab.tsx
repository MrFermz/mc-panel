"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2Icon, SearchIcon, UserIcon } from "lucide-react";
import {
  addPlayer,
  listPlayers,
  removePlayer,
  saveServerProperties,
  ApiError,
} from "@/lib/api";
import type { ServerPlayer } from "@/lib/types";
import { useT, type TranslationKey } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";

// crafatar ต้องการ uuid แบบไม่มีขีด — โหลดหน้า skin จาก public renderer (ไม่ผ่าน next/image กัน remote-host config)
function PlayerAvatar({ uuid, username }: { uuid: string; username: string }) {
  const [failed, setFailed] = React.useState(false);
  const bare = uuid.replace(/-/g, "");
  if (failed || bare === "") {
    return (
      <div className="bg-muted text-muted-foreground flex size-8 items-center justify-center rounded">
        <UserIcon className="size-4" />
      </div>
    );
  }
  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={`https://crafatar.com/avatars/${bare}?size=40&overlay`}
      alt={username}
      loading="lazy"
      className="size-8 rounded"
      onError={() => setFailed(true)}
    />
  );
}

// map error code จาก POST/DELETE players → ข้อความ toast ที่เป็นมิตร
function playerErrorMessage(err: unknown, t: (k: TranslationKey) => string): string {
  if (!(err instanceof ApiError)) return t("players.errGeneric");
  switch (err.code) {
    case "player_not_found":
      return t("players.errNotFound");
    case "player_exists":
      return t("players.errExists");
    case "mojang_unavailable":
      return t("players.errMojang");
    case "invalid_username":
      return t("players.errInvalid");
    case "forbidden":
      return t("players.errForbidden");
    case "node_offline":
      return t("players.errNodeOffline");
    case "agent_timeout":
      return t("players.errAgentTimeout");
    default:
      return err.message || t("players.errGeneric");
  }
}

type Filter = "all" | "whitelisted" | "joined" | "op" | "banned";

// สีของแต่ละสถานะ — แยกชัดเจน อ่านง่ายทั้ง light/dark
function StatusBadges({ player }: { player: ServerPlayer }) {
  const t = useT();
  return (
    <div className="flex flex-wrap gap-1">
      {player.whitelisted && (
        <Badge
          variant="outline"
          className="border-transparent bg-emerald-500/15 text-emerald-700 dark:text-emerald-400"
        >
          {t("players.badgeWhitelisted")}
        </Badge>
      )}
      {player.seen && (
        <Badge
          variant="outline"
          className="border-transparent bg-sky-500/15 text-sky-700 dark:text-sky-400"
        >
          {t("players.badgeJoined")}
        </Badge>
      )}
      {player.op && (
        <Badge
          variant="outline"
          className="border-transparent bg-amber-500/15 text-amber-700 dark:text-amber-400"
        >
          {t("players.badgeOp")}
        </Badge>
      )}
      {player.banned && (
        <Badge
          variant="outline"
          className="border-transparent bg-red-500/15 text-red-700 dark:text-red-400"
        >
          {t("players.badgeBanned")}
        </Badge>
      )}
    </div>
  );
}

export default function PlayersTab({ serverId }: { serverId: string }) {
  const t = useT();
  const queryClient = useQueryClient();
  const [username, setUsername] = React.useState("");
  const [search, setSearch] = React.useState("");
  const [filter, setFilter] = React.useState<Filter>("all");

  const players = useQuery({
    queryKey: ["servers", serverId, "players"],
    queryFn: () => listPlayers(serverId),
  });

  const whitelistEnabled = players.data?.whitelist_enabled ?? false;

  const invalidatePlayers = () =>
    queryClient.invalidateQueries({
      queryKey: ["servers", serverId, "players"],
    });

  const add = useMutation({
    mutationFn: () => addPlayer(serverId, username.trim()),
    onSuccess: (data) => {
      toast.success(t("players.added", { name: data.player.username }));
      setUsername("");
      invalidatePlayers();
    },
    onError: (err) => toast.error(playerErrorMessage(err, t)),
  });

  // toggle whitelist ต่อ row: on → add by username, off → remove by uuid
  const toggle = useMutation({
    mutationFn: async (player: ServerPlayer) => {
      if (player.whitelisted) {
        await removePlayer(serverId, player.uuid);
      } else {
        await addPlayer(serverId, player.username);
      }
    },
    onSuccess: (_data, player) => {
      toast.success(
        player.whitelisted
          ? t("players.removed")
          : t("players.added", { name: player.username }),
      );
      invalidatePlayers();
    },
    onError: (err) => toast.error(playerErrorMessage(err, t)),
  });

  const enableWhitelist = useMutation({
    mutationFn: () => saveServerProperties(serverId, { "white-list": "true" }),
    onSuccess: () => {
      toast.success(t("players.whitelistEnabled"));
      invalidatePlayers();
      queryClient.invalidateQueries({
        queryKey: ["servers", serverId, "properties"],
      });
    },
    onError: (err) =>
      toast.error(
        err instanceof ApiError ? err.message : t("players.errGeneric"),
      ),
  });

  const canSubmit = username.trim().length > 0 && !add.isPending;

  const all = React.useMemo(() => players.data?.players ?? [], [players.data]);

  const counts = React.useMemo(
    () => ({
      all: all.length,
      whitelisted: all.filter((p) => p.whitelisted).length,
      joined: all.filter((p) => p.seen).length,
      op: all.filter((p) => p.op).length,
      banned: all.filter((p) => p.banned).length,
    }),
    [all],
  );

  const visible = React.useMemo(() => {
    const q = search.trim().toLowerCase();
    return all.filter((p) => {
      if (q && !p.username.toLowerCase().includes(q)) return false;
      if (filter === "whitelisted") return p.whitelisted;
      if (filter === "joined") return p.seen;
      if (filter === "op") return p.op;
      if (filter === "banned") return p.banned;
      return true;
    });
  }, [all, search, filter]);

  const toggling = (uuid: string) =>
    toggle.isPending && toggle.variables?.uuid === uuid;

  const chips: { key: Filter; labelKey: TranslationKey; count: number }[] = [
    { key: "all", labelKey: "players.filterAll", count: counts.all },
    {
      key: "whitelisted",
      labelKey: "players.filterWhitelisted",
      count: counts.whitelisted,
    },
    { key: "joined", labelKey: "players.filterJoined", count: counts.joined },
    { key: "op", labelKey: "players.filterOp", count: counts.op },
    { key: "banned", labelKey: "players.filterBanned", count: counts.banned },
  ];

  return (
    <div className="grid gap-4">
      {!whitelistEnabled ? (
        <div className="border-destructive/40 bg-destructive/5 grid gap-2 rounded-md border p-3 text-sm sm:flex sm:items-center sm:justify-between">
          <div className="grid gap-1">
            <p className="font-medium">{t("players.whitelistOff")}</p>
            <p className="text-muted-foreground text-xs">
              {t("players.whitelistApplyHint")}
            </p>
          </div>
          <Button
            size="sm"
            disabled={enableWhitelist.isPending}
            onClick={() => enableWhitelist.mutate()}
          >
            {enableWhitelist.isPending
              ? t("common.saving")
              : t("players.enableWhitelist")}
          </Button>
        </div>
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("players.whitelistOn")}{" "}
          <span className="text-xs">{t("players.whitelistApplyHint")}</span>
        </p>
      )}

      <form
        className="flex flex-wrap gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          if (canSubmit) add.mutate();
        }}
      >
        <Input
          className="w-full sm:max-w-xs"
          maxLength={16}
          placeholder={t("players.addPlaceholder")}
          value={username}
          onChange={(e) => setUsername(e.target.value)}
        />
        <Button type="submit" disabled={!canSubmit}>
          {add.isPending && <Loader2Icon className="size-4 animate-spin" />}
          {t("players.add")}
        </Button>
      </form>

      {players.isPending ? (
        <Skeleton className="h-32 w-full" />
      ) : players.isError ? (
        <p className="text-destructive text-sm">{t("players.failedLoad")}</p>
      ) : all.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t("players.empty")}</p>
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-2">
            {chips.map((c) => (
              <Button
                key={c.key}
                type="button"
                size="sm"
                variant={filter === c.key ? "default" : "outline"}
                onClick={() => setFilter(c.key)}
              >
                {t(c.labelKey)}
                <span
                  className={cn(
                    "ml-1.5 text-xs",
                    filter === c.key
                      ? "text-primary-foreground/80"
                      : "text-muted-foreground",
                  )}
                >
                  {c.count}
                </span>
              </Button>
            ))}
            <div className="relative w-full sm:ml-auto sm:w-auto">
              <SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
              <Input
                className="w-full pl-8 sm:w-56"
                placeholder={t("players.searchPlaceholder")}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
          </div>

          {visible.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("players.noneMatch")}
            </p>
          ) : (
            <>
              {/* มือถือ: การ์ดต่อผู้เล่น (table ล้นที่จอแคบ) */}
              <div className="grid gap-2 md:hidden">
                {visible.map((player) => (
                  <div
                    key={player.uuid || player.username}
                    className="flex items-center gap-3 rounded-md border p-3"
                  >
                    <PlayerAvatar uuid={player.uuid} username={player.username} />
                    <div className="min-w-0 flex-1">
                      <p className="truncate font-medium">{player.username}</p>
                      <StatusBadges player={player} />
                    </div>
                    <div className="flex flex-col items-center gap-1">
                      <Switch
                        checked={player.whitelisted}
                        disabled={toggling(player.uuid)}
                        onCheckedChange={() => toggle.mutate(player)}
                        aria-label={t("players.toggleWhitelist")}
                      />
                      <span className="text-muted-foreground text-[10px]">
                        {t("players.badgeWhitelisted")}
                      </span>
                    </div>
                  </div>
                ))}
              </div>

              {/* จอใหญ่: ตาราง */}
              <div className="hidden md:block">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-12" />
                      <TableHead>{t("players.username")}</TableHead>
                      <TableHead>{t("players.status")}</TableHead>
                      <TableHead className="text-right">
                        {t("players.whitelist")}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {visible.map((player) => (
                      <TableRow key={player.uuid || player.username}>
                        <TableCell>
                          <PlayerAvatar
                            uuid={player.uuid}
                            username={player.username}
                          />
                        </TableCell>
                        <TableCell className="font-medium">
                          {player.username}
                        </TableCell>
                        <TableCell>
                          <StatusBadges player={player} />
                        </TableCell>
                        <TableCell className="text-right">
                          <div className="flex justify-end">
                            <Switch
                              checked={player.whitelisted}
                              disabled={toggling(player.uuid)}
                              onCheckedChange={() => toggle.mutate(player)}
                              aria-label={t("players.toggleWhitelist")}
                            />
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </>
          )}
        </>
      )}
    </div>
  );
}
