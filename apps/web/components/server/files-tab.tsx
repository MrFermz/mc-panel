"use client";

import * as React from "react";
import dynamic from "next/dynamic";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ChevronRightIcon,
  CornerLeftUpIcon,
  FileIcon,
  FilePlusIcon,
  FolderIcon,
  FolderPlusIcon,
  HouseIcon,
  PencilIcon,
  RefreshCwIcon,
  Trash2Icon,
} from "lucide-react";
import {
  ApiError,
  deleteFile,
  listFiles,
  makeDir,
  readFileContent,
  renameFile,
  writeFileContent,
} from "@/lib/api";
import type { FileEntry } from "@/lib/types";
import { formatBytes, formatDateTime } from "@/lib/format";
import { useT, type TranslateFn, type TranslationKey } from "@/lib/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { cn } from "@/lib/utils";

// CodeMirror แตะ window/DOM ตั้งแต่ import — ปิด SSR กัน hydration mismatch (แบบเดียวกับ ConsoleTab)
const CodeEditor = dynamic(() => import("@/components/server/code-editor"), {
  ssr: false,
  loading: () => <Skeleton className="h-80 w-full" />,
});

// map error code จาก API → ข้อความ i18n ที่ผู้ใช้เข้าใจ (ตาม docs/api.md)
const FILE_ERROR_KEYS: Record<string, TranslationKey> = {
  forbidden: "files.errForbidden",
  file_not_found: "files.errNotFound",
  file_too_large: "files.errTooLarge",
  invalid_path: "files.errInvalidPath",
  node_offline: "files.errNodeOffline",
  agent_timeout: "files.errAgentTimeout",
  binary_file: "files.errBinary",
};

function fileErrorMessage(err: unknown, t: TranslateFn): string {
  if (err instanceof ApiError) {
    const key = FILE_ERROR_KEYS[err.code];
    if (key) return t(key);
    return err.message;
  }
  return t("files.errGeneric");
}

function joinPath(dir: string, name: string): string {
  return dir ? `${dir}/${name}` : name;
}

function parentPath(path: string): string {
  const idx = path.lastIndexOf("/");
  return idx <= 0 ? "" : path.slice(0, idx);
}

function baseName(path: string): string {
  const idx = path.lastIndexOf("/");
  return idx < 0 ? path : path.slice(idx + 1);
}

type NameMode = "newFolder" | "newFile" | "rename";

