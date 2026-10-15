"use client";

import { ArrowLeft, ArrowRight, CloudOff, Wifi } from "lucide-react";
import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ChargesStep } from "@/features/inspections/components/charges-step";
import { CompleteStep } from "@/features/inspections/components/complete-step";
import { DamageStep } from "@/features/inspections/components/damage-step";
import { UploadQueuePanel } from "@/features/inspections/components/photo-capture";
import { PhotosStep } from "@/features/inspections/components/photos-step";
import { SignatureStep } from "@/features/inspections/components/signature-step";
import {
  AmountsStep,
  ReadingsStep,
  ReviewStep,
  VerifyStep,
} from "@/features/inspections/components/wizard-steps";
import {
  HANDOVER_STEPS,
  KIND_LABEL_KEYS,
  RETURN_STEPS,
  STEP_LABEL_KEYS,
  type WizardStep,
} from "@/features/inspections/constants";
import {
  useInspection,
  useStartInspection,
  useUpdateDraft,
  useUploadQueue,
} from "@/features/inspections/hooks/use-inspections";
import {
  inspectionProblemText,
  isNetworkError,
} from "@/features/inspections/lib/errors";
import {
  clearDraft,
  loadDraft,
  saveDraft,
} from "@/features/inspections/lib/offline-store";
import type {
  InspectionDraftRequest,
  InspectionKind,
} from "@/features/inspections/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const STEP_KEY = (uuid: string) => `inspection-step:${uuid}`;

function readStep(uuid: string): number {
  try {
    const v = Number(window.localStorage.getItem(STEP_KEY(uuid)));
    return Number.isInteger(v) && v >= 0 ? v : 0;
  } catch {
    return 0;
  }
}

function writeStep(uuid: string, step: number) {
  try {
    window.localStorage.setItem(STEP_KEY(uuid), String(step));
  } catch {
    // Private mode / blocked storage: the step is not remembered.
  }
}

function subscribeOnline(onChange: () => void) {
  window.addEventListener("online", onChange);
  window.addEventListener("offline", onChange);
  return () => {
    window.removeEventListener("online", onChange);
    window.removeEventListener("offline", onChange);
  };
}

/** Merges equipment maps of queued patches (later wins per item). */
function mergePatch(
  a: InspectionDraftRequest,
  b: InspectionDraftRequest,
): InspectionDraftRequest {
  return {
    ...a,
    ...b,
    equipment:
      a.equipment || b.equipment
        ? { ...(a.equipment ?? {}), ...(b.equipment ?? {}) }
        : undefined,
  };
}

/**
 * Mobile / tablet handover and return wizard (TEC-712 / TEC-713). The form
 * lives on the server; edits made without a connection are kept on the
 * device and synced when it returns, photos go through the device upload
 * queue.
 */
