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
  CheckIcon,
  FileArchiveIcon,
  FolderIcon,
  Loader2Icon,
} from "lucide-react";
import {
  apiGet,
  apiSend,
  getNextPort,
  importServer,
  ApiError,
} from "@/lib/api";
import { MemoryPresets } from "@/components/server/memory-presets";
import { ServerPropertiesCard } from "@/components/server/server-settings";
import ServerAccess from "@/components/server/server-access";
import ServerPlayers from "@/components/server/server-players";
import {
  createServerResponseSchema,
  jobResponseSchema,
  metaNodesResponseSchema,
  metaServerTypesResponseSchema,
  nodesResponseSchema,
  serverDetailResponseSchema,
  serversResponseSchema,
  versionsResponseSchema,
  type Job,
  type Server,
} from "@/lib/types";
import { formatMb } from "@/lib/format";
import { useT, type TranslationKey } from "@/lib/i18n";
import { useSettingsStore } from "@/lib/settings/store";
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

// ลำดับ step ของ wizard (แทน tabs เดิม) — index ตรงกับ state `step`
const WIZARD_STEPS = [
  { key: "general", titleKey: "wizard.tabGeneral" },
  { key: "properties", titleKey: "wizard.tabProperties" },
  { key: "access", titleKey: "wizard.tabAccess" },
  { key: "players", titleKey: "wizard.tabPlayers" },
] as const satisfies ReadonlyArray<{ key: string; titleKey: TranslationKey }>;

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

// ---------- client-side detection ของ archive ก่อน upload (Task 4) ----------

type Detected = { name?: string; serverType?: string; mcVersion?: string };

// เดา server_type จากชื่อ jar
function guessServerType(jarName: string): string {
  const n = jarName.toLowerCase();
  if (/paper|purpur|spigot/.test(n)) return "paper";
  if (/fabric/.test(n)) return "fabric";
  if (/forge/.test(n)) return "forge";
  if (/velocity/.test(n)) return "velocity";
  return "vanilla";
}

// fallback: ดึงเลขเวอร์ชันจากชื่อไฟล์ jar
function versionFromName(name: string): string | undefined {
  const m = name.match(/\d+\.\d+(?:\.\d+)?/);
  return m ? m[0] : undefined;
}

// server jar เป็น zip — root มี version.json ที่ระบุ mc version จริง (id/name)
async function versionFromJarBytes(
  bytes: Uint8Array,
): Promise<string | undefined> {
  try {
    const { default: JSZip } = await import("jszip");
    const inner = await JSZip.loadAsync(bytes);
    const entry = inner.file("version.json");
    if (!entry) return undefined;
    const raw = await entry.async("string");
    const json = JSON.parse(raw) as { id?: string; name?: string };
    return json.id || json.name || undefined;
  } catch {
    return undefined;
  }
}

// jar ที่อยู่ root ของ archive (ไม่มี "/" ในชื่อ) — prefer ชื่อที่บอก type ชัด
function pickRootJar(paths: string[]): string | undefined {
  const jars = paths.filter(
    (p) => p.toLowerCase().endsWith(".jar") && !p.includes("/"),
  );
  if (jars.length === 0) return undefined;
  return (
    jars.find((p) =>
      /paper|purpur|spigot|vanilla|fabric|forge|velocity|server/i.test(p),
    ) ?? jars[0]
  );
}

// import โหมด zip: อ่านตรงจากไฟล์ .zip ที่เลือก (ไม่ต้อง extract ทั้งก้อน)
async function detectFromZip(file: File): Promise<Detected> {
  const detected: Detected = { name: file.name.replace(/\.zip$/i, "") };
  try {
    const { default: JSZip } = await import("jszip");
    const outer = await JSZip.loadAsync(file);
    const jarPath = pickRootJar(Object.keys(outer.files));
    if (jarPath) {
      detected.serverType = guessServerType(jarPath);
      const entry = outer.file(jarPath);
      const bytes = entry ? await entry.async("uint8array") : undefined;
      detected.mcVersion =
        (bytes ? await versionFromJarBytes(bytes) : undefined) ??
        versionFromName(jarPath);
    }
  } catch {
    // ล้มเหลว = ปล่อยให้กรอกเอง ไม่ block
  }
  return detected;
}

// path ของไฟล์ในโฟลเดอร์ โดยตัดชื่อโฟลเดอร์บนสุดออก (เทียบ root ของ archive)
function rootRelPath(f: File): string {
  const rel = f.webkitRelativePath || f.name;
  const slash = rel.indexOf("/");
  return slash >= 0 ? rel.slice(slash + 1) : rel;
}

