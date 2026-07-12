"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { FileArchiveIcon, FolderIcon } from "lucide-react";
import { apiGet, getNextPort, importServer, ApiError } from "@/lib/api";
import { MemoryPresets } from "@/components/server/memory-presets";
import {
  metaNodesResponseSchema,
  metaServerTypesResponseSchema,
  versionsResponseSchema,
  type Server,
} from "@/lib/types";
import { useT, type TranslationKey } from "@/lib/i18n";
import { useUiStore } from "@/lib/settings/ui-store";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

type SourceMode = "zip" | "folder";

// map error code จาก backend → ข้อความ toast ที่เป็นมิตร
const ERROR_KEYS: Record<string, TranslationKey> = {
  eula_required: "import.errEulaRequired",
  empty_archive: "import.errEmptyArchive",
  host_port_taken: "import.errHostPortTaken",
  node_offline: "import.errNodeOffline",
  agent_timeout: "import.errAgentTimeout",
  import_failed: "import.errImportFailed",
};

// สร้าง .zip จากโฟลเดอร์ที่ user เลือก โดยตัดชื่อโฟลเดอร์บนสุดออก
// เพื่อให้ไฟล์เซิร์ฟเวอร์ (server.properties, world/ ...) อยู่ที่ root ของ archive
async function zipFolder(files: File[]): Promise<Blob> {
  const { default: JSZip } = await import("jszip");
  const zip = new JSZip();
  for (const file of files) {
    const rel = file.webkitRelativePath || file.name;
    const slash = rel.indexOf("/");
    const path = slash >= 0 ? rel.slice(slash + 1) : rel;
    if (path === "") continue;
    zip.file(path, file);
  }
  return zip.generateAsync({ type: "blob", compression: "DEFLATE" });
}