export function InspectionWizard({
  rentalUuid,
  kind,
}: {
  rentalUuid: string;
  kind: InspectionKind;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const started = useStartInspection(rentalUuid, kind);
  const uuid = started.data?.uuid ?? null;
  const live = useInspection(uuid);
  const ins = live.data ?? started.data;
  const update = useUpdateDraft(uuid ?? "");
  const queue = useUploadQueue(uuid);
  const [steps_, setSteps] = useState<Record<string, number>>({});
  const [unsynced, setUnsynced] = useState(false);
  const offline = !useSyncExternalStore(
    subscribeOnline,
    () => navigator.onLine,
    () => true,
  );
  const steps: readonly WizardStep[] =
    kind === "handover" ? HANDOVER_STEPS : RETURN_STEPS;
  // The step is remembered per form on the device.
  const step = uuid
    ? Math.min(steps_[uuid] ?? readStep(uuid), steps.length - 1)
    : 0;

  /** Sends the draft fields kept on the device. */
  const sync = useCallback(async () => {
    if (!uuid) return;
    const pending = (await loadDraft(uuid)) as
      InspectionDraftRequest | undefined;
    if (!pending || Object.keys(pending).length === 0) {
      setUnsynced(false);
      return;
    }
    try {
      await update.mutateAsync(pending);
      await clearDraft(uuid);
      setUnsynced(false);
    } catch (error) {
      setUnsynced(true);
      if (!isNetworkError(error)) {
        await clearDraft(uuid);
        appToast.error(inspectionProblemText(t, error));
      }
    }
  }, [t, update, uuid]);

  useEffect(() => {
    const online = () => void sync();
    const timer = window.setTimeout(online, 0);
    window.addEventListener("online", online);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("online", online);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- once per form
  }, [uuid]);

  const commit = useCallback(
    async (patch: InspectionDraftRequest) => {
      if (!uuid) return false;
      try {
        await update.mutateAsync(patch);
        return true;
      } catch (error) {
        if (isNetworkError(error)) {
          const prev = ((await loadDraft(uuid)) ??
            {}) as InspectionDraftRequest;
          await saveDraft(uuid, mergePatch(prev, patch));
          setUnsynced(true);
          appToast.info(t("inspections.offline.saved_on_device"));
          return true;
        }
        appToast.error(inspectionProblemText(t, error));
        return false;
      }
    },
    [t, update, uuid],
  );

  const goto = (next: number) => {
    const bounded = Math.max(0, Math.min(steps.length - 1, next));
    if (uuid) {
      setSteps((prev) => ({ ...prev, [uuid]: bounded }));
      writeStep(uuid, bounded);
    }
    if (typeof window !== "undefined") window.scrollTo({ top: 0 });
  };

  const pendingUploads = queue.items.filter((i) => i.status !== "done").length;
  const finalized = Boolean(ins && ins.status !== "draft");
  const { clearDone } = queue;
  useEffect(() => {
    if (finalized) void clearDone();
  }, [clearDone, finalized]);
  const slotLabel = useMemo(() => {
    const labels = new Map(
      (ins?.template.photo_slots ?? []).map((s) => [s.key, s.label]),
    );
    return (slot: string) =>
      labels.get(slot) ??
      (slot === "damage"
        ? t("inspections.photos.damage")
        : t("inspections.photos.extra"));
  }, [ins?.template.photo_slots, t]);

  const title = t(
    kind === "handover"
      ? "inspections.wizard.title_handover"
      : "inspections.wizard.title_return",
  );

  if (started.isError) {
    return (
      <EntityPage title={title} permission={permissions.tenant.rentals.write}>
        <ErrorState
          title={t("inspections.wizard.start_failed")}
          description={inspectionProblemText(t, started.error)}
        />
      </EntityPage>
    );
  }
  if (!ins) return <Loading />;

  const readOnly = ins.status !== "draft";
  const current = steps[step];
  const onPhoto = (file: File, slot: string) => void queue.add(file, slot);
  const onDamagePhoto = (file: File, damage: string) =>
    void queue.add(file, "damage", damage);
  const stepProps = { ins, readOnly, commit };

  return (
    <EntityPage
      title={`${title} · ${ins.rental.reference}`}
      description={`${ins.rental.vehicle.plate} · ${ins.rental.customer.display_name}`}
      permission={permissions.tenant.rentals.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("rentals.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("inspections.field.title"),
          href: routes.tenant.field.root(slug),
        },
        { label: t(KIND_LABEL_KEYS[kind]) },
      ]}
    >
      <div className="mx-auto grid w-full max-w-3xl gap-4 pb-24">
        <div className="grid gap-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-sm font-medium" data-testid="wizard-step">
              {t("inspections.wizard.step_of", {
                step: step + 1,
                total: steps.length,
              })}{" "}
              · {t(STEP_LABEL_KEYS[current])}
            </span>
            <span className="flex flex-wrap items-center gap-1.5">
              {ins.version > 1 ? (
                <Badge variant="outline">v{ins.version}</Badge>
              ) : null}
              {offline ? (
                <Badge variant="warning">
                  <CloudOff className="size-3" />
                  {t("inspections.offline.offline")}
                </Badge>
              ) : unsynced ? (
                <Badge variant="warning">
                  <Wifi className="size-3" />
                  {t("inspections.offline.unsynced")}
                </Badge>
              ) : null}
              {pendingUploads > 0 ? (
                <Badge variant="secondary">
                  {t("inspections.queue.pending", { count: pendingUploads })}
                </Badge>
              ) : null}
            </span>
          </div>
          <Progress value={((step + 1) / steps.length) * 100} />
        </div>

        {current === "verify" ? <VerifyStep {...stepProps} /> : null}
        {current === "amounts" ? <AmountsStep {...stepProps} /> : null}
        {current === "readings" ? <ReadingsStep {...stepProps} /> : null}
        {current === "damages" ? (
          <DamageStep ins={ins} readOnly={readOnly} onPhoto={onDamagePhoto} />
        ) : null}
        {current === "photos" ? (
          <PhotosStep
            ins={ins}
            readOnly={readOnly}
            queue={queue.items}
            onPhoto={onPhoto}
          />
        ) : null}
        {current === "charges" ? <ChargesStep {...stepProps} /> : null}
        {current === "review" ? <ReviewStep {...stepProps} /> : null}
        {current === "signature" ? (
          <SignatureStep ins={ins} readOnly={readOnly} />
        ) : null}
        {current === "complete" ? (
          <CompleteStep ins={ins} queuePending={pendingUploads} />
        ) : null}

        {current === "photos" ||
        current === "damages" ||
        current === "complete" ? (
          <UploadQueuePanel
            items={queue.items}
            slotLabel={slotLabel}
            onRetry={() => void queue.retry()}
            onRemove={(id) => void queue.remove(id)}
          />
        ) : null}

        <div className="bg-background/95 supports-[backdrop-filter]:bg-background/80 sticky bottom-0 z-10 -mx-1 flex items-center justify-between gap-2 border-t px-1 py-3 backdrop-blur">
          <Button
            type="button"
            variant="outline"
            disabled={step === 0}
            onClick={() => goto(step - 1)}
          >
            <ArrowLeft className="size-4" />
            {t("inspections.wizard.back")}
          </Button>
          {step < steps.length - 1 ? (
            <Button
              type="button"
              data-testid="wizard-next"
              onClick={() => goto(step + 1)}
            >
              {t("inspections.wizard.next")}
              <ArrowRight className="size-4" />
            </Button>
          ) : null}
        </div>
      </div>
    </EntityPage>
  );
}
