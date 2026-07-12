"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2Icon } from "lucide-react";
import { apiGet, apiSend, getNextPort, ApiError } from "@/lib/api";
import { MemoryPresets } from "@/components/server/memory-presets";
import {
  createServerResponseSchema,
  jobResponseSchema,
  metaNodesResponseSchema,
  metaServerTypesResponseSchema,
  versionsResponseSchema,
  type Job,
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
  DialogFooter,
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

const POLL_INTERVAL_MS = 1_500;

function CreationProgress({
  server,
  job,
  onBack,
  onDone,
}: {
  server: Server;
  job: Job;
  onBack: () => void;
  onDone: (server: Server) => void;
}) {
  const t = useT();
  const poll = useQuery({
    queryKey: ["jobs", job.id],
    queryFn: () => apiGet(`/api/jobs/${job.id}`, jobResponseSchema),
    refetchInterval: (query) => {
      const status = query.state.data?.job.status;
      return status === "succeeded" || status === "failed"
        ? false
        : POLL_INTERVAL_MS;
    },
  });

  const status = poll.data?.job.status ?? job.status;
  const jobError = poll.data?.job.error ?? "";

  React.useEffect(() => {
    if (status === "succeeded") {
      toast.success(t("new.created", { name: server.name }));
      onDone(server);
    }
  }, [status, server, onDone, t]);

  return (
    <div className="grid gap-4">
      <DialogHeader>
        <DialogTitle>{t("new.provisioning", { name: server.name })}</DialogTitle>
        <DialogDescription>{t("new.provisioningDesc")}</DialogDescription>
      </DialogHeader>
      {status === "failed" ? (
        <>
          <p className="text-destructive text-sm">
            {t("new.failedProvision")}
            {jobError ? `: ${jobError}` : "."}
          </p>
          <DialogFooter>
            <Button variant="outline" onClick={onBack}>
              {t("new.backToForm")}
            </Button>
            <Button onClick={() => onDone(server)}>{t("new.viewServer")}</Button>
          </DialogFooter>
        </>
      ) : (
        <div className="text-muted-foreground flex items-center gap-2 text-sm">
          <Loader2Icon className="size-4 animate-spin" />
          {t("new.jobStatus", {
            status: t(`jobStatus.${status}` as TranslationKey),
          })}
        </div>
      )}
    </div>
  );
}

function NewServerForm({
  onCreated,
}: {
  onCreated: (created: { server: Server; job: Job }) => void;
}) {
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

  const create = useMutation({
    mutationFn: () =>
      apiSend(
        "POST",
        "/api/servers",
        {
          name: name.trim(),
          node_id: nodeId,
          server_type: serverType,
          mc_version: mcVersion,
          memory_mb: Number(memoryMb),
          host_port: hostPort === "" ? null : Number(hostPort),
          accept_eula: needsEula ? acceptEula : true,
        },
        createServerResponseSchema,
      ),
    onSuccess: (data) => onCreated(data),
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : t("new.failedCreate"));
    },
  });

  const memory = Number(memoryMb);
  const port = hostPort === "" ? null : Number(hostPort);
  const valid =
    name.trim().length > 0 &&
    nodeId !== "" &&
    serverType !== "" &&
    mcVersion !== "" &&
    Number.isInteger(memory) &&
    memory >= 512 &&
    (port === null || (Number.isInteger(port) && port >= 1024 && port <= 65535)) &&
    (!needsEula || acceptEula);

  const onSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (valid && !create.isPending) create.mutate();
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("new.title")}</DialogTitle>
        <DialogDescription>{t("new.subtitle")}</DialogDescription>
      </DialogHeader>
      <form onSubmit={onSubmit} className="grid gap-5">
        <div className="grid gap-2">
          <Label htmlFor="new-name">{t("new.name")}</Label>
          <Input
            id="new-name"
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
          <Label htmlFor="new-memory">{t("new.memory")}</Label>
          <Input
            id="new-memory"
            type="number"
            min={512}
            required
            value={memoryMb}
            onChange={(e) => setMemoryMb(e.target.value)}
          />
          <MemoryPresets value={memoryMb} onChange={setMemoryMb} />
        </div>

        <div className="grid gap-2">
          <Label htmlFor="new-port">{t("new.hostPort")}</Label>
          <Input
            id="new-port"
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

        {needsEula && (
          <div className="flex items-start gap-2">
            <Checkbox
              id="new-eula"
              checked={acceptEula}
              onCheckedChange={(v) => setAcceptEula(v === true)}
            />
            <Label htmlFor="new-eula" className="flex-wrap font-normal">
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

        <Button type="submit" disabled={!valid || create.isPending}>
          {create.isPending ? t("new.creating") : t("new.create")}
        </Button>
      </form>
    </>
  );
}

export function NewServerDialog() {
  const open = useUiStore((s) => s.newServerOpen);
  const setOpen = useUiStore((s) => s.setNewServerOpen);
  const router = useRouter();
  const queryClient = useQueryClient();
  const [created, setCreated] = React.useState<{
    server: Server;
    job: Job;
  } | null>(null);

  // reset ทุกครั้งที่ปิด modal เพื่อให้เปิดใหม่ได้ฟอร์มสะอาด
  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) setCreated(null);
  };

  const finish = React.useCallback(
    (server: Server) => {
      queryClient.invalidateQueries({ queryKey: ["servers"] });
      handleOpenChange(false);
      router.push(`/servers/${server.id}`);
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [queryClient, router],
  );

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        {created ? (
          <CreationProgress
            server={created.server}
            job={created.job}
            onBack={() => setCreated(null)}
            onDone={finish}
          />
        ) : (
          <NewServerForm onCreated={setCreated} />
        )}
      </DialogContent>
    </Dialog>
  );
}