function ImportServerForm({ onDone }: { onDone: (server: Server) => void }) {
  const t = useT();
  const [name, setName] = React.useState("");
  const [nodeId, setNodeId] = React.useState("");
  const [serverType, setServerType] = React.useState("");
  const [mcVersion, setMcVersion] = React.useState("");
  const [memoryMb, setMemoryMb] = React.useState("2048");
  const [hostPort, setHostPort] = React.useState("");
  // จำว่า user แตะช่อง port เองหรือยัง — ถ้าแตะแล้วห้าม auto-prefill ทับ
  const [portEdited, setPortEdited] = React.useState(false);
  const [acceptEula, setAcceptEula] = React.useState(false);

  const [mode, setMode] = React.useState<SourceMode>("zip");
  const [zipFile, setZipFile] = React.useState<File | null>(null);
  const [folderFiles, setFolderFiles] = React.useState<File[]>([]);
  const [folderName, setFolderName] = React.useState("");
  const [progress, setProgress] = React.useState(0);
  const [zipping, setZipping] = React.useState(false);

  const zipInputRef = React.useRef<HTMLInputElement>(null);
  const folderInputRef = React.useRef<HTMLInputElement>(null);

  const nodesQuery = useQuery({
    queryKey: ["meta", "nodes"],
    queryFn: () => apiGet("/api/meta/nodes", metaNodesResponseSchema),
  });

  // แนะนำ host port ว่างของ node ที่เลือก — พังก็ปล่อยช่องว่างไว้เฉย ๆ (ไม่ crash)
  const nextPortQuery = useQuery({
    queryKey: ["meta", "next-port", nodeId],
    queryFn: () => getNextPort(nodeId),
    enabled: nodeId !== "",
    retry: false,
  });

  const suggestedPort = nextPortQuery.data;
  React.useEffect(() => {
    if (!portEdited && suggestedPort !== undefined) {
      setHostPort(String(suggestedPort));
    }
  }, [portEdited, suggestedPort]);

  const typesQuery = useQuery({
    queryKey: ["meta", "server-types"],
    queryFn: () => apiGet("/api/meta/server-types", metaServerTypesResponseSchema),
  });
  const versionsQuery = useQuery({
    queryKey: ["meta", "versions", serverType],
    queryFn: () =>
      apiGet(
        `/api/meta/versions?type=${encodeURIComponent(serverType)}`,
        versionsResponseSchema,
      ),
    enabled: serverType !== "",
  });

  const selectedType = typesQuery.data?.types.find((x) => x.id === serverType);
  const needsEula = selectedType?.needs_eula ?? serverType !== "velocity";

  const onZipChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] ?? null;
    if (file && !file.name.toLowerCase().endsWith(".zip")) {
      toast.error(t("import.notZip"));
      setZipFile(null);
      e.target.value = "";
      return;
    }
    setZipFile(file);
  };

  const onFolderChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const list = e.target.files;
    if (!list || list.length === 0) {
      setFolderFiles([]);
      setFolderName("");
      return;
    }
    const files = Array.from(list);
    const first = files[0]?.webkitRelativePath ?? "";
    const top = first.includes("/") ? first.slice(0, first.indexOf("/")) : "";
    setFolderName(top || t("import.folder"));
    setFolderFiles(files);
  };

  const importMut = useMutation({
    mutationFn: async () => {
      let archive: Blob;
      let filename: string;
      if (mode === "zip") {
        archive = zipFile as File;
        filename = (zipFile as File).name;
      } else {
        setZipping(true);
        try {
          archive = await zipFolder(folderFiles);
        } finally {
          setZipping(false);
        }
        filename = "import.zip";
      }

      const form = new FormData();
      form.set("name", name.trim());
      form.set("node_id", nodeId);
      form.set("server_type", serverType);
      form.set("mc_version", mcVersion);
      form.set("memory_mb", String(Number(memoryMb)));
      form.set("host_port", hostPort === "" ? "" : String(Number(hostPort)));
      form.set("accept_eula", String(needsEula ? acceptEula : true));
      form.set("archive", archive, filename);

      setProgress(0);
      return importServer(form, setProgress);
    },
    onSuccess: (data) => {
      toast.success(t("import.imported", { name: data.server.name }));
      onDone(data.server);
    },
    onError: (err) => {
      const key = err instanceof ApiError ? ERROR_KEYS[err.code] : undefined;
      if (key) {
        toast.error(t(key));
      } else {
        toast.error(
          err instanceof ApiError ? err.message : t("import.errGeneric"),
        );
      }
    },
  });

  const memory = Number(memoryMb);
  const port = hostPort === "" ? null : Number(hostPort);
  const hasFile = mode === "zip" ? zipFile !== null : folderFiles.length > 0;
  const busy = importMut.isPending || zipping;
  const valid =
    name.trim().length > 0 &&
    nodeId !== "" &&
    serverType !== "" &&
    mcVersion !== "" &&
    Number.isInteger(memory) &&
    memory >= 512 &&
    (port === null || (Number.isInteger(port) && port >= 1024 && port <= 65535)) &&
    hasFile &&
    (!needsEula || acceptEula);

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (valid && !busy) importMut.mutate();
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("import.title")}</DialogTitle>
        <DialogDescription>{t("import.subtitle")}</DialogDescription>
      </DialogHeader>
      <form onSubmit={onSubmit} className="grid gap-5">
        <div className="grid gap-2">
          <Label htmlFor="import-name">{t("new.name")}</Label>
          <Input
            id="import-name"
            required
            maxLength={100}
            placeholder="survival-1"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </div>

        <div className="grid gap-2">
          <Label>{t("new.node")}</Label>
          <Select value={nodeId} onValueChange={setNodeId}>
            <SelectTrigger>
              <SelectValue
                placeholder={
                  nodesQuery.isPending
                    ? t("new.loadingNodes")
                    : t("new.selectNode")
                }
              />
            </SelectTrigger>
            <SelectContent>
              {(nodesQuery.data?.nodes ?? []).map((node) => (
                <SelectItem key={node.id} value={node.id}>
                  {node.name}
                  {node.status !== "online" ? ` (${node.status})` : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-2">
          <Label>{t("new.serverType")}</Label>
          <Select
            value={serverType}
            onValueChange={(v) => {
              setServerType(v);
              setMcVersion("");
            }}
          >
            <SelectTrigger>
              <SelectValue
                placeholder={
                  typesQuery.isPending
                    ? t("new.loadingTypes")
                    : t("new.selectType")
                }
              />
            </SelectTrigger>
            <SelectContent>
              {(typesQuery.data?.types ?? []).map((ty) => (
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
            value={mcVersion}
            onValueChange={setMcVersion}
            disabled={serverType === ""}
          >
            <SelectTrigger>
              <SelectValue
                placeholder={
                  serverType === ""
                    ? t("new.pickTypeFirst")
                    : versionsQuery.isPending
                      ? t("new.loadingVersions")
                      : t("new.selectVersion")
                }
              />
            </SelectTrigger>
            <SelectContent>
              {(versionsQuery.data?.versions ?? []).map((v) => (
                <SelectItem key={v} value={v}>
                  {v}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {versionsQuery.isError && (
            <p className="text-destructive text-xs">{t("new.failedVersions")}</p>
          )}
        </div>

        <div className="grid gap-2">
          <Label htmlFor="import-memory">{t("new.memory")}</Label>
          <Input
            id="import-memory"
            type="number"
            min={512}
            required
            value={memoryMb}
            onChange={(e) => setMemoryMb(e.target.value)}
          />
          <MemoryPresets value={memoryMb} onChange={setMemoryMb} />
        </div>

        <div className="grid gap-2">
          <Label htmlFor="import-port">{t("new.hostPort")}</Label>
          <Input
            id="import-port"
            type="number"
            min={1024}
            max={65535}
            placeholder="25565"
            value={hostPort}
            onChange={(e) => {
              setPortEdited(true);
              setHostPort(e.target.value);
            }}
          />
          <p className="text-muted-foreground text-xs">
            {t("new.hostPortEmptyHint")}
          </p>
        </div>

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
                onChange={onZipChange}
              />
              <Button
                type="button"
                variant="outline"
                onClick={() => zipInputRef.current?.click()}
              >
                {t("import.selectZip")}
              </Button>
              {zipFile && (
                <p className="text-muted-foreground truncate text-xs">
                  {t("import.selected", { name: zipFile.name })}
                </p>
              )}
            </>
          ) : (
            <>
              <input
                ref={folderInputRef}
                type="file"
                className="hidden"
                onChange={onFolderChange}
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
              {folderFiles.length > 0 && (
                <p className="text-muted-foreground truncate text-xs">
                  {t("import.selectedFolder", {
                    name: folderName,
                    count: folderFiles.length,
                  })}
                </p>
              )}
            </>
          )}
          <p className="text-muted-foreground text-xs">
            {t("import.selectHint")}
          </p>
        </div>

        {needsEula && (
          <div className="flex items-start gap-2">
            <Checkbox
              id="import-eula"
              checked={acceptEula}
              onCheckedChange={(v) => setAcceptEula(v === true)}
            />
            <Label htmlFor="import-eula" className="flex-wrap font-normal">
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

        {busy && (
          <div className="grid gap-1.5">
            <div className="bg-muted h-2 overflow-hidden rounded-full">
              <div
                className="bg-primary h-full transition-all"
                style={{ width: `${zipping ? 100 : progress}%` }}
              />
            </div>
            <p className="text-muted-foreground text-xs">
              {zipping
                ? t("import.zipping")
                : t("import.progress", { pct: progress })}
            </p>
          </div>
        )}

        <Button type="submit" disabled={!valid || busy}>
          {busy ? t("import.uploading") : t("import.import")}
        </Button>
      </form>
    </>
  );
}

export function ImportServerDialog() {
  const open = useUiStore((s) => s.importServerOpen);
  const setOpen = useUiStore((s) => s.setImportServerOpen);
  const router = useRouter();
  const queryClient = useQueryClient();

  const finish = React.useCallback(
    (server: Server) => {
      queryClient.invalidateQueries({ queryKey: ["servers"] });
      setOpen(false);
      router.push(`/servers/${server.id}`);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [queryClient, router],
  );

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        {/* mount เฉพาะตอนเปิด → form reset สะอาดทุกครั้งที่เปิดใหม่ */}
        {open && <ImportServerForm onDone={finish} />}
      </DialogContent>
    </Dialog>
  );
}