// import โหมด folder: หา jar ที่ root ตรงจาก File[] แล้วอ่าน version.json ข้างใน
async function detectFromFolder(
  files: File[],
  folderName: string,
): Promise<Detected> {
  const detected: Detected = { name: folderName || undefined };
  try {
    const jarFiles = files.filter((f) => {
      const p = rootRelPath(f).toLowerCase();
      return p.endsWith(".jar") && !p.includes("/");
    });
    const jar =
      jarFiles.find((f) =>
        /paper|purpur|spigot|vanilla|fabric|forge|velocity|server/i.test(
          rootRelPath(f),
        ),
      ) ?? jarFiles[0];
    if (jar) {
      detected.serverType = guessServerType(rootRelPath(jar));
      const bytes = new Uint8Array(await jar.arrayBuffer());
      detected.mcVersion =
        (await versionFromJarBytes(bytes)) ??
        versionFromName(rootRelPath(jar));
    }
  } catch {
    // ล้มเหลว = ปล่อยให้กรอกเอง ไม่ block
  }
  return detected;
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
// return ค่าปัจจุบัน + setter ที่ import ใช้ prefill + JSX ของ field ทั้งชุด
function useServerMetadata(disabled: boolean): MetadataValues & {
  fields: React.ReactNode;
  setName: (v: string) => void;
  setServerType: (v: string) => void;
  setMcVersion: (v: string) => void;
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

  // งบ RAM ต่อโหนด: total ของ node − ผลรวม memory_mb ของ server ที่มีอยู่บนโหนดนั้น
  // ทั้งสอง query แชร์ cache กับ dashboard (["nodes"], ["servers"]) — พังก็แค่ไม่โชว์ hint
  const nodesFullQuery = useQuery({
    queryKey: ["nodes"],
    queryFn: () => apiGet("/api/nodes", nodesResponseSchema),
    retry: false,
  });
  const serversQuery = useQuery({
    queryKey: ["servers"],
    queryFn: () => apiGet("/api/servers", serversResponseSchema),
    retry: false,
  });

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

  // งบ RAM ของโหนดที่เลือก (backend เป็น source of truth จริง — นี่แค่ช่วยเตือนล่วงหน้า)
  const selectedNode = nodesFullQuery.data?.nodes.find((n) => n.id === nodeId);
  const usedMb = (serversQuery.data?.servers ?? [])
    .filter((s) => s.node_id === nodeId)
    .reduce((sum, s) => sum + s.memory_mb, 0);
  const totalMb = selectedNode?.memory_total_mb ?? 0;
  const freeMb = Math.max(0, totalMb - usedMb);
  const overBudget =
    selectedNode !== undefined &&
    Number.isInteger(memory) &&
    memory > 0 &&
    memory > freeMb;

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
          {selectedNode && (
            <p
              className={cn(
                "text-xs",
                overBudget ? "text-destructive" : "text-muted-foreground",
              )}
            >
              {t("new.ramBudget", {
                free: formatMb(freeMb),
                total: formatMb(totalMb),
                used: formatMb(usedMb),
              })}
            </p>
          )}
          {overBudget && (
            <p className="text-destructive text-xs">{t("new.ramOverBudget")}</p>
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
    setName,
    setServerType,
    setMcVersion,
  };
}

// ---------- Step 1: create ----------

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
      // insufficient_memory: message มีตัวเลข used/total มาแล้ว — โชว์ตรง ๆ
      if (err instanceof ApiError && err.code === "insufficient_memory") {
        toast.error(err.message || t("new.errInsufficientMemory"));
        return;
      }
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
            {create.isPending ? t("new.creating") : t("wizard.createContinue")}
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

// ---------- Step 1: import ----------

function ImportServerForm({
  onImported,
}: {
  onImported: (server: Server) => void;
}) {
  const t = useT();
  const meta = useServerMetadata(false);
  const { setName, setServerType, setMcVersion } = meta;
  const metaName = meta.name;

  const [mode, setMode] = React.useState<SourceMode>("zip");
  const [zipFile, setZipFile] = React.useState<File | null>(null);
  const [folderFiles, setFolderFiles] = React.useState<File[]>([]);
  const [folderName, setFolderName] = React.useState("");
  const [progress, setProgress] = React.useState(0);
  const [zipping, setZipping] = React.useState(false);
  const [detected, setDetected] = React.useState<Detected | null>(null);
  const [detecting, setDetecting] = React.useState(false);

  const zipInputRef = React.useRef<HTMLInputElement>(null);
  const folderInputRef = React.useRef<HTMLInputElement>(null);

  // เอาผล detection มา prefill ฟอร์ม (user แก้ต่อได้) — ชื่อเซตเฉพาะตอนช่องยังว่าง
  const applyDetected = React.useCallback(
    (d: Detected) => {
      setDetected(d);
      if (d.name && metaName.trim() === "") setName(d.name);
      if (d.serverType) setServerType(d.serverType);
      if (d.mcVersion) setMcVersion(d.mcVersion);
    },
    [metaName, setName, setServerType, setMcVersion],
  );

  const onZipChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] ?? null;
    if (file && !file.name.toLowerCase().endsWith(".zip")) {
      toast.error(t("import.notZip"));
      setZipFile(null);
      e.target.value = "";
      return;
    }
    setZipFile(file);
    setDetected(null);
    if (file) {
      setDetecting(true);
      try {
        applyDetected(await detectFromZip(file));
      } finally {
        setDetecting(false);
      }
    }
  };

  const onFolderChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const list = e.target.files;
    if (!list || list.length === 0) {
      setFolderFiles([]);
      setFolderName("");
      setDetected(null);
      return;
    }
    const files = Array.from(list);
    const first = files[0]?.webkitRelativePath ?? "";
    const top = first.includes("/") ? first.slice(0, first.indexOf("/")) : "";
    const name = top || t("import.folder");
    setFolderName(name);
    setFolderFiles(files);
    setDetected(null);
    setDetecting(true);
    try {
      applyDetected(await detectFromFolder(files, top));
    } finally {
      setDetecting(false);
    }
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
              {detecting && (
                <p className="text-muted-foreground flex items-center gap-1.5 text-xs">
                  <Loader2Icon className="size-3.5 animate-spin" />
                  {t("wizard.detecting")}
                </p>
              )}
              {!detecting &&
                detected &&
                (detected.serverType || detected.mcVersion) && (
                  <p className="text-muted-foreground text-xs">
                    {t("wizard.detectedHint", {
                      type: detected.serverType ?? "—",
                      version: detected.mcVersion ?? "—",
                    })}
                  </p>
                )}
            </div>

            {meta.fields}

            <Button
              type="submit"
              className="justify-self-start"
              disabled={!valid || busy}
            >
              {busy ? t("import.uploading") : t("wizard.importContinue")}
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

// ---------- Step 1 (after create): read-only summary ----------

function GeneralSummary({
  server,
  mode,
  jobStatus,
}: {
  server: Server;
  mode: "new" | "import";
  jobStatus: Job["status"] | null;
}) {
  const t = useT();
  const provisioning = mode === "new" && jobStatus !== "succeeded";
  const provisionFailed = jobStatus === "failed";

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("wizard.summary")}</CardTitle>
        <CardDescription>
          {mode === "import" ? t("wizard.imported") : t("wizard.created")}
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
            {!provisionFailed && <Loader2Icon className="size-4 animate-spin" />}
            {provisionFailed
              ? t("wizard.provisionFailed")
              : t("wizard.provisioning")}
          </div>
        )}

        <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
          <div className="flex items-center justify-between gap-2 border-b py-1">
            <dt className="text-muted-foreground">{t("wizard.summaryName")}</dt>
            <dd className="truncate font-medium">{server.name}</dd>
          </div>
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
      </CardContent>
    </Card>
  );
}

