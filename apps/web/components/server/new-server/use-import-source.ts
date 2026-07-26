"use client";

import * as React from "react";
import { toast } from "sonner";
import { useT } from "@/lib/i18n";
import { zipFolder } from "@/components/server/new-server/archive";
import { gameProfile, type DetectedInstance } from "@/lib/games";

export type SourceMode = "zip" | "folder";

export interface ImportSource {
  mode: SourceMode;
  setMode: (m: SourceMode) => void;
  zipFile: File | null;
  folderFiles: File[];
  folderName: string;
  detected: DetectedInstance | null;
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
  game?: string;
  name: string;
  setName: (v: string) => void;
  setVariant: (v: string) => void;
  setGameVersion: (v: string) => void;
}): ImportSource {
  const t = useT();
  const { setName, setVariant, setGameVersion } = meta;
  // ความรู้เรื่อง "ไฟล์แบบไหนคือเกมอะไร" อยู่ใน game profile ตัวเดียว ไม่กระจายใน wizard
  const game = gameProfile(meta.game);
  const metaName = meta.name;

  const [mode, setMode] = React.useState<SourceMode>("zip");
  const [zipFile, setZipFile] = React.useState<File | null>(null);
  const [folderFiles, setFolderFiles] = React.useState<File[]>([]);
  const [folderName, setFolderName] = React.useState("");
  const [zipping, setZipping] = React.useState(false);
  const [detected, setDetected] = React.useState<DetectedInstance | null>(null);
  const [detecting, setDetecting] = React.useState(false);

  // เอาผล detection มา prefill ฟอร์ม (user แก้ต่อได้) — ชื่อเซตเฉพาะตอนช่องยังว่าง
  const applyDetected = React.useCallback(
    (d: DetectedInstance) => {
      setDetected(d);
      if (d.name && metaName.trim() === "") setName(d.name);
      if (d.variant) setVariant(d.variant);
      if (d.gameVersion) setGameVersion(d.gameVersion);
    },
    [metaName, setName, setVariant, setGameVersion],
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
        applyDetected(await game.detectFromZip(file));
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
      applyDetected(await game.detectFromFolder(files, top));
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
