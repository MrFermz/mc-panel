"use client";

import * as React from "react";
import { useT } from "@/lib/i18n";
import { useMe } from "@/lib/use-me";
import type { Server } from "@/lib/types";
import { LoadingOverlay } from "@/components/loading-overlay";
import { StepGeneral } from "@/components/server/new-server/step-general";
import { useServerMetadata } from "@/components/server/new-server/use-server-metadata";
import { useImportSource } from "@/components/server/new-server/use-import-source";
import { useCreateServer } from "@/components/server/new-server/use-create-server";
import { Button } from "@/components/ui/button";

// โหมด import เป็นฟอร์มหน้าเดียว ไม่ผ่าน stepper ของ wizard — ไฟล์ต้นทาง + metadata
// แล้วยิง POST /api/servers/import ทันที (properties/access/players ตั้งต่อที่หน้าของ server
// หลังนำเข้าเสร็จ) เพื่อให้เส้นทาง import มีชิ้นส่วนน้อยที่สุดเวลาไล่หาสาเหตุที่นำเข้าไม่สำเร็จ
export function ImportServerPage({
  onImported,
}: {
  onImported: (server: Server) => void;
}) {
  const t = useT();
  const me = useMe().data?.user;
  const meta = useServerMetadata();
  const importSource = useImportSource(meta);

  const create = useCreateServer({
    mode: "import",
    meta,
    importSource,
    // ขั้นตอนหลัง create ทั้งหมดถูกตัดออกจากเส้นทางนี้โดยตั้งใจ
    changedProps: {},
    accessDraft: [],
    playersDraft: [],
    selfUserId: me?.id,
    onCreated: onImported,
  });

  const valid = meta.valid && importSource.hasFile;

  return (
    <>
      <form
        className="grid gap-6"
        onSubmit={(e) => {
          e.preventDefault();
          if (valid && !create.pending) create.run();
        }}
      >
        <StepGeneral mode="import" meta={meta} importSource={importSource} />
        <Button
          type="submit"
          className="justify-self-start"
          loading={create.pending}
          disabled={!valid}
        >
          {t("import.import")}
        </Button>
      </form>

      {create.pending && (
        <LoadingOverlay
          title={
            importSource.zipping
              ? t("import.zipping")
              : create.phaseKey
                ? t(create.phaseKey)
                : t("wizard.overlayTitle")
          }
          description={t("wizard.overlayHint")}
          progress={create.uploadPct}
        />
      )}
    </>
  );
}