// การ์ด provisioning สำหรับ step ที่ต้องรอไฟล์บนโหนดพร้อมก่อน (properties/players)
function ProvisioningCard() {
  const t = useT();
  return (
    <Card className="py-12">
      <CardContent className="text-muted-foreground flex flex-col items-center gap-3 text-center text-sm">
        <Loader2Icon className="size-6 animate-spin" />
        <p>{t("wizard.provisioning")}</p>
      </CardContent>
    </Card>
  );
}

// ตัวบอกลำดับ step — horizontal บน desktop (โชว์ชื่อ), compact บน mobile (โชว์เลข)
function StepIndicator({ current }: { current: number }) {
  const t = useT();
  return (
    <ol className="flex items-center">
      {WIZARD_STEPS.map((s, i) => {
        const done = i < current;
        const active = i === current;
        return (
          <React.Fragment key={s.key}>
            <li className="flex shrink-0 items-center gap-2">
              <span
                className={cn(
                  "flex size-7 shrink-0 items-center justify-center rounded-full border text-xs font-medium transition-colors",
                  done && "border-primary bg-primary text-primary-foreground",
                  active && "border-primary text-primary",
                  !done && !active && "text-muted-foreground",
                )}
              >
                {done ? <CheckIcon className="size-4" /> : i + 1}
              </span>
              <span
                className={cn(
                  "hidden text-sm font-medium sm:inline",
                  active ? "text-foreground" : "text-muted-foreground",
                )}
              >
                {t(s.titleKey)}
              </span>
            </li>
            {i < WIZARD_STEPS.length - 1 && (
              <span
                className={cn(
                  "mx-2 h-px flex-1 sm:mx-3",
                  done ? "bg-primary" : "bg-border",
                )}
              />
            )}
          </React.Fragment>
        );
      })}
    </ol>
  );
}

