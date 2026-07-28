"use client";

import { formatMb } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { MemoryPresets } from "@/components/server/memory-presets";
import type { ServerMetadata } from "@/components/server/new-server/use-server-metadata";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import type { GameProfile } from "@/lib/games";

// step 1 — ข้อมูลพื้นฐานของ server
// เป็นด่านเดียวที่บังคับกรอกให้ครบ ที่เหลือข้ามได้
// เกมถูกเลือกมาแล้วจากหน้าก่อนหน้า (ไม่มีช่องให้เปลี่ยนที่นี่) — ชื่อ/ลิงก์ license
// และกติกาอื่น ๆ ของเกมมาจาก profile ที่ส่งเข้ามา ไม่ hardcode ใน wizard
export function StepGeneral({
  meta,
  profile,
}: {
  meta: ServerMetadata;
  profile: GameProfile;
}) {
  const t = useT();
  const { budget } = meta;
  const game = profile;

  return (
    <Card>
      <CardHeader className="flex-row items-center gap-3">
        {/* eslint-disable-next-line @next/next/no-img-element -- ปกเป็น SVG static ใน public/ */}
        <img
          src={game.coverSrc}
          alt=""
          className="h-11 w-16 shrink-0 rounded-md border object-cover"
        />
        <div className="grid gap-1">
          <CardTitle>{t("new.titleForGame", { game: game.label })}</CardTitle>
          <CardDescription>{t(game.descriptionKey)}</CardDescription>
        </div>
      </CardHeader>
      <CardContent className="grid gap-5">
        <div className="grid gap-2">
          <Label htmlFor="wz-name">{t("new.name")}</Label>
          <Input
            id="wz-name"
            required
            maxLength={100}
            placeholder="survival-1"
            value={meta.name}
            onChange={(e) => meta.setName(e.target.value)}
          />
        </div>

        <div className="grid gap-2">
          <Label>{t("new.node")}</Label>
          <Select value={meta.nodeId} onValueChange={meta.setNodeId}>
            <SelectTrigger>
              <SelectValue
                placeholder={
                  meta.nodesPending
                    ? t("new.loadingNodes")
                    : t("new.selectNode")
                }
              />
            </SelectTrigger>
            <SelectContent>
              {meta.nodes.map((node) => (
                <SelectItem key={node.id} value={node.id}>
                  {node.name}
                  {node.status !== "online" ? ` (${node.status})` : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-5 sm:grid-cols-2">
          <div className="grid gap-2">
            <Label>{t("new.variant")}</Label>
            <Select
              value={meta.variant}
              onValueChange={meta.setVariant}
            >
              <SelectTrigger>
                <SelectValue
                  placeholder={
                    meta.typesPending
                      ? t("new.loadingTypes")
                      : t("new.selectType")
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {meta.types.map((ty) => (
                  <SelectItem key={ty.id} value={ty.id}>
                    {ty.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-2">
            <Label>{t("new.version")}</Label>
            <Select
              value={meta.gameVersion}
              onValueChange={meta.setGameVersion}
              disabled={meta.variant === ""}
            >
              <SelectTrigger>
                <SelectValue
                  placeholder={
                    meta.variant === ""
                      ? t("new.pickTypeFirst")
                      : meta.versionsPending
                        ? t("new.loadingVersions")
                        : t("new.selectVersion")
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {meta.versions.map((v) => (
                  <SelectItem key={v} value={v}>
                    {v}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {meta.versionsError && (
              <p className="text-destructive text-xs">
                {t("new.failedVersions")}
              </p>
            )}
          </div>
        </div>

        <div className="grid gap-5 sm:grid-cols-2">
          <div className="grid gap-2">
            <Label htmlFor="wz-memory">{t("new.memory")}</Label>
            <Input
              id="wz-memory"
              type="number"
              min={meta.minMemoryMb}
              required
              value={meta.memoryMb}
              onChange={(e) => meta.setMemoryMb(e.target.value)}
            />
            <MemoryPresets value={meta.memoryMb} onChange={meta.setMemoryMb} />
            {budget && (
              <p
                className={cn(
                  "text-xs",
                  budget.over ? "text-destructive" : "text-muted-foreground",
                )}
              >
                {t("new.ramBudget", {
                  free: formatMb(budget.freeMb),
                  total: formatMb(budget.totalMb),
                  used: formatMb(budget.usedMb),
                })}
              </p>
            )}
            {budget?.over && (
              <p className="text-destructive text-xs">
                {t("new.ramOverBudget")}
              </p>
            )}
          </div>

          <div className="grid gap-2">
            <Label htmlFor="wz-port">{t("new.hostPort")}</Label>
            <Input
              id="wz-port"
              type="number"
              min={1024}
              max={65535}
              value={meta.hostPort}
              onChange={(e) => meta.setHostPort(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              {t("new.hostPortEmptyHint")}
            </p>
          </div>
        </div>

        {meta.requiresLicense && (
          <div className="flex items-start gap-2">
            <Checkbox
              id="wz-eula"
              checked={meta.acceptLicense}
              onCheckedChange={(v) => meta.setAcceptLicense(v === true)}
            />
            <Label htmlFor="wz-eula" className="flex-wrap font-normal">
              <span>
                {t("new.licenseAccept")}{" "}
                <a
                  href={game.licenseUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="underline underline-offset-2"
                >
                  {t("new.licenseLink", { license: game.licenseName })}
                </a>
              </span>
            </Label>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
