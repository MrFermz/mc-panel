"use client";

import * as React from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeftIcon } from "lucide-react";
import { type Permission, type Server } from "@/lib/types";
import { CAPABILITY, hasCapability } from "@/lib/capabilities";
import { useMe } from "@/lib/use-me";
import { useT } from "@/lib/i18n";
import { useSettingsStore } from "@/lib/settings/store";
import { useSetBreadcrumbs } from "@/components/layout/breadcrumb-context";
import { LoadingOverlay } from "@/components/loading-overlay";
import { wizardSteps } from "@/components/server/new-server/steps";
import { StepIndicator } from "@/components/server/new-server/step-indicator";
import { StepGeneral } from "@/components/server/new-server/step-general";
import {
  StepProperties,
  changedFrom,
  useCatalogDefaults,
} from "@/components/server/new-server/step-properties";
import { StepAccess } from "@/components/server/new-server/step-access";
import { StepPlayers } from "@/components/server/new-server/step-players";
import { knownGameProfile, type GameProfile } from "@/lib/games";
import { useServerMetadata } from "@/components/server/new-server/use-server-metadata";
import { useMetaGames } from "@/components/server/new-server/use-meta-games";
import { useCreateServer } from "@/components/server/new-server/use-create-server";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

function NewServerWizard({
  profile,
  onCreated,
}: {
  profile: GameProfile;
  onCreated: (server: Server) => void;
}) {
  const t = useT();
  const me = useMe().data?.user;

  const [step, setStep] = React.useState(0);
  // draft ทั้งหมดอยู่ในหน้าเว็บจนกว่าจะกด create ที่ step สุดท้าย
  const [propsDraft, setPropsDraft] = React.useState<Record<string, string>>({});
  const [accessDraft, setAccessDraft] = React.useState<Permission[]>([]);
  const [playersDraft, setPlayersDraft] = React.useState<string[]>([]);

  // ลำดับ step เป็นของเกมนั้น ๆ (เกมที่ panel เขียนรายชื่อผู้เล่นไม่ได้ก็ไม่มี step ผู้เล่น)
  const steps = React.useMemo(() => wizardSteps(profile), [profile]);
  const lastStep = steps.length - 1;

  const meta = useServerMetadata(profile.id);
  // เรียกที่นี่ด้วยเพื่อคำนวณ changedProps ตอนกด create แม้ user ไม่เคยเปิด step properties
  // (react-query dedupe ด้วย key เดียวกันกับที่ StepProperties ใช้ — ไม่ได้ยิงซ้ำ)
  const { defaults } = useCatalogDefaults(profile.id);

  // คนสร้างเป็น owner เสมอ — backend ทำให้เองที่ CreateServerWithOwner จึงโชว์ไว้ในลิสต์
  // ตั้งแต่แรกให้เห็นว่ามีอยู่แล้ว (แถวนี้ล็อกไว้ และตอน apply จะถูกข้าม ไม่ POST ซ้ำ)
  // อยู่ที่หน้า ไม่ใช่ใน StepAccess เพราะต้องมีค่าแม้ user ข้าม step access ไปเลย
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

  const create = useCreateServer({
    meta,
    changedProps: changedFrom(defaults, propsDraft),
    accessDraft,
    playersDraft,
    selfUserId: me?.id,
    onCreated,
  });

  const setProp = React.useCallback((key: string, value: string) => {
    setPropsDraft((prev) => ({ ...prev, [key]: value }));
  }, []);

  // key ที่เปิด allowlist เป็นของเกมที่เลือกไว้ ไม่ใช่ค่าคงที่ของ wizard
  const allowlistKey = profile.allowlistEnabledKey;

  const stepContent = () => {
    switch (steps[step]?.key) {
      case "general":
        return <StepGeneral meta={meta} profile={profile} />;
      case "properties":
        return (
          <StepProperties
            game={profile.id}
            draft={propsDraft}
            onChange={setProp}
          />
        );
      case "access":
        return (
          <StepAccess
            draft={accessDraft}
            onChange={setAccessDraft}
            selfUserId={me?.id}
          />
        );
      case "players":
        return (
          <StepPlayers
            value={playersDraft}
            onChange={setPlayersDraft}
            game={profile.id}
            allowlistEnabled={
              allowlistKey === "" ||
              (propsDraft[allowlistKey] ?? defaults[allowlistKey]) === "true"
            }
            onEnableAllowlist={() => setProp(allowlistKey, "true")}
          />
        );
      default:
        return null;
    }
  };

  return (
    <>
      {/* pb เผื่อความสูงของ footer ที่ตรึงอยู่ ไม่ให้ทับเนื้อหาบรรทัดสุดท้าย */}
      <div className="grid gap-6 pb-24">
        <div className="grid gap-2">
          <div>
            <Button variant="ghost" size="sm" className="-ml-2" asChild>
              <Link href="/servers/new">
                <ArrowLeftIcon />
                {t("gamePicker.changeGame")}
              </Link>
            </Button>
          </div>
          <h1 className="text-xl font-semibold">
            {t("new.titleForGame", { game: profile.label })}
          </h1>
          <p className="text-muted-foreground text-sm">{t("new.subtitle")}</p>
        </div>

        {/* stepper ตรึงใต้ top bar ของ layout (h-14) — ยืดออกนอก padding ของ <main>
            ด้วย -mx เพื่อให้พื้นหลังคลุมเต็มความกว้าง ไม่งั้นเนื้อหาจะลอดข้างตอนเลื่อน */}
        <div className="bg-background/95 sticky top-14 z-30 -mx-4 border-b px-4 py-3 backdrop-blur md:-mx-6 md:px-6">
          <StepIndicator steps={steps} current={step} onSelect={setStep} />
        </div>

        <div>{stepContent()}</div>

        {step > 0 && step < lastStep && (
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
          {step < lastStep ? (
            <Button
              disabled={step === 0 && !meta.valid}
              onClick={() => setStep((s) => s + 1)}
            >
              {t("wizard.next")}
            </Button>
          ) : (
            <Button
              loading={create.pending}
              disabled={!meta.valid}
              onClick={create.run}
            >
              {t("wizard.createNow")}
            </Button>
          )}
        </div>
      </div>

      {create.pending && (
        <LoadingOverlay
          title={
            create.phaseKey ? t(create.phaseKey) : t("wizard.overlayTitle")
          }
          description={t("wizard.overlayHint")}
        />
      )}
    </>
  );
}

export default function NewServerPage() {
  const t = useT();
  const me = useMe().data?.user;
  const router = useRouter();
  const params = useParams<{ game: string }>();
  const gameId = params.game;
  const setDashboardServerId = useSettingsStore((st) => st.setDashboardServerId);
  const { games, pending } = useMetaGames();

  useSetBreadcrumbs(
    React.useMemo(() => [{ label: t("wizard.newBreadcrumb") }], [t]),
  );

  const onCreated = React.useCallback(
    (server: Server) => {
      // ไม่มีหน้า detail ต่อ server — ตั้งตัวที่เพิ่งสร้างเป็น active แล้วไป dashboard
      setDashboardServerId(server.id);
      router.push("/dashboard");
    },
    [router, setDashboardServerId],
  );

  // กันเข้าตรง URL — ทั้งสิทธิ์สร้าง (servers.create) และสิทธิ์ต่อเกม (games.{id});
  // ไม่กันซ้ำที่นี่ user จะกรอกฟอร์มทั้งชุดแล้วไปเจอ 403 ตอนกดสร้าง
  // (เช็คหลัง hook ทั้งหมดเสมอ — early return ก่อน hook ทำ order พัง)
  const profile = knownGameProfile(gameId);
  const listed = games.find((g) => g.id === gameId);

  if (me && !hasCapability(me, CAPABILITY.serversCreate)) {
    return (
      <p className="text-muted-foreground text-sm">{t("common.noAccess")}</p>
    );
  }
  if (pending) {
    return <Skeleton className="h-96 w-full" />;
  }
  // เกมที่ backend ไม่รองรับ หรือใหม่กว่าที่ web รู้จัก (ยังไม่มี profile = เดินฟอร์มไม่ได้)
  if (!listed || !profile) {
    return (
      <div className="grid gap-3">
        <p className="text-muted-foreground text-sm">
          {t("gamePicker.unknownGame")}
        </p>
        <div>
          <Button variant="outline" asChild>
            <Link href="/servers/new">{t("gamePicker.backToGames")}</Link>
          </Button>
        </div>
      </div>
    );
  }
  if (!listed.can_create) {
    return (
      <div className="grid gap-3">
        <p className="text-muted-foreground text-sm">
          {t("gamePicker.lockedHint")}
        </p>
        <div>
          <Button variant="outline" asChild>
            <Link href="/servers/new">{t("gamePicker.backToGames")}</Link>
          </Button>
        </div>
      </div>
    );
  }

  return <NewServerWizard profile={profile} onCreated={onCreated} />;
}