function NewServerWizard() {
  const t = useT();
  const router = useRouter();
  const queryClient = useQueryClient();
  const searchParams = useSearchParams();
  const setDashboardServerId = useSettingsStore((st) => st.setDashboardServerId);
  const mode = searchParams.get("mode") === "import" ? "import" : "new";

  const [createdId, setCreatedId] = React.useState<string | null>(null);
  const [jobId, setJobId] = React.useState<string | null>(null);
  const [ready, setReady] = React.useState(false);
  const [step, setStep] = React.useState(0);

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
  // จึงต้อง refetch เพื่อโชว์เวอร์ชันที่ตรวจพบ + ให้ properties/players step เห็นไฟล์จริง
  const serverQuery = useQuery({
    queryKey: ["servers", createdId],
    queryFn: () =>
      apiGet(`/api/servers/${createdId}`, serverDetailResponseSchema),
    enabled: createdId !== null,
  });
  const server = serverQuery.data?.server;

  // new mode: poll create_server job จนเสร็จ แล้วปลดล็อก step ที่อ่านไฟล์ (properties/players)
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
      setStep(1);
      queryClient.invalidateQueries({ queryKey: ["servers"] });
    },
    [queryClient],
  );

  const onImported = React.useCallback(
    (s: Server) => {
      setCreatedId(s.id);
      setReady(true);
      setStep(1);
      queryClient.invalidateQueries({ queryKey: ["servers"] });
    },
    [queryClient],
  );

  // ไม่มีหน้า detail ต่อ server แล้ว — จบ wizard = ตั้งตัวที่เพิ่งสร้างเป็น active
  // แล้วไป dashboard (เมนู console/files/… ทำงานกับ active server ตัวนี้ต่อ)
  const goToServer = React.useCallback(() => {
    if (!createdId) return;
    setDashboardServerId(createdId);
    router.push("/dashboard");
  }, [createdId, router, setDashboardServerId]);

  // step ที่อ่านไฟล์จริงบนโหนด (properties/players) เปิดได้เมื่อไฟล์พร้อม (ready)
  const filesReady = created && ready;

  const stepContent = () => {
    // step 1: general — ก่อนสร้างคือฟอร์ม, หลังสร้างเป็น summary อ่านอย่างเดียว
    if (step === 0) {
      if (!created) {
        return mode === "import" ? (
          <ImportServerForm onImported={onImported} />
        ) : (
          <NewServerForm onCreated={onCreated} />
        );
      }
      return server ? (
        <GeneralSummary server={server} mode={mode} jobStatus={jobStatus} />
      ) : (
        <Skeleton className="h-64 w-full" />
      );
    }

    if (!server) return <Skeleton className="h-64 w-full" />;

    // step 2: server.properties (ต้องรอไฟล์พร้อม)
    if (step === 1) {
      return filesReady ? (
        <ServerPropertiesCard serverId={server.id} />
      ) : (
        <ProvisioningCard />
      );
    }
    // step 3: access (DB อย่างเดียว — พร้อมทันทีหลังสร้าง)
    if (step === 2) {
      return <ServerAccess serverId={server.id} />;
    }
    // step 4: players/whitelist (ต้องรอไฟล์พร้อม)
    // เพิ่งสร้างเสร็จ ยังไม่ได้ start — ปุ่ม action ที่สั่งผ่าน console จึงยังกดไม่ได้
    return filesReady ? (
      <ServerPlayers
        serverId={server.id}
        isRunning={server.status === "running"}
        onlineNames={server.stats?.online_players ?? []}
        canManage
        canModerate
      />
    ) : (
      <ProvisioningCard />
    );
  };

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

      <StepIndicator current={step} />

      <div>{stepContent()}</div>

      {/* nav controls โผล่หลังสร้างเซิร์ฟเวอร์ — ก่อนหน้านั้นปุ่ม create/import ในฟอร์มเป็นตัวเดินหน้า */}
      {created && (
        <div className="flex items-center justify-between gap-2">
          <Button
            variant="outline"
            disabled={step === 0}
            onClick={() => setStep((s) => Math.max(0, s - 1))}
          >
            {t("common.back")}
          </Button>
          {step < WIZARD_STEPS.length - 1 ? (
            <Button onClick={() => setStep((s) => s + 1)}>
              {t("wizard.next")}
            </Button>
          ) : (
            <Button onClick={goToServer}>{t("wizard.done")}</Button>
          )}
        </div>
      )}
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