export default function FilesTab({ serverId }: { serverId: string }) {
  const t = useT();
  const queryClient = useQueryClient();

  const [path, setPath] = React.useState("");
  const [editing, setEditing] = React.useState<string | null>(null);
  const [nameDialog, setNameDialog] = React.useState<{
    mode: NameMode;
    target?: FileEntry;
  } | null>(null);
  const [nameValue, setNameValue] = React.useState("");
  const [deleteTarget, setDeleteTarget] = React.useState<FileEntry | null>(null);

  const list = useQuery({
    queryKey: ["servers", serverId, "files", path],
    queryFn: () => listFiles(serverId, path),
  });

  const invalidateList = () =>
    queryClient.invalidateQueries({
      queryKey: ["servers", serverId, "files", path],
    });

  const nameMutation = useMutation({
    mutationFn: async (vars: { mode: NameMode; target?: FileEntry; value: string }) => {
      const name = vars.value.trim();
      if (vars.mode === "rename" && vars.target) {
        await renameFile(
          serverId,
          joinPath(path, vars.target.name),
          joinPath(path, name),
        );
      } else if (vars.mode === "newFolder") {
        await makeDir(serverId, joinPath(path, name));
      } else {
        // สร้างไฟล์ว่างด้วยการเขียน content ว่าง
        await writeFileContent(serverId, joinPath(path, name), "");
      }
      return vars.mode;
    },
    onSuccess: (mode) => {
      toast.success(
        mode === "rename"
          ? t("files.renamed")
          : mode === "newFolder"
            ? t("files.folderCreated")
            : t("files.fileCreated"),
      );
      setNameDialog(null);
      invalidateList();
    },
    onError: (err) => toast.error(fileErrorMessage(err, t)),
  });

  const removeMutation = useMutation({
    mutationFn: (entry: FileEntry) =>
      deleteFile(serverId, joinPath(path, entry.name)),
    onSuccess: () => {
      toast.success(t("files.deleted"));
      setDeleteTarget(null);
      invalidateList();
    },
    onError: (err) => toast.error(fileErrorMessage(err, t)),
  });

  const openNameDialog = (mode: NameMode, target?: FileEntry) => {
    setNameDialog({ mode, target });
    setNameValue(target?.name ?? "");
  };

  const openEntry = (entry: FileEntry) => {
    if (entry.is_dir) {
      setPath(joinPath(path, entry.name));
    } else {
      setEditing(joinPath(path, entry.name));
    }
  };

  // breadcrumb: [root, seg1, seg1/seg2, ...]
  const segments = path ? path.split("/") : [];
  const crumbs = segments.map((seg, i) => ({
    label: seg,
    target: segments.slice(0, i + 1).join("/"),
  }));

  const nameDialogTitleKey: TranslationKey =
    nameDialog?.mode === "rename"
      ? "files.renameTitle"
      : nameDialog?.mode === "newFolder"
        ? "files.newFolderTitle"
        : "files.newFileTitle";

  return (
    <div className="grid gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="text-muted-foreground flex min-w-0 items-center gap-1 text-sm">
          <button
            type="button"
            onClick={() => setPath("")}
            className="hover:text-foreground flex items-center gap-1"
            aria-label={t("files.rootLabel")}
          >
            <HouseIcon className="size-3.5" />
          </button>
          {crumbs.map((c) => (
            <React.Fragment key={c.target}>
              <ChevronRightIcon className="size-3.5 shrink-0 opacity-50" />
              <button
                type="button"
                onClick={() => setPath(c.target)}
                className="hover:text-foreground max-w-40 truncate"
              >
                {c.label}
              </button>
            </React.Fragment>
          ))}
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={path === ""}
            onClick={() => setPath(parentPath(path))}
          >
            <CornerLeftUpIcon />
            {t("files.up")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => list.refetch()}
            aria-label={t("files.refresh")}
          >
            <RefreshCwIcon className={cn(list.isFetching && "animate-spin")} />
          </Button>
          <Button size="sm" variant="secondary" onClick={() => openNameDialog("newFolder")}>
            <FolderPlusIcon />
            {t("files.newFolder")}
          </Button>
          <Button size="sm" onClick={() => openNameDialog("newFile")}>
            <FilePlusIcon />
            {t("files.newFile")}
          </Button>
        </div>
      </div>

      {list.isPending ? (
        <Skeleton className="h-48 w-full" />
      ) : list.isError ? (
        <p className="text-destructive text-sm">
          {fileErrorMessage(list.error, t)}
        </p>
      ) : list.data.entries.length === 0 ? (
        <p className="text-muted-foreground text-sm">{t("files.empty")}</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("files.name")}</TableHead>
              <TableHead className="w-28">{t("files.size")}</TableHead>
              <TableHead className="w-48">{t("files.modified")}</TableHead>
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {list.data.entries.map((entry) => (
              <TableRow key={entry.name}>
                <TableCell>
                  <button
                    type="button"
                    onClick={() => openEntry(entry)}
                    className="hover:text-foreground flex items-center gap-2 text-left"
                  >
                    {entry.is_dir ? (
                      <FolderIcon className="size-4 shrink-0 text-sky-500" />
                    ) : (
                      <FileIcon className="text-muted-foreground size-4 shrink-0" />
                    )}
                    <span className="truncate">{entry.name}</span>
                  </button>
                </TableCell>
                <TableCell className="text-muted-foreground text-xs">
                  {entry.is_dir ? "-" : formatBytes(entry.size)}
                </TableCell>
                <TableCell className="text-muted-foreground text-xs">
                  {formatDateTime(entry.mod_time)}
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={() => openNameDialog("rename", entry)}
                      aria-label={`${t("files.rename")} ${entry.name}`}
                    >
                      <PencilIcon />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="text-destructive"
                      onClick={() => setDeleteTarget(entry)}
                      aria-label={`${t("common.delete")} ${entry.name}`}
                    >
                      <Trash2Icon />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {editing !== null && (
        <FileEditorDialog
          serverId={serverId}
          path={editing}
          onClose={() => setEditing(null)}
        />
      )}

      <Dialog
        open={nameDialog !== null}
        onOpenChange={(open) => {
          if (!open) setNameDialog(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t(nameDialogTitleKey)}</DialogTitle>
          </DialogHeader>
          <form
            className="grid gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (!nameDialog || nameMutation.isPending) return;
              if (!nameValue.trim()) return;
              nameMutation.mutate({
                mode: nameDialog.mode,
                target: nameDialog.target,
                value: nameValue,
              });
            }}
          >
            <div className="grid gap-2">
              <Label htmlFor="file-name">{t("files.nameLabel")}</Label>
              <Input
                id="file-name"
                autoFocus
                value={nameValue}
                onChange={(e) => setNameValue(e.target.value)}
              />
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setNameDialog(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={nameMutation.isPending || !nameValue.trim()}
              >
                {nameMutation.isPending
                  ? t("common.saving")
                  : nameDialog?.mode === "rename"
                    ? t("common.save")
                    : t("files.createBtn")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        title={t("files.deleteTitle")}
        description={
          deleteTarget
            ? t("files.deleteDesc", { name: deleteTarget.name })
            : ""
        }
        confirmLabel={t("common.delete")}
        destructive
        pending={removeMutation.isPending}
        onConfirm={() => {
          if (deleteTarget) removeMutation.mutate(deleteTarget);
        }}
      />
    </div>
  );
}

function FileEditorDialog({
  serverId,
  path,
  onClose,
}: {
  serverId: string;
  path: string;
  onClose: () => void;
}) {
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = React.useState<string | null>(null);

  const content = useQuery({
    queryKey: ["servers", serverId, "file-content", path],
    queryFn: () => readFileContent(serverId, path),
    retry: false,
    refetchOnWindowFocus: false,
  });

  const truncated = content.data?.truncated ?? false;
  const value = draft ?? content.data?.content ?? "";
  const dirty = draft !== null && draft !== (content.data?.content ?? "");

  const save = useMutation({
    mutationFn: () => writeFileContent(serverId, path, value),
    onSuccess: () => {
      toast.success(t("files.saved"));
      setDraft(null);
      queryClient.invalidateQueries({
        queryKey: ["servers", serverId, "file-content", path],
      });
      onClose();
    },
    onError: (err) => toast.error(fileErrorMessage(err, t)),
  });

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="truncate">{baseName(path)}</DialogTitle>
          <DialogDescription className="truncate font-mono">
            {path}
          </DialogDescription>
        </DialogHeader>

        {content.isPending ? (
          <Skeleton className="h-80 w-full" />
        ) : content.isError ? (
          <p className="text-destructive text-sm">
            {fileErrorMessage(content.error, t)}
          </p>
        ) : (
          <div className="grid gap-2">
            {truncated && (
              <p className="text-xs text-amber-500">{t("files.truncated")}</p>
            )}
            <CodeEditor
              value={value}
              onChange={(next) => setDraft(next)}
              readOnly={truncated}
              filename={baseName(path)}
            />
          </div>
        )}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.close")}
          </Button>
          <Button
            type="button"
            disabled={
              content.isError || truncated || save.isPending || !dirty
            }
            onClick={() => save.mutate()}
          >
            {save.isPending ? t("common.saving") : t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
