"use client";

import * as React from "react";
import { toast } from "sonner";
import { useT } from "@/lib/i18n";
import {
  detectFromFolder,
  detectFromZip,
  zipFolder,
  type Detected,
} from "@/components/server/new-server/detect";

export type SourceMode = "zip" | "folder";

export interface ImportSource {
  mode: SourceMode;
  setMode: (m: SourceMode) => void;
  zipFile: File | null;
  folderFiles: File[];
  folderName: string;
  detected: Detected | null;
  detecting: boolean;
  zipping: boolean;
  hasFile: boolean;
  onZipChange: (e: React.ChangeEvent<HTMLInputElement>) => Promise<void>;
  onFolderChange: (e: React.ChangeEvent<HTMLInputElement>) => Promise<void>;
  buildArchive: () => Promise<{ blob: Blob; filename: string }>;
}

// ไฟล์ต้นทางของโหมด import + การ detect type/version ที่ prefill ฟอร์ม metadata ให้
// (state อยู่ที่ wizard ไฟล์ที่เลือกจึงไม่หายตอนเดินหน้า/ถอยหลัง step)
export function useImportSource(meta: {
  name: string;
  setName: (v: string) => void;
  setServerType: (v: string) => void;
  setMcVersion: (v: string) => void;
}): ImportSource {
  const t = useT();
  const { setName, setServerType, setMcVersion } = meta;
  const metaName = meta.name;

  const [mode, setMode] = React.useState<SourceMode>("zip");
  const [zipFile, setZipFile] = React.useState<File | null>(null);
  const [folderFiles, setFolderFiles] = React.useState<File[]>([]);
  const [folderName, setFolderName] = React.useState("");
  const [zipping, setZipping] = React.useState(false);
  const [detected, setDetected] = React.useState<Detected | null>(null);
  const [detecting, setDetecting] = React.useState(false);

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
    setFolderName(top || t("import.folder"));
    setFolderFiles(files);
    setDetected(null);
    setDetecting(true);
    try {
      applyDetected(await detectFromFolder(files, top));
    } finally {
      setDetecting(false);
    }
  };

  const buildArchive = async () => {
    if (mode === "zip") {
      const file = zipFile as File;
      return { blob: file as Blob, filename: file.name };
    }
    setZipping(true);
    try {
      return { blob: await zipFolder(folderFiles), filename: "import.zip" };
    } finally {
      setZipping(false);
    }
  };

  return {
    mode,
    setMode,
    zipFile,
    folderFiles,
    folderName,
    detected,
    detecting,
    zipping,
    hasFile: mode === "zip" ? zipFile !== null : folderFiles.length > 0,
    onZipChange,
    onFolderChange,
    buildArchive,
  };
}
