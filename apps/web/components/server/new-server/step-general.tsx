"use client";

import * as React from "react";
import { FileArchiveIcon, FolderIcon, Loader2Icon } from "lucide-react";
import { formatMb } from "@/lib/format";
import { useT } from "@/lib/i18n";
import { MemoryPresets } from "@/components/server/memory-presets";
import type { ServerMetadata } from "@/components/server/new-server/use-server-metadata";
import type { ImportSource } from "@/components/server/new-server/use-import-source";
import type { WizardMode } from "@/components/server/new-server/steps";
import { Button } from "@/components/ui/button";
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

// ตัวเลือกไฟล์ต้นทาง — โผล่เฉพาะโหมด import
function ImportSourcePicker({ source }: { source: ImportSource }) {
  const t = useT();
  const zipInputRef = React.useRef<HTMLInputElement>(null);
  const folderInputRef = React.useRef<HTMLInputElement>(null);
  const { mode, setMode, detected, detecting } = source;

  return (
    <div className="grid gap-2">
      <Label>{t("import.source")}</Label>
      <div className="bg-muted grid grid-cols-2 gap-0.5 rounded-md p-0.5">
        <button
          type="button"
          onClick={() => setMode("zip")}
          className={cn(
            "flex items-center justify-center gap-1.5 rounded-sm py-1.5 text-sm transition-colors",
            mode === "zip"
              ? "bg-background text-foreground shadow-xs"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          <FileArchiveIcon className="size-4" />
          {t("import.zipFile")}
        </button>
        <button
          type="button"
          onClick={() => setMode("folder")}
          className={cn(
            "flex items-center justify-center gap-1.5 rounded-sm py-1.5 text-sm transition-colors",
            mode === "folder"
              ? "bg-background text-foreground shadow-xs"
              : "text-muted-foreground hover:text-foreground",
          )}
        >
          <FolderIcon className="size-4" />
          {t("import.folder")}
        </button>
      </div>

      {mode === "zip" ? (
        <>
          <input
            ref={zipInputRef}
            type="file"
            accept=".zip"
            className="hidden"
            onChange={source.onZipChange}
          />
          <Button
            type="button"
            variant="outline"
            onClick={() => zipInputRef.current?.click()}
          >
            {t("import.selectZip")}
          </Button>
          {source.zipFile && (
            <p className="text-muted-foreground truncate text-xs">
              {t("import.selected", { name: source.zipFile.name })}
            </p>
          )}
        </>
      ) : (
        <>
          <input
            ref={folderInputRef}
            type="file"
            className="hidden"
            onChange={source.onFolderChange}
            // webkitdirectory ไม่มีใน React types — ต้อง cast
            // eslint-disable-next-line @typescript-eslint/no-explicit-any
            {...({ webkitdirectory: "", directory: "" } as any)}
          />
          <Button
            type="button"
            variant="outline"
            onClick={() => folderInputRef.current?.click()}
          >
            {t("import.selectFolder")}
          </Button>
          {source.folderFiles.length > 0 && (
            <p className="text-muted-foreground truncate text-xs">
              {t("import.selectedFolder", {
                name: source.folderName,
                count: source.folderFiles.length,
              })}
            </p>
          )}
        </>
      )}
      <p className="text-muted-foreground text-xs">{t("import.selectHint")}</p>
      {detecting && (
        <p className="text-muted-foreground flex items-center gap-1.5 text-xs">
          <Loader2Icon className="size-3.5 animate-spin" />
          {t("wizard.detecting")}
        </p>
      )}
      {!detecting && detected && (detected.serverType || detected.mcVersion) && (
        <p className="text-muted-foreground text-xs">
          {t("wizard.detectedHint", {
            type: detected.serverType ?? "—",
            version: detected.mcVersion ?? "—",
          })}
        </p>
      )}
    </div>
  );
}

// step 1 — ข้อมูลพื้นฐานของ server (+ ไฟล์ต้นทางเมื่อเป็นโหมด import)
// เป็นด่านเดียวที่บังคับกรอกให้ครบ ที่เหลือข้ามได้
export function StepGeneral({
  mode,
  meta,
  importSource,
}: {
  mode: WizardMode;
  meta: ServerMetadata;
  importSource: ImportSource;
}) {
  const t = useT();
  const { budget } = meta;

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {mode === "import" ? t("import.title") : t("new.title")}
        </CardTitle>
        <CardDescription>
          {mode === "import" ? t("import.subtitle") : t("new.subtitle")}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-5">
        {mode === "import" && <ImportSourcePicker source={importSource} />}

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
            <Label>{t("new.serverType")}</Label>
            <Select
              value={meta.serverType}
              onValueChange={meta.setServerType}
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
              value={meta.mcVersion}
              onValueChange={meta.setMcVersion}
              disabled={meta.serverType === ""}
            >
              <SelectTrigger>
                <SelectValue
                  placeholder={
                    meta.serverType === ""
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
              min={512}
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
              placeholder="25565"
              value={meta.hostPort}
              onChange={(e) => meta.setHostPort(e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              {t("new.hostPortEmptyHint")}
            </p>
          </div>
        </div>

        {meta.needsEula && (
          <div className="flex items-start gap-2">
            <Checkbox
              id="wz-eula"
              checked={meta.acceptEula}
              onCheckedChange={(v) => meta.setAcceptEula(v === true)}
            />
            <Label htmlFor="wz-eula" className="flex-wrap font-normal">
              <span>
                {t("new.eulaAccept")}{" "}
                <a
                  href="https://www.minecraft.net/en-us/eula"
                  target="_blank"
                  rel="noreferrer"
                  className="underline underline-offset-2"
                >
                  {t("new.eulaLink")}
                </a>
              </span>
            </Label>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
