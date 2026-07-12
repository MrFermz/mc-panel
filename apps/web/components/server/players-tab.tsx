"use client";

import * as React from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2Icon, UserIcon } from "lucide-react";
import {
  addPlayer,
  getServerProperties,
  listPlayers,
  removePlayer,
  saveServerProperties,
  ApiError,
} from "@/lib/api";
import type { ServerPlayer } from "@/lib/types";
import { formatDateTime } from "@/lib/format";
import { useT, type TranslationKey } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { ConfirmDialog } from "@/components/confirm-dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

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

export default function PlayersTab({ serverId }: { serverId: string }) {
  const t = useT();
  const queryClient = useQueryClient();
  const [username, setUsername] = React.useState("");
  const [removeTarget, setRemoveTarget] = React.useState<ServerPlayer | null>(
    null,
  );

  const players = useQuery({
    queryKey: ["servers", serverId, "players"],
    queryFn: () => listPlayers(serverId),
  });

  // อ่านค่า white-list จาก server.properties (แชร์ cache กับ settings tab)
  const properties = useQuery({
    queryKey: ["servers", serverId, "properties"],
    queryFn: () => getServerProperties(serverId),
    retry: false,
    refetchOnWindowFocus: false,
  });

  const whitelistValue = properties.data?.values["white-list"];

  const add = useMutation({
    mutationFn: () => addPlayer(serverId, username.trim()),
    onSuccess: (data) => {
      toast.success(t("players.added", { name: data.player.username }));
      setUsername("");
      queryClient.invalidateQueries({
        queryKey: ["servers", serverId, "players"],
      });
    },
    onError: (err) => toast.error(playerErrorMessage(err, t)),
  });

  const remove = useMutation({
    mutationFn: (uuid: string) => removePlayer(serverId, uuid),
    onSuccess: () => {
      toast.success(t("players.removed"));
      setRemoveTarget(null);
      queryClient.invalidateQueries({
        queryKey: ["servers", serverId, "players"],
      });
    },
    onError: (err) => toast.error(playerErrorMessage(err, t)),
  });

  const enableWhitelist = useMutation({
    mutationFn: () => saveServerProperties(serverId, { "white-list": "true" }),
    onSuccess: () => {
      toast.success(t("players.whitelistEnabled"));
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

  return (
    <div className="grid gap-4">
      {whitelistValue === "false" && (
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
      )}
      {whitelistValue === "true" && (
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
      ) : players.data.players.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t("players.empty")}</p>
      ) : (
        <>
          {/* มือถือ: การ์ดต่อผู้เล่น (table ล้นที่จอแคบ) */}
          <div className="grid gap-2 md:hidden">
            {players.data.players.map((player) => (
              <div
                key={player.uuid}
                className="flex items-center gap-3 rounded-md border p-3"
              >
                <PlayerAvatar uuid={player.uuid} username={player.username} />
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">{player.username}</p>
                  <p className="text-muted-foreground text-xs">
                    {formatDateTime(player.added_at)}
                  </p>
                </div>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setRemoveTarget(player)}
                >
                  {t("common.remove")}
                </Button>
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
                  <TableHead>{t("players.addedAt")}</TableHead>
                  <TableHead className="text-right">
                    {t("players.actions")}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {players.data.players.map((player) => (
                  <TableRow key={player.uuid}>
                    <TableCell>
                      <PlayerAvatar
                        uuid={player.uuid}
                        username={player.username}
                      />
                    </TableCell>
                    <TableCell className="font-medium">
                      {player.username}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-xs">
                      {formatDateTime(player.added_at)}
                    </TableCell>
                    <TableCell className="text-right">
                      <Button
                        size="sm"
                        variant="outline"
                        onClick={() => setRemoveTarget(player)}
                      >
                        {t("common.remove")}
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </>
      )}

      <ConfirmDialog
        open={removeTarget !== null}
        onOpenChange={(open) => {
          if (!open) setRemoveTarget(null);
        }}
        title={t("players.removeTitle")}
        description={t("players.removeDesc", {
          name: removeTarget?.username ?? "",
        })}
        confirmLabel={t("common.remove")}
        destructive
        pending={remove.isPending}
        onConfirm={() => {
          if (removeTarget) remove.mutate(removeTarget.uuid);
        }}
      />
    </div>
  );
}
