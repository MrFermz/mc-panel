"use client";

import * as React from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { type Permission, type Server } from "@/lib/types";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { useSettingsStore } from "@/lib/settings/store";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { LoadingOverlay } from "@/components/loading-overlay";
import { PageLoader } from "@/components/page-loader";
import { LAST_STEP } from "@/components/server/new-server/steps";
import { StepIndicator } from "@/components/server/new-server/step-indicator";
import { StepGeneral } from "@/components/server/new-server/step-general";
import {
  StepProperties,
  changedFrom,
  useCatalogDefaults,
} from "@/components/server/new-server/step-properties";
import { StepAccess } from "@/components/server/new-server/step-access";
import { StepPlayers } from "@/components/server/new-server/step-players";
import { useServerMetadata } from "@/components/server/new-server/use-server-metadata";
import { useImportSource } from "@/components/server/new-server/use-import-source";
import { useCreateServer } from "@/components/server/new-server/use-create-server";
import { Button } from "@/components/ui/button";

function NewServerWizard() {
  const t = useT();
  const me = useMe().data?.user;
  const router = useRouter();
  const searchParams = useSearchParams();
  const setDashboardServerId = useSettingsStore((st) => st.setDashboardServerId);
  const mode = searchParams.get("mode") === "import" ? "import" : "new";

  const [step, setStep] = React.useState(0);
  // draft ทั้งหมดอยู่ในหน้าเว็บจนกว่าจะกด create ที่ step สุดท้าย
  const [propsDraft, setPropsDraft] = React.useState<Record<string, string>>({});
  const [accessDraft, setAccessDraft] = React.useState<Permission[]>([]);
  const [playersDraft, setPlayersDraft] = React.useState<string[]>([]);

  const meta = useServerMetadata();
  const importSource = useImportSource(meta);
  // เรียกที่นี่ด้วยเพื่อคำนวณ changedProps ตอนกด create แม้ user ไม่เคยเปิด step 2
  // (react-query dedupe ด้วย key เดียวกันกับที่ StepProperties ใช้ — ไม่ได้ยิงซ้ำ)
  const { defaults } = useCatalogDefaults();

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

  // คนสร้างเป็น owner เสมอ — backend ทำให้เองที่ CreateServerWithOwner จึงโชว์ไว้ในลิสต์
  // ตั้งแต่แรกให้เห็นว่ามีอยู่แล้ว (แถวนี้ล็อกไว้ และตอน apply จะถูกข้าม ไม่ POST ซ้ำ)
  // อยู่ที่หน้า ไม่ใช่ใน StepAccess เพราะต้องมีค่าแม้ user ข้าม step 3 ไปเลย
  React.useEffect(() => {
    if (!me) return;
    setAccessDraft((prev) =>
      prev.some((p) => p.user_id === me.id)
        ? prev
        : [
            {
              user_id: me.id,
              username: me.username,
              display_name: me.display_name,
              avatar_url: me.avatar_url,
              role: "owner",
              capabilities: [],
            },
            ...prev,
          ],
    );
  }, [me]);

  const onCreated = React.useCallback(
    (server: Server) => {
      // ไม่มีหน้า detail ต่อ server — ตั้งตัวที่เพิ่งสร้างเป็น active แล้วไป dashboard
      setDashboardServerId(server.id);
      router.push("/dashboard");
    },
    [router, setDashboardServerId],
  );

  const create = useCreateServer({
    mode,
    meta,
    importSource,
    changedProps: changedFrom(defaults, propsDraft),
    accessDraft,
    playersDraft,
    selfUserId: me?.id,
    onCreated,
  });

  const setProp = React.useCallback((key: string, value: string) => {
    setPropsDraft((prev) => ({ ...prev, [key]: value }));
  }, []);

  // step 1 เป็นด่านเดียวที่บังคับกรอก — ที่เหลือข้ามได้หมด
  const canLeaveFirstStep =
    meta.valid && (mode !== "import" || importSource.hasFile);

  const stepContent = () => {
    if (step === 0) {
      return (
        <StepGeneral mode={mode} meta={meta} importSource={importSource} />
      );
    }
    if (step === 1) {
      return <StepProperties draft={propsDraft} onChange={setProp} />;
    }
    if (step === 2) {
      return (
        <StepAccess
          draft={accessDraft}
          onChange={setAccessDraft}
          selfUserId={me?.id}
        />
      );
    }
    return (
      <StepPlayers
        value={playersDraft}
        onChange={setPlayersDraft}
        whitelistEnabled={
          (propsDraft["white-list"] ?? defaults["white-list"]) === "true"
        }
        onEnableWhitelist={() => setProp("white-list", "true")}
      />
    );
  };

  // กันเข้าตรง URL — ปุ่ม/เมนูที่พามาที่นี่ซ่อนตาม servers.create อยู่แล้ว แต่หน้านี้เข้าถึงได้
  // เองด้วย ถ้าไม่กันซ้ำ user จะกรอก wizard ทั้งชุดแล้วไปเจอ 403 ตอนกดสร้าง
  // (เช็คหลัง hook ทั้งหมดเสมอ — early return ก่อน hook ทำ order พัง)
  if (me && !hasCapability(me, CAPABILITY.serversCreate)) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>
    );
  }

  return (
    <>
      {/* pb เผื่อความสูงของ footer ที่ตรึงอยู่ ไม่ให้ทับเนื้อหาบรรทัดสุดท้าย */}
      <div className="grid gap-6 pb-24">
        <div className="grid gap-1">
          <h1 className="text-xl font-semibold">
            {mode === "import" ? t("import.title") : t("new.title")}
          </h1>
          <p className="text-muted-foreground text-sm">
            {mode === "import" ? t("import.subtitle") : t("new.subtitle")}
          </p>
        </div>

        {/* stepper ตรึงใต้ top bar ของ layout (h-14) — ยืดออกนอก padding ของ <main>
            ด้วย -mx เพื่อให้พื้นหลังคลุมเต็มความกว้าง ไม่งั้นเนื้อหาจะลอดข้างตอนเลื่อน */}
        <div className="bg-background/95 sticky top-14 z-30 -mx-4 border-b px-4 py-3 backdrop-blur md:-mx-6 md:px-6">
          <StepIndicator current={step} onSelect={setStep} />
        </div>

        <div>{stepContent()}</div>

        {step > 0 && step < LAST_STEP && (
          <p className="text-muted-foreground -mt-3 text-xs">
            {t("wizard.optionalStep")}
          </p>
        )}
      </div>

      {/* ปุ่มเดินหน้า/ถอยหลัง/สร้าง ตรึงล่างจอเสมอ — ความกว้างในสุดตรงกับ top bar */}
      <div className="bg-background/95 fixed inset-x-0 bottom-0 z-40 border-t backdrop-blur">
        <div className="mx-auto flex w-full max-w-6xl items-center justify-between gap-2 px-4 py-3 md:px-6">
          <Button
            variant="outline"
            disabled={step === 0 || create.pending}
            onClick={() => setStep((s) => Math.max(0, s - 1))}
          >
            {t("common.back")}
          </Button>
          {step < LAST_STEP ? (
            <Button
              disabled={step === 0 && !canLeaveFirstStep}
              onClick={() => setStep((s) => s + 1)}
            >
              {t("wizard.next")}
            </Button>
          ) : (
            <Button
              loading={create.pending}
              disabled={!canLeaveFirstStep}
              onClick={create.run}
            >
              {mode === "import" ? t("wizard.importNow") : t("wizard.createNow")}
            </Button>
          )}
        </div>
      </div>

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

// useSearchParams ต้องอยู่ใน Suspense boundary ไม่งั้น build บ่น CSR bailout
export default function NewServerPage() {
  return (
    <React.Suspense fallback={<PageLoader />}>
      <NewServerWizard />
    </React.Suspense>
  );
}
