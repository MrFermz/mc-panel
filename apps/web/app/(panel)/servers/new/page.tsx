"use client";

import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { toast } from "sonner";
import {
  FileArchiveIcon,
  FolderIcon,
  Loader2Icon,
  LockIcon,
} from "lucide-react";
import {
  apiGet,
  apiSend,
  getNextPort,
  importServer,
  ApiError,
} from "@/lib/api";
import { MemoryPresets } from "@/components/server/memory-presets";
import { ServerPropertiesCard } from "@/components/server/settings-tab";
import AccessTab from "@/components/server/access-tab";
import PlayersTab from "@/components/server/players-tab";
import {
  createServerResponseSchema,
  jobResponseSchema,
  metaNodesResponseSchema,
  metaServerTypesResponseSchema,
  serverDetailResponseSchema,
  serverResponseSchema,
  versionsResponseSchema,
  type Job,
  type Server,
} from "@/lib/types";
import { formatMb } from "@/lib/format";
import { useT, type TranslationKey } from "@/lib/i18n";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

const POLL_INTERVAL_MS = 1_500;

type SourceMode = "zip" | "folder";

// map error code จาก backend → ข้อความ toast ที่เป็นมิตร (import path)
const IMPORT_ERROR_KEYS: Record<string, TranslationKey> = {
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

interface MetadataValues {
  name: string;
  nodeId: string;
  serverType: string;
  mcVersion: string;
  memoryMb: string;
  hostPort: string;
  acceptEula: boolean;
  needsEula: boolean;
  metaValid: boolean;
}

// ฟอร์ม metadata ที่ new + import ใช้ร่วมกัน (name/node/type/version/memory/port/eula)
// return ค่าปัจจุบัน + JSX ของ field ทั้งชุด เพื่อให้ทั้งสองโหมดวางต่อ picker/ปุ่มของตัวเองได้
function useServerMetadata(disabled: boolean): MetadataValues & {
  fields: React.ReactNode;
} {
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

  const memory = Number(memoryMb);
  const port = hostPort === "" ? null : Number(hostPort);
  const metaValid =
    name.trim().length > 0 &&
    nodeId !== "" &&
    serverType !== "" &&
    mcVersion !== "" &&
    Number.isInteger(memory) &&
    memory >= 512 &&
    (port === null || (Number.isInteger(port) && port >= 1024 && port <= 65535)) &&
    (!needsEula || acceptEula);

  const fields = (
    <div className="grid gap-5">
      <div className="grid gap-2">
        <Label htmlFor="wz-name">{t("new.name")}</Label>
        <Input
          id="wz-name"
          required
          maxLength={100}
          placeholder="survival-1"
          disabled={disabled}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </div>

      <div className="grid gap-2">
        <Label>{t("new.node")}</Label>
        <Select value={nodeId} onValueChange={setNodeId} disabled={disabled}>
          <SelectTrigger>
            <SelectValue
              placeholder={
                nodesQuery.isPending ? t("new.loadingNodes") : t("new.selectNode")
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

      <div className="grid gap-5 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label>{t("new.serverType")}</Label>
          <Select
            value={serverType}
            onValueChange={(v) => {
              setServerType(v);
              setMcVersion("");
            }}
            disabled={disabled}
          >
            <SelectTrigger>
              <SelectValue
                placeholder={
                  typesQuery.isPending ? t("new.loadingTypes") : t("new.selectType")
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
            disabled={disabled || serverType === ""}
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
      </div>

      <div className="grid gap-5 sm:grid-cols-2">
        <div className="grid gap-2">
          <Label htmlFor="wz-memory">{t("new.memory")}</Label>
          <Input
            id="wz-memory"
            type="number"
            min={512}
            required
            disabled={disabled}
            value={memoryMb}
            onChange={(e) => setMemoryMb(e.target.value)}
          />
          <MemoryPresets value={memoryMb} onChange={setMemoryMb} />
        </div>

        <div className="grid gap-2">
          <Label htmlFor="wz-port">{t("new.hostPort")}</Label>
          <Input
            id="wz-port"
            type="number"
            min={1024}
            max={65535}
            placeholder="25565"
            disabled={disabled}
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
      </div>

      {needsEula && (
        <div className="flex items-start gap-2">
          <Checkbox
            id="wz-eula"
            checked={acceptEula}
            disabled={disabled}
            onCheckedChange={(v) => setAcceptEula(v === true)}
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
    </div>
  );

  return {
    name,
    nodeId,
    serverType,
    mcVersion,
    memoryMb,
    hostPort,
    acceptEula,
    needsEula,
    metaValid,
    fields,
  };
}

// ---------- General tab: create ----------

function NewServerForm({
  onCreated,
}: {
  onCreated: (server: Server, job: Job) => void;
}) {
  const t = useT();
  const meta = useServerMetadata(false);

  const create = useMutation({
    mutationFn: () =>
      apiSend(
        "POST",
        "/api/servers",
        {
          name: meta.name.trim(),
          node_id: meta.nodeId,
          server_type: meta.serverType,
          mc_version: meta.mcVersion,
          memory_mb: Number(meta.memoryMb),
          host_port: meta.hostPort === "" ? null : Number(meta.hostPort),
          accept_eula: meta.needsEula ? meta.acceptEula : true,
        },
        createServerResponseSchema,
      ),
    onSuccess: (data) => onCreated(data.server, data.job),
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : t("new.failedCreate"));
    },
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("new.title")}</CardTitle>
        <CardDescription>{t("new.subtitle")}</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="grid gap-5"
          onSubmit={(e) => {
            e.preventDefault();
            if (meta.metaValid && !create.isPending) create.mutate();
          }}
        >
          {meta.fields}
          <Button
            type="submit"
            className="justify-self-start"
            disabled={!meta.metaValid || create.isPending}
          >
            {create.isPending ? t("new.creating") : t("new.create")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

// ---------- General tab: import ----------

function ImportServerForm({
  onImported,
}: {
  onImported: (server: Server) => void;
}) {
  const t = useT();
  const meta = useServerMetadata(false);

  const [mode, setMode] = React.useState<SourceMode>("zip");
  const [zipFile, setZipFile] = React.useState<File | null>(null);
  const [folderFiles, setFolderFiles] = React.useState<File[]>([]);
  const [folderName, setFolderName] = React.useState("");
  const [progress, setProgress] = React.useState(0);
  const [zipping, setZipping] = React.useState(false);

  const zipInputRef = React.useRef<HTMLInputElement>(null);
  const folderInputRef = React.useRef<HTMLInputElement>(null);

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
      form.set("name", meta.name.trim());
      form.set("node_id", meta.nodeId);
      form.set("server_type", meta.serverType);
      form.set("mc_version", meta.mcVersion);
      form.set("memory_mb", String(Number(meta.memoryMb)));
      form.set(
        "host_port",
        meta.hostPort === "" ? "" : String(Number(meta.hostPort)),
      );
      form.set("accept_eula", String(meta.needsEula ? meta.acceptEula : true));
      form.set("archive", archive, filename);

      setProgress(0);
      return importServer(form, setProgress);
    },
    onSuccess: (data) => {
      toast.success(t("import.imported", { name: data.server.name }));
      onImported(data.server);
    },
    onError: (err) => {
      const key = err instanceof ApiError ? IMPORT_ERROR_KEYS[err.code] : undefined;
      toast.error(
        key
          ? t(key)
          : err instanceof ApiError
            ? err.message
            : t("import.errGeneric"),
      );
    },
  });

  const hasFile = mode === "zip" ? zipFile !== null : folderFiles.length > 0;
  const busy = importMut.isPending || zipping;
  const valid = meta.metaValid && hasFile;

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>{t("import.title")}</CardTitle>
          <CardDescription>{t("import.subtitle")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-5"
            onSubmit={(e) => {
              e.preventDefault();
              if (valid && !busy) importMut.mutate();
            }}
          >
            {meta.fields}

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

            <Button
              type="submit"
              className="justify-self-start"
              disabled={!valid || busy}
            >
              {busy ? t("import.uploading") : t("import.import")}
            </Button>
          </form>
        </CardContent>
      </Card>

      {busy && (
        <ImportOverlay progress={zipping ? 100 : progress} zipping={zipping} />
      )}
    </>
  );
}

// full-page overlay ระหว่างอัปโหลด/บีบอัด import — บล็อกทุก interaction จนกว่าจะจบ
function ImportOverlay({
  progress,
  zipping,
}: {
  progress: number;
  zipping: boolean;
}) {
  const t = useT();
  return (
    <div className="bg-background/80 fixed inset-0 z-[100] flex items-center justify-center backdrop-blur-sm">
      <div className="bg-card mx-4 grid w-full max-w-sm gap-4 rounded-lg border p-6 shadow-xl">
        <div className="flex items-center gap-2">
          <Loader2Icon className="text-primary size-5 animate-spin" />
          <p className="font-medium">{t("wizard.overlayTitle")}</p>
        </div>
        <div className="bg-muted h-2 overflow-hidden rounded-full">
          <div
            className="bg-primary h-full transition-all"
            style={{ width: `${progress}%` }}
          />
        </div>
        <p className="text-muted-foreground text-sm">
          {zipping ? t("import.zipping") : t("import.progress", { pct: progress })}
        </p>
        <p className="text-muted-foreground text-xs">{t("wizard.overlayHint")}</p>
      </div>
    </div>
  );
}

// ---------- General tab: created summary + light edit ----------

function CreatedGeneral({
  server,
  mode,
  jobStatus,
  onDone,
}: {
  server: Server;
  mode: "new" | "import";
  jobStatus: Job["status"] | null;
  onDone: () => void;
}) {
  const t = useT();
  const queryClient = useQueryClient();
  const [name, setName] = React.useState(server.name);
  const [memoryMb, setMemoryMb] = React.useState(String(server.memory_mb));
  const [hostPort, setHostPort] = React.useState(
    server.host_port === null ? "" : String(server.host_port),
  );

  // sync จาก server ที่ refetch มา (เช่น import ตรวจ mc_version เสร็จ → ค่าอัปเดต)
  React.useEffect(() => {
    setName(server.name);
    setMemoryMb(String(server.memory_mb));
    setHostPort(server.host_port === null ? "" : String(server.host_port));
  }, [server.name, server.memory_mb, server.host_port]);

  const save = useMutation({
    mutationFn: () =>
      apiSend(
        "PATCH",
        `/api/servers/${server.id}`,
        {
          name: name.trim(),
          memory_mb: Number(memoryMb),
          host_port: hostPort === "" ? 0 : Number(hostPort),
        },
        serverResponseSchema,
      ),
    onSuccess: () => {
      toast.success(t("sset.saved"));
      queryClient.invalidateQueries({ queryKey: ["servers", server.id] });
      queryClient.invalidateQueries({ queryKey: ["servers"] });
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : t("sset.failedSave"));
    },
  });

  const memory = Number(memoryMb);
  const port = hostPort === "" ? null : Number(hostPort);
  const valid =
    name.trim().length > 0 &&
    Number.isInteger(memory) &&
    memory >= 512 &&
    (port === null || (Number.isInteger(port) && port >= 1024 && port <= 65535));

  const provisioning = mode === "new" && jobStatus !== "succeeded";
  const provisionFailed = jobStatus === "failed";

  return (
    <div className="grid gap-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("wizard.summary")}</CardTitle>
          <CardDescription>
            {mode === "import"
              ? t("wizard.imported")
              : t("wizard.created")}
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          {provisioning && (
            <div
              className={cn(
                "flex items-center gap-2 rounded-md border p-3 text-sm",
                provisionFailed
                  ? "border-destructive/40 text-destructive"
                  : "text-muted-foreground",
              )}
            >
              {!provisionFailed && (
                <Loader2Icon className="size-4 animate-spin" />
              )}
              {provisionFailed
                ? t("wizard.provisionFailed")
                : t("wizard.provisioning")}
            </div>
          )}

          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
            <div className="flex items-center justify-between gap-2 border-b py-1">
              <dt className="text-muted-foreground">{t("wizard.summaryType")}</dt>
              <dd className="font-medium capitalize">{server.server_type}</dd>
            </div>
            <div className="flex items-center justify-between gap-2 border-b py-1">
              <dt className="text-muted-foreground">
                {t("wizard.detectedVersion")}
              </dt>
              <dd className="font-medium">{server.mc_version}</dd>
            </div>
            <div className="flex items-center justify-between gap-2 border-b py-1">
              <dt className="text-muted-foreground">{t("new.memory")}</dt>
              <dd className="font-medium">{formatMb(server.memory_mb)}</dd>
            </div>
            <div className="flex items-center justify-between gap-2 border-b py-1">
              <dt className="text-muted-foreground">{t("wizard.summaryPort")}</dt>
              <dd className="font-medium">
                {server.host_port === null ? "—" : server.host_port}
              </dd>
            </div>
          </dl>

          <Button onClick={onDone} className="justify-self-start">
            {t("wizard.done")}
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("wizard.editGeneral")}</CardTitle>
          <CardDescription>{t("sset.generalDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="grid max-w-md gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (valid && !save.isPending) save.mutate();
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="wz-c-name">{t("sset.name")}</Label>
              <Input
                id="wz-c-name"
                maxLength={100}
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="wz-c-memory">{t("sset.memory")}</Label>
              <Input
                id="wz-c-memory"
                type="number"
                min={512}
                value={memoryMb}
                onChange={(e) => setMemoryMb(e.target.value)}
              />
              <MemoryPresets value={memoryMb} onChange={setMemoryMb} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="wz-c-port">{t("sset.hostPort")}</Label>
              <Input
                id="wz-c-port"
                type="number"
                min={1024}
                max={65535}
                placeholder={t("sset.hostPortPlaceholder")}
                value={hostPort}
                onChange={(e) => setHostPort(e.target.value)}
              />
            </div>
            <Button
              type="submit"
              className="justify-self-start"
              disabled={!valid || save.isPending}
            >
              {save.isPending ? t("common.saving") : t("sset.saveChanges")}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}

// hint การ์ดสำหรับ tab ที่ยังล็อกอยู่ (ก่อนสร้าง/นำเข้าเซิร์ฟเวอร์)
function LockedTab() {
  const t = useT();
  return (
    <Card className="py-12">
      <CardContent className="text-muted-foreground flex flex-col items-center gap-3 text-center text-sm">
        <LockIcon className="size-6" />
        <p>{t("wizard.lockedHint")}</p>
      </CardContent>
    </Card>
  );
}

function NewServerWizard() {
  const t = useT();
  const router = useRouter();
  const queryClient = useQueryClient();
  const searchParams = useSearchParams();
  const mode = searchParams.get("mode") === "import" ? "import" : "new";

  const [createdId, setCreatedId] = React.useState<string | null>(null);
  const [jobId, setJobId] = React.useState<string | null>(null);
  const [ready, setReady] = React.useState(false);
  const [activeTab, setActiveTab] = React.useState("general");

  const created = createdId !== null;

  useSetBreadcrumbs(
    React.useMemo(
      () => [
        {
          label:
            mode === "import"
              ? t("wizard.importBreadcrumb")
              : t("wizard.newBreadcrumb"),
        },
      ],
      [mode, t],
    ),
  );

  // ดึง server สด ๆ หลังสร้าง/นำเข้า — import ตรวจ mc_version เองแล้วอัปเดต DB
  // จึงต้อง refetch เพื่อโชว์เวอร์ชันที่ตรวจพบ + ให้ properties/players tab เห็นไฟล์จริง
  const serverQuery = useQuery({
    queryKey: ["servers", createdId],
    queryFn: () =>
      apiGet(`/api/servers/${createdId}`, serverDetailResponseSchema),
    enabled: createdId !== null,
  });
  const server = serverQuery.data?.server;

  // new mode: poll create_server job จนเสร็จ แล้วปลดล็อกแท็บอื่น (ไฟล์เพิ่งถูกเขียน)
  const jobQuery = useQuery({
    queryKey: ["jobs", jobId],
    queryFn: () => apiGet(`/api/jobs/${jobId}`, jobResponseSchema),
    enabled: jobId !== null && !ready,
    refetchInterval: (query) => {
      const status = query.state.data?.job.status;
      return status === "succeeded" || status === "failed"
        ? false
        : POLL_INTERVAL_MS;
    },
  });
  const jobStatus = jobQuery.data?.job.status ?? null;

  React.useEffect(() => {
    if (jobStatus === "succeeded") {
      setReady(true);
      queryClient.invalidateQueries({ queryKey: ["servers", createdId] });
    }
  }, [jobStatus, createdId, queryClient]);

  const onCreated = React.useCallback(
    (s: Server, job: Job) => {
      setCreatedId(s.id);
      setJobId(job.id);
      queryClient.invalidateQueries({ queryKey: ["servers"] });
    },
    [queryClient],
  );

  const onImported = React.useCallback(
    (s: Server) => {
      setCreatedId(s.id);
      setReady(true);
      queryClient.invalidateQueries({ queryKey: ["servers"] });
    },
    [queryClient],
  );

  const goToServer = React.useCallback(() => {
    if (createdId) router.push(`/servers/${createdId}`);
  }, [createdId, router]);

  // แท็บ properties/players อ่านไฟล์จริงบนโหนด — เปิดได้เมื่อไฟล์พร้อม (ready)
  const tabsUnlocked = created && ready;

  return (
    <div className="grid gap-6">
      <div className="grid gap-1">
        <h1 className="text-xl font-semibold">
          {mode === "import" ? t("import.title") : t("new.title")}
        </h1>
        <p className="text-muted-foreground text-sm">
          {mode === "import" ? t("import.subtitle") : t("new.subtitle")}
        </p>
      </div>

      <Tabs value={activeTab} onValueChange={setActiveTab}>
        <div className="max-w-full overflow-x-auto">
          <TabsList className="w-max max-w-none">
            <TabsTrigger value="general">{t("wizard.tabGeneral")}</TabsTrigger>
            <TabsTrigger value="properties" disabled={!tabsUnlocked}>
              {t("wizard.tabProperties")}
            </TabsTrigger>
            <TabsTrigger value="access" disabled={!created}>
              {t("wizard.tabAccess")}
            </TabsTrigger>
            <TabsTrigger value="players" disabled={!tabsUnlocked}>
              {t("wizard.tabPlayers")}
            </TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="general" className="mt-4">
          {!created ? (
            mode === "import" ? (
              <ImportServerForm onImported={onImported} />
            ) : (
              <NewServerForm onCreated={onCreated} />
            )
          ) : server ? (
            <CreatedGeneral
              server={server}
              mode={mode}
              jobStatus={jobStatus}
              onDone={goToServer}
            />
          ) : (
            <Skeleton className="h-64 w-full" />
          )}
        </TabsContent>

        <TabsContent value="properties" className="mt-4">
          {tabsUnlocked && server ? (
            <ServerPropertiesCard serverId={server.id} />
          ) : (
            <LockedTab />
          )}
        </TabsContent>

        <TabsContent value="access" className="mt-4">
          {created && server ? <AccessTab serverId={server.id} /> : <LockedTab />}
        </TabsContent>

        <TabsContent value="players" className="mt-4">
          {tabsUnlocked && server ? (
            <PlayersTab serverId={server.id} />
          ) : (
            <LockedTab />
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}

// useSearchParams ต้องอยู่ใน Suspense boundary ไม่งั้น build บ่น CSR bailout
export default function NewServerPage() {
  return (
    <React.Suspense
      fallback={
        <div className="grid gap-4">
          <Skeleton className="h-10 w-64" />
          <Skeleton className="h-96 w-full" />
        </div>
      }
    >
      <NewServerWizard />
    </React.Suspense>
  );
}
