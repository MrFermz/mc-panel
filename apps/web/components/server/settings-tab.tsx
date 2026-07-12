"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { toast } from "sonner";
import {
  apiSend,
  getServerProperties,
  saveServerProperties,
  ApiError,
} from "@/lib/api";
import {
  jobResponseSchema,
  serverResponseSchema,
  type Server,
  type ServerPropertyField,
} from "@/lib/types";
import { useT } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { MemoryPresets } from "@/components/server/memory-presets";

export default function SettingsTab({ server }: { server: Server }) {
  const t = useT();
  const router = useRouter();
  const queryClient = useQueryClient();
  const [name, setName] = React.useState(server.name);
  const [memoryMb, setMemoryMb] = React.useState(String(server.memory_mb));
  const [hostPort, setHostPort] = React.useState(
    server.host_port === null ? "" : String(server.host_port),
  );
  const [deleteOpen, setDeleteOpen] = React.useState(false);

  const save = useMutation({
    mutationFn: () =>
      apiSend(
        "PATCH",
        `/api/servers/${server.id}`,
        {
          name: name.trim(),
          memory_mb: Number(memoryMb),
          // ส่ง 0 เมื่อล้างช่อง — server ตีความ 0 = เลิก expose host port (null = "ไม่เปลี่ยน")
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

  const remove = useMutation({
    mutationFn: () =>
      apiSend("DELETE", `/api/servers/${server.id}`, undefined, jobResponseSchema),
    onSuccess: () => {
      toast.success(t("sset.deleting", { name: server.name }));
      setDeleteOpen(false);
      queryClient.invalidateQueries({ queryKey: ["servers"] });
      router.push("/");
    },
    onError: (err) => {
      toast.error(err instanceof ApiError ? err.message : t("sset.failedDelete"));
    },
  });

  const memory = Number(memoryMb);
  const port = hostPort === "" ? null : Number(hostPort);
  const valid =
    name.trim().length > 0 &&
    Number.isInteger(memory) &&
    memory >= 512 &&
    (port === null || (Number.isInteger(port) && port >= 1024 && port <= 65535));

  return (
    <div className="grid items-start gap-6 lg:grid-cols-3">
      <div className="grid content-start gap-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("sset.general")}</CardTitle>
          <CardDescription>{t("sset.generalDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <form
            className="grid gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (valid && !save.isPending) save.mutate();
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="s-name">{t("sset.name")}</Label>
              <Input
                id="s-name"
                maxLength={100}
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="s-memory">{t("sset.memory")}</Label>
              <Input
                id="s-memory"
                type="number"
                min={512}
                value={memoryMb}
                onChange={(e) => setMemoryMb(e.target.value)}
              />
              <MemoryPresets value={memoryMb} onChange={setMemoryMb} />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="s-port">{t("sset.hostPort")}</Label>
              <Input
                id="s-port"
                type="number"
                min={1024}
                max={65535}
                placeholder={t("sset.hostPortPlaceholder")}
                value={hostPort}
                onChange={(e) => setHostPort(e.target.value)}
              />
            </div>
            <Button type="submit" disabled={!valid || save.isPending}>
              {save.isPending ? t("common.saving") : t("sset.saveChanges")}
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card className="border-destructive/40">
        <CardHeader>
          <CardTitle className="text-destructive">{t("sset.dangerZone")}</CardTitle>
          <CardDescription>{t("sset.dangerDesc")}</CardDescription>
        </CardHeader>
        <CardContent>
          <Button variant="destructive" onClick={() => setDeleteOpen(true)}>
            {t("sset.deleteServer")}
          </Button>
        </CardContent>
      </Card>
      </div>

      <div className="lg:col-span-2">
        <ServerPropertiesCard serverId={server.id} />
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t("sset.deleteTitle", { name: server.name })}
        description={t("sset.deleteDesc")}
        confirmLabel={t("sset.deleteServer")}
        destructive
        requireText={server.name}
        pending={remove.isPending}
        onConfirm={() => remove.mutate()}
      />
    </div>
  );
}

function PropertyControl({
  field,
  value,
  onChange,
}: {
  field: ServerPropertyField;
  value: string;
  onChange: (value: string) => void;
}) {
  if (field.type === "enum") {
    return (
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id={`prop-${field.key}`}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {field.options.map((opt) => (
            <SelectItem key={opt} value={opt}>
              {opt}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }
  if (field.type === "bool") {
    return (
      <div className="flex h-9 items-center">
        <Switch
          id={`prop-${field.key}`}
          checked={value === "true"}
          onCheckedChange={(v) => onChange(v ? "true" : "false")}
        />
      </div>
    );
  }
  if (field.type === "int") {
    return (
      <Input
        id={`prop-${field.key}`}
        type="number"
        {...(field.min !== null ? { min: field.min } : {})}
        {...(field.max !== null ? { max: field.max } : {})}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  }
  return (
    <Input
      id={`prop-${field.key}`}
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}

function ServerPropertiesCard({ serverId }: { serverId: string }) {
  const t = useT();
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: ["servers", serverId, "properties"],
    queryFn: () => getServerProperties(serverId),
    retry: false,
    refetchOnWindowFocus: false,
  });

  const values = query.data?.values;
  const [draft, setDraft] = React.useState<Record<string, string>>({});
  // seed draft ตอน values โหลดมา (refetch ปิด window-focus ไว้แล้ว จึงไม่ทับ edit ค้าง)
  React.useEffect(() => {
    if (values) setDraft({ ...values });
  }, [values]);

  const fields = query.data?.fields ?? [];
  const extra = query.data?.extra ?? {};
  const extraKeys = Object.keys(extra);

  const dirty = React.useMemo(() => {
    if (!values) return false;
    return Object.keys(values).some((k) => draft[k] !== values[k]);
  }, [draft, values]);

  const save = useMutation({
    mutationFn: () => saveServerProperties(serverId, draft),
    onSuccess: () => {
      toast.success(t("sset.propsSaved"));
      queryClient.invalidateQueries({
        queryKey: ["servers", serverId, "properties"],
      });
    },
    onError: (err) => {
      if (err instanceof ApiError && err.code === "invalid_property") {
        toast.error(t("sset.propsInvalid"));
        return;
      }
      toast.error(err instanceof ApiError ? err.message : t("sset.failedSave"));
    },
  });

  const setField = (key: string, val: string) =>
    setDraft((prev) => ({ ...prev, [key]: val }));

  const offline =
    query.isError &&
    query.error instanceof ApiError &&
    query.error.code === "node_offline";

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("sset.propsTitle")}</CardTitle>
        <CardDescription>{t("sset.propsRestartHint")}</CardDescription>
      </CardHeader>
      <CardContent>
        {query.isPending ? (
          <Skeleton className="h-40 w-full" />
        ) : offline ? (
          <p className="text-muted-foreground text-sm">{t("sset.propsOffline")}</p>
        ) : query.isError ? (
          <p className="text-destructive text-sm">
            {query.error instanceof ApiError
              ? query.error.message
              : t("sset.failedSave")}
          </p>
        ) : (
          <div className="grid gap-4">
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              {fields.map((field) => (
                <div key={field.key} className="grid gap-2">
                  <Label htmlFor={`prop-${field.key}`} className="font-normal">
                    {field.label}
                  </Label>
                  <PropertyControl
                    field={field}
                    value={draft[field.key] ?? values?.[field.key] ?? ""}
                    onChange={(v) => setField(field.key, v)}
                  />
                </div>
              ))}
            </div>

            {extraKeys.length > 0 && (
              <Accordion type="single" collapsible>
                <AccordionItem value="extra">
                  <AccordionTrigger className="text-muted-foreground text-xs">
                    {t("sset.propsExtra")}
                  </AccordionTrigger>
                  <AccordionContent>
                    <p className="text-muted-foreground mb-2 text-xs">
                      {t("sset.propsExtraHint")}
                    </p>
                    <div className="grid gap-1">
                      {extraKeys.map((key) => (
                        <div
                          key={key}
                          className="text-muted-foreground font-mono text-xs break-all"
                        >
                          {key}={extra[key]}
                        </div>
                      ))}
                    </div>
                  </AccordionContent>
                </AccordionItem>
              </Accordion>
            )}

            <div>
              <Button
                type="button"
                disabled={!dirty || save.isPending}
                onClick={() => save.mutate()}
              >
                {save.isPending ? t("common.saving") : t("common.save")}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
