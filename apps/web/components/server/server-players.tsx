"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2Icon, SearchIcon } from "lucide-react";
import {
  addPlayer,
  listPlayers,
  playerAction,
  removePlayer,
  saveServerProperties,
  ApiError,
} from "@/lib/api";
import type { ServerPlayer } from "@/lib/types";
import { formatPlaytime } from "@/lib/format";
import { useT, type TranslationKey } from "@/lib/i18n";
import { PlayerHead } from "@/components/server/player-head";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
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

// map error code จาก players endpoint → ข้อความ toast ที่เป็นมิตร
function playerErrorMessage(
  err: unknown,
  t: (k: TranslationKey) => string,
): string {
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
    case "invalid_state":
      return t("players.errNotRunning");
    default:
      return err.message || t("players.errGeneric");
  }
}

type Filter = "all" | "online" | "whitelisted" | "op" | "banned";

// ROLE = สิทธิ์ในเกม ไม่ใช่สิทธิ์ในแผงควบคุม (panel มี owner/collaborator แยกที่หน้า access
// ซึ่งผูกกับ user ของ panel ไม่ใช่ MC account — จับคู่กันไม่ได้ จึงมีแค่ Op/Member)
function RoleBadge({ player }: { player: ServerPlayer }) {
  const t = useT();
  if (player.op) {
    return (
      <Badge
        variant="outline"
        className="border-transparent bg-emerald-500/15 text-emerald-700 dark:text-emerald-400"
      >
        {t("players.roleOp")}
      </Badge>
    );
  }
  return (
    <Badge
      variant="outline"
      className="border-transparent bg-muted text-muted-foreground"
    >
      {t("players.roleMember")}
    </Badge>
  );
}

function StatusCell({ player }: { player: ServerPlayer }) {
  const t = useT();
  const [dot, label] = player.banned
    ? ["bg-red-500", t("players.statusBanned")]
    : player.online
      ? ["bg-emerald-500", t("players.statusOnline")]
      : ["bg-muted-foreground/50", t("players.statusOffline")];
  return (
    <span className="flex items-center gap-2">
      <span className={cn("size-1.5 rounded-full", dot)} />
      <span className="text-muted-foreground">{label}</span>
    </span>
  );
}

