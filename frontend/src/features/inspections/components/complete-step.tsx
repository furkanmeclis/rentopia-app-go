"use client";

import {
  AlertTriangle,
  CheckCircle2,
  CircleDashed,
  KeyRound,
  ListChecks,
} from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { EntitySectionCard } from "@/components/entity";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useCompleteInspection } from "@/features/inspections/hooks/use-inspections";
import { inspectionProblemText } from "@/features/inspections/lib/errors";
import type { Inspection } from "@/features/inspections/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

function Row({ ok, label }: { ok: boolean; label: string }) {
  const Icon = ok ? CheckCircle2 : CircleDashed;
  return (
    <li className="flex items-center gap-2 text-sm">
      <Icon
        className={cn(
          "size-4 shrink-0",
          ok ? "text-emerald-600" : "text-muted-foreground",
        )}
        aria-hidden
      />
      <span className={ok ? "" : "text-muted-foreground"}>{label}</span>
    </li>
  );
}

/**
 * Step 8: what still blocks the form (RT-A17: "Tamamla" stays disabled
 * until every photo is uploaded and verified; the API refuses the same),
 * then the completion through the rentals use case.
 */
export function CompleteStep({
  ins,
  queuePending,
}: {
  ins: Inspection;
  queuePending: number;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const complete = useCompleteInspection(ins.uuid);
  const [reason, setReason] = useState("");
  const [contractReason, setContractReason] = useState("");
  const r = ins.readiness;
  const handover = ins.kind === "handover";
  const signed = Boolean(ins.signature);
  const waivePerm = can(permissions.tenant.signature.waive);
  const needsContractWaiver =
    handover && ins.rental.contract.required && !ins.rental.contract.approved;

  if (ins.status !== "draft") {
    const review = ins.review_status === "pending";
    return (
      <EntitySectionCard title={t("inspections.complete.done_title")}>
        <div className="grid gap-3" data-testid="inspection-finalized">
          <Alert variant={review ? "destructive" : "default"}>
            {review ? (
              <AlertTriangle className="size-4" />
            ) : (
              <CheckCircle2 className="size-4 text-emerald-600" />
            )}
            <AlertTitle>
              {review
                ? t("inspections.complete.in_review")
                : t(
                    handover
                      ? "inspections.complete.handed_over"
                      : "inspections.complete.returned",
                  )}
            </AlertTitle>
            <AlertDescription>
              {review
                ? t("inspections.complete.in_review_hint")
                : t("inspections.complete.done_hint")}
            </AlertDescription>
          </Alert>
          <div className="flex flex-wrap gap-2">
            <Button asChild variant="outline">
              <Link href={routes.tenant.rentals.detail(slug, ins.rental.uuid)}>
                <KeyRound className="size-4" />
                {t("inspections.complete.open_rental")}
              </Link>
            </Button>
            <Button asChild variant="outline">
              <Link href={routes.tenant.field.root(slug)}>
                <ListChecks className="size-4" />
                {t("inspections.complete.back_to_tasks")}
              </Link>
            </Button>
          </div>
        </div>
      </EntitySectionCard>
    );
  }

  const fieldsReady = r.missing_fields.every((f) => f === "signature");
  const reasonOk = signed || reason.trim().length >= 3;
  const permOk = signed || !handover || waivePerm;
  const contractOk =
    !needsContractWaiver ||
    Boolean(ins.signature?.contract_approval) ||
    contractReason.trim().length >= 3;
  const canComplete =
    r.photos_ready &&
    fieldsReady &&
    queuePending === 0 &&
    reasonOk &&
    permOk &&
    contractOk;

  return (
    <div className="grid gap-4">
      <EntitySectionCard title={t("inspections.complete.checklist")}>
        <ul className="grid gap-2" data-testid="readiness">
          <Row
            ok={r.missing_slots.length === 0}
            label={
              r.missing_slots.length === 0
                ? t("inspections.complete.slots_ok")
                : t("inspections.complete.slots_missing", {
                    count: r.missing_slots.length,
                  })
            }
          />
          <Row
            ok={r.incomplete_photos === 0 && queuePending === 0}
            label={
              r.incomplete_photos === 0 && queuePending === 0
                ? t("inspections.complete.uploads_ok")
                : t("inspections.complete.uploads_pending", {
                    count: r.incomplete_photos + queuePending,
                  })
            }
          />
          <Row
            ok={r.damages_without_photo.length === 0}
            label={
              r.damages_without_photo.length === 0
                ? t("inspections.complete.damages_ok")
                : t("inspections.complete.damages_missing", {
                    count: r.damages_without_photo.length,
                  })
            }
          />
          <Row ok={fieldsReady} label={t("inspections.complete.fields")} />
          <Row ok={signed} label={t("inspections.complete.signature")} />
        </ul>
      </EntitySectionCard>
      {!signed ? (
        <EntitySectionCard title={t("inspections.complete.no_signature")}>
          <div className="grid gap-1.5">
            <Label htmlFor="no-signature">
              {t(
                handover
                  ? "inspections.complete.waiver_reason"
                  : "inspections.complete.refusal_reason",
              )}
            </Label>
            <Textarea
              id="no-signature"
              rows={2}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
            <span className="text-muted-foreground text-xs">
              {t(
                handover
                  ? "inspections.complete.waiver_hint"
                  : "inspections.complete.refusal_hint",
              )}
            </span>
            {handover && !waivePerm ? (
              <span className="text-destructive text-xs">
                {t("inspections.complete.waiver_forbidden")}
              </span>
            ) : null}
          </div>
        </EntitySectionCard>
      ) : null}
      {needsContractWaiver && !ins.signature?.contract_approval ? (
        <EntitySectionCard title={t("inspections.complete.contract_title")}>
          <div className="grid gap-1.5">
            <Label htmlFor="contract-waiver">
              {t("inspections.complete.contract_waiver")}
            </Label>
            <Textarea
              id="contract-waiver"
              rows={2}
              value={contractReason}
              onChange={(e) => setContractReason(e.target.value)}
            />
          </div>
        </EntitySectionCard>
      ) : null}
      <Button
        type="button"
        size="lg"
        data-testid="complete-inspection"
        disabled={!canComplete || complete.isPending}
        onClick={() =>
          complete.mutate(
            {
              no_signature_reason: signed ? undefined : reason.trim(),
              contract_waiver_reason: contractReason.trim() || undefined,
            },
            {
              onSuccess: (done) =>
                appToast.success(
                  done.review_status === "pending"
                    ? t("inspections.toast.in_review")
                    : t("inspections.toast.completed"),
                ),
              onError: (e) => appToast.error(inspectionProblemText(t, e)),
            },
          )
        }
      >
        <CheckCircle2 className="size-4" />
        {t("inspections.complete.submit")}
      </Button>
    </div>
  );
}
