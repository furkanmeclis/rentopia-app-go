"use client";

import { AlertTriangle, ArrowRight } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  CAPACITY_REASON_LABEL_KEYS,
  DEFAULT_HOLD_MINUTES,
  MAX_HOLD_MINUTES,
} from "@/features/availability/constants";
import { useCreateHold } from "@/features/availability/hooks/use-availability";
import type {
  AvailabilityAlternatives,
  AvailabilityProduct,
  AvailabilitySearchRequest,
} from "@/features/availability/types";
import { ApiError } from "@/lib/api/errors";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const FORM_ID = "availability-hold-form";

type Values = { hold_minutes: string; note: string };

type CapacityProblem = {
  reason: string;
  alternatives: AvailabilityAlternatives | null;
};

/** CAPACITY_UNAVAILABLE carries the reason (detail code) and alternatives (data). */
function capacityProblem(error: unknown): CapacityProblem | null {
  if (!(error instanceof ApiError) || error.code !== "CAPACITY_UNAVAILABLE") {
    return null;
  }
  const body = error.body as
    | (typeof error.body & { data?: AvailabilityAlternatives | null })
    | undefined;
  return {
    reason: error.details[0]?.code ?? "no_vehicle",
    alternatives: body?.data ?? null,
  };
}

/**
 * "Opsiyon koy": puts a timed hold for the searched range on one product.
 * The engine plans a vehicle for the whole range in one transaction; when
 * the last unit went in the meantime the dialog shows why and offers the
 * alternatives the API returned (other products, nearby ranges).
 */
export function HoldDialog({
  product,
  request,
  onOpenChange,
  onPickProduct,
  onPickRange,
}: {
  product: AvailabilityProduct | null;
  request: AvailabilitySearchRequest | null;
  onOpenChange: (open: boolean) => void;
  onPickProduct: (productUuid: string) => void;
  onPickRange: (startsAt: string, endsAt: string) => void;
}) {
  const { t, locale } = useLocale();
  const create = useCreateHold();
  const [problem, setProblem] = useState<CapacityProblem | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        hold_minutes: z
          .string()
          .trim()
          .refine((v) => {
            const n = Number(v);
            return Number.isInteger(n) && n >= 1 && n <= MAX_HOLD_MINUTES;
          }, t("availability.validation.hold_minutes")),
        note: z.string().max(2000),
      }),
    [t],
  );
  const open = Boolean(product && request);
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("availability.hold_dialog.title")}
      description={t("availability.hold_dialog.description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={create.isPending}>
            {t("availability.actions.hold")}
          </Button>
        </>
      }
    >
      {product && request ? (
        <div className="space-y-4">
          <div className="bg-muted/40 space-y-1 rounded-md border p-3 text-sm">
            <p className="font-medium">
              <code className="text-xs">{product.product.code}</code>{" "}
              {product.product.title}
            </p>
            <p className="text-muted-foreground tabular-nums">
              {datetime(request.starts_at, undefined, locale)} –{" "}
              {datetime(request.ends_at, undefined, locale)}
            </p>
            <p className="text-muted-foreground">
              {t("availability.hold_dialog.sellable", {
                count: product.sellable,
              })}
            </p>
          </div>
          {problem ? (
            <Alert variant="destructive">
              <AlertTriangle className="size-4" />
              <AlertTitle>
                {t("availability.hold_dialog.unavailable")}
              </AlertTitle>
              <AlertDescription className="space-y-2">
                <p>
                  {t(
                    CAPACITY_REASON_LABEL_KEYS[problem.reason] ??
                      CAPACITY_REASON_LABEL_KEYS.no_vehicle,
                  )}
                </p>
                {problem.alternatives &&
                (problem.alternatives.products.length > 0 ||
                  problem.alternatives.times.length > 0) ? (
                  <div className="flex flex-wrap gap-2">
                    {problem.alternatives.products.map((alt) => (
                      <Button
                        key={alt.product.uuid}
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setProblem(null);
                          onPickProduct(alt.product.uuid);
                        }}
                      >
                        <ArrowRight className="size-3.5" />
                        {alt.product.code} ({alt.sellable})
                      </Button>
                    ))}
                    {problem.alternatives.times.map((alt) => (
                      <Button
                        key={alt.starts_at}
                        type="button"
                        size="sm"
                        variant="outline"
                        onClick={() => {
                          setProblem(null);
                          onPickRange(alt.starts_at, alt.ends_at);
                        }}
                      >
                        <ArrowRight className="size-3.5" />
                        {datetime(alt.starts_at, undefined, locale)}
                      </Button>
                    ))}
                  </div>
                ) : null}
              </AlertDescription>
            </Alert>
          ) : null}
          <AppForm<Values>
            key={product.product.uuid}
            id={FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              hold_minutes: String(DEFAULT_HOLD_MINUTES),
              note: "",
            }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                const result = await create.mutateAsync({
                  ...request,
                  product_uuid: product.product.uuid,
                  hold_minutes: Number(values.hold_minutes),
                  note: values.note.trim(),
                });
                const hold = result.allocation;
                appToast.success(
                  t("availability.toast.held", {
                    plate: hold.vehicle.plate,
                    expires: hold.expires_at
                      ? datetime(hold.expires_at, "HH:mm", locale)
                      : "—",
                  }),
                );
                close(false);
              } catch (error) {
                const capacity = capacityProblem(error);
                if (capacity) {
                  setProblem(capacity);
                  return;
                }
                appToast.error(
                  error instanceof Error
                    ? error.message
                    : t("availability.toast.hold_failed"),
                );
              }
            }}
          >
            <AppInput
              name="hold_minutes"
              type="number"
              inputMode="numeric"
              min={1}
              max={MAX_HOLD_MINUTES}
              label={t("availability.fields.hold_minutes")}
              description={t("availability.fields.hold_minutes_hint")}
            />
            <AppTextarea name="note" label={t("availability.fields.note")} />
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}