export default function ServerPlayers({
  serverId,
  isRunning,
  onlineNames,
  canManage,
  canModerate,
}: {
  serverId: string;
  isRunning: boolean;
  // รายชื่อที่ออนไลน์ "สด" จาก stats ของ server (WS patch cache ["servers"] ให้แล้ว) —
  // ไม่ใช้ค่า online ที่ติดมากับ payload ของ players เพราะนั่น fetch ครั้งเดียวแล้วค้าง
  // (กฎ repo: ห้าม poll REST — รับ update ต่อจาก WS)
  onlineNames: string[];
  // canManage = players.manage (whitelist), canModerate = players.moderate (op/kick/ban)
  canManage: boolean;
  canModerate: boolean;
}) {
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

  // op/deop/kick/ban/pardon วิ่งผ่าน console ของ server — ผลจริงเกิดในเกมทันที
  // ไฟล์ ops/banned อัปเดตตาม MC จึง refetch เพื่อดึงสถานะใหม่
  const act = useMutation({
    mutationFn: (v: {
      player: ServerPlayer;
      action: "op" | "deop" | "kick" | "ban" | "pardon";
    }) => playerAction(serverId, v.action, v.player.username),
    onSuccess: (_data, v) => {
      toast.success(t(`players.done.${v.action}` as TranslationKey));
      // MC เขียนไฟล์หลังรันคำสั่งเสร็จ — หน่วงนิดให้ไฟล์ทันก่อนอ่านซ้ำ
      window.setTimeout(invalidatePlayers, 500);
    },
    onError: (err) => toast.error(playerErrorMessage(err, t)),
  });

  const acting = (uuid: string) =>
    act.isPending && act.variables?.player.uuid === uuid;

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

  const onlineSet = React.useMemo(
    () => new Set(onlineNames.map((n) => n.toLowerCase())),
    [onlineNames],
  );

  const all = React.useMemo(
    () =>
      (players.data?.players ?? []).map((p) => ({
        ...p,
        online: onlineSet.has(p.username.toLowerCase()),
      })),
    [players.data, onlineSet],
  );

  const counts = React.useMemo(
    () => ({
      all: all.length,
      online: all.filter((p) => p.online).length,
      whitelisted: all.filter((p) => p.whitelisted).length,
      op: all.filter((p) => p.op).length,
      banned: all.filter((p) => p.banned).length,
    }),
    [all],
  );

  const visible = React.useMemo(() => {
    const q = search.trim().toLowerCase();
    return all.filter((p) => {
      if (q && !p.username.toLowerCase().includes(q)) return false;
      if (filter === "online") return p.online;
      if (filter === "whitelisted") return p.whitelisted;
      if (filter === "op") return p.op;
      if (filter === "banned") return p.banned;
      return true;
    });
  }, [all, search, filter]);

  const toggling = (uuid: string) =>
    toggle.isPending && toggle.variables?.uuid === uuid;

  const chips: { key: Filter; labelKey: TranslationKey; count: number }[] = [
    { key: "all", labelKey: "players.filterAll", count: counts.all },
    { key: "online", labelKey: "players.filterOnline", count: counts.online },
    {
      key: "whitelisted",
      labelKey: "players.filterWhitelisted",
      count: counts.whitelisted,
    },
    { key: "op", labelKey: "players.filterOp", count: counts.op },
    { key: "banned", labelKey: "players.filterBanned", count: counts.banned },
  ];

  // ปุ่ม action ทุกตัวสั่งผ่าน console — server ต้องรันอยู่ถึงจะกดได้
  const actionsDisabled = !isRunning;

  const rowActions = (player: ServerPlayer) =>
    !canModerate ? null : (
    <div className="flex flex-wrap justify-end gap-1.5">
      <Button
        size="sm"
        variant="secondary"
        disabled={actionsDisabled || acting(player.uuid)}
        onClick={() =>
          act.mutate({ player, action: player.op ? "deop" : "op" })
        }
      >
        {player.op ? t("players.removeOp") : t("players.makeOp")}
      </Button>
      <Button
        size="sm"
        variant="outline"
        className="border-transparent bg-amber-500/15 text-amber-700 hover:bg-amber-500/25 dark:text-amber-400"
        disabled={actionsDisabled || acting(player.uuid) || !player.online}
        onClick={() => act.mutate({ player, action: "kick" })}
      >
        {t("players.kick")}
      </Button>
      <Button
        size="sm"
        variant="outline"
        className="border-transparent bg-red-500/15 text-red-700 hover:bg-red-500/25 dark:text-red-400"
        disabled={actionsDisabled || acting(player.uuid)}
        onClick={() =>
          act.mutate({ player, action: player.banned ? "pardon" : "ban" })
        }
      >
        {player.banned ? t("players.unban") : t("players.ban")}
      </Button>
    </div>
  );

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
          {canManage && (
            <Button
              size="sm"
              disabled={enableWhitelist.isPending}
              onClick={() => enableWhitelist.mutate()}
            >
              {enableWhitelist.isPending
                ? t("common.saving")
                : t("players.enableWhitelist")}
            </Button>
          )}
        </div>
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("players.whitelistOn")}{" "}
          <span className="text-xs">{t("players.whitelistApplyHint")}</span>
        </p>
      )}

      {canManage && (
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
      )}

      {players.isPending ? (
        <Skeleton className="h-32 w-full" />
      ) : players.isError ? (
        <p className="text-destructive text-sm">{t("players.failedLoad")}</p>
      ) : all.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t("players.empty")}</p>
      ) : (
        <>
          {/* แถวบน: ค้นหาซ้าย · จำนวนขวา */}
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="relative w-full sm:w-80">
              <SearchIcon className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2" />
              <Input
                className="w-full pl-8"
                placeholder={t("players.searchPlaceholder")}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            <span className="text-muted-foreground text-sm">
              {t("players.countSummary", { count: visible.length })}
            </span>
          </div>

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
          </div>

          {visible.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("players.noneMatch")}
            </p>
          ) : (
            <>
              {/* มือถือ: การ์ดต่อผู้เล่น (table ล้นที่จอแคบ) */}
              <div className="grid gap-2 lg:hidden">
                {visible.map((player) => (
                  <div
                    key={player.uuid || player.username}
                    className="grid gap-3 rounded-md border p-3"
                  >
                    <div className="flex items-center gap-3">
                      <PlayerHead
                        name={player.username}
                        serverId={serverId}
                        uuid={player.uuid}
                      />
                      <div className="min-w-0 flex-1">
                        <p className="truncate font-medium">
                          {player.username}
                        </p>
                        <StatusCell player={player} />
                      </div>
                      <RoleBadge player={player} />
                    </div>
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-muted-foreground text-xs">
                        {t("players.playtime")}{" "}
                        {formatPlaytime(player.playtime_seconds)}
                      </span>
                      <div className="flex items-center gap-2">
                        <Switch
                          checked={player.whitelisted}
                          disabled={toggling(player.uuid) || !canManage}
                          onCheckedChange={() => toggle.mutate(player)}
                          aria-label={t("players.toggleWhitelist")}
                        />
                        <span className="text-muted-foreground text-[10px]">
                          {t("players.badgeWhitelisted")}
                        </span>
                      </div>
                    </div>
                    {rowActions(player)}
                  </div>
                ))}
              </div>

              {/* จอใหญ่: ตาราง */}
              <Card className="hidden py-0 lg:block">
                <CardContent className="overflow-x-auto px-0">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t("players.colPlayer")}</TableHead>
                        <TableHead>{t("players.colRole")}</TableHead>
                        <TableHead>{t("players.colStatus")}</TableHead>
                        <TableHead>{t("players.colPlaytime")}</TableHead>
                        <TableHead>{t("players.whitelist")}</TableHead>
                        <TableHead className="text-right">
                          {t("players.colActions")}
                        </TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {visible.map((player) => (
                        <TableRow key={player.uuid || player.username}>
                          <TableCell>
                            <div className="flex items-center gap-3">
                              <PlayerHead
                                name={player.username}
                                serverId={serverId}
                                uuid={player.uuid}
                              />
                              <div className="min-w-0">
                                <p className="truncate font-medium">
                                  {player.username}
                                </p>
                                {player.banned && (
                                  <p className="text-xs font-medium text-red-600 dark:text-red-400">
                                    {t("players.statusBanned")}
                                  </p>
                                )}
                              </div>
                            </div>
                          </TableCell>
                          <TableCell>
                            <RoleBadge player={player} />
                          </TableCell>
                          <TableCell>
                            <StatusCell player={player} />
                          </TableCell>
                          <TableCell className="text-muted-foreground">
                            {formatPlaytime(player.playtime_seconds)}
                          </TableCell>
                          <TableCell>
                            <Switch
                              checked={player.whitelisted}
                              disabled={toggling(player.uuid) || !canManage}
                              onCheckedChange={() => toggle.mutate(player)}
                              aria-label={t("players.toggleWhitelist")}
                            />
                          </TableCell>
                          <TableCell>{rowActions(player)}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>

              {!isRunning && (
                <p className="text-muted-foreground text-xs">
                  {t("players.actionsNeedRunning")}
                </p>
              )}
            </>
          )}
        </>
      )}
    </div>
  );
}
