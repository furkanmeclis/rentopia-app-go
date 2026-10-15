"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Info } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppDatePicker, AppForm, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { portalKeys } from "@/features/customer-portal/hooks/use-portal";
import { portalService } from "@/features/customer-portal/services/portal.service";
import type {
  CustomerRequestType,
  PortalReservation,
} from "@/features/customer-portal/types";
import { moneyText } from "@/features/pricing/lib/format";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const FORM_ID = "portal-request-form";

type Values = { message: string; starts_at: string; ends_at: string };

/** Local "yyyy-MM-dd'T'HH:mm" → ISO, or null when empty. */
function toIso(local: string): string | null {
  if (!local) return null;
  const d = new Date(local);
  return Number.isNaN(d.getTime()) ? null : d.toISOString();
}

function errorText(t: (key: string) => string, error: unknown) {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "REQUEST_EXISTS":
        return t("customer_portal.requests.errors.exists");
      case "INVALID_STATE":
        return t("customer_portal.requests.errors.state");
      case "VALIDATION_ERROR":
        return t("customer_portal.requests.errors.validation");
    }
  }
  return t("customer_portal.requests.errors.generic");
}

/**
 * Change / cancellation request. A cancellation shows the policy text and
 * fee of the reservation's policy snapshot; staff decide unless the policy
 * explicitly allows cancelling here.
 */
export function PortalRequestDialog({
  orgSlug,
  reservation,
  type,
  onOpenChange,
}: {
  orgSlug: string;
  reservation: PortalReservation;
  type: CustomerRequestType | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const client = useQueryClient();
  const create = useMutation({
    mutationFn: (values: Values) =>
      portalService.createRequest(orgSlug, reservation.uuid, {
        type: type ?? "change",
        message: values.message.trim(),
        requested_starts_at: type === "change" ? toIso(values.starts_at) : null,
        requested_ends_at: type === "change" ? toIso(values.ends_at) : null,
      }),
    onSuccess: (out) => {
      appToast.success(
        t(
          out.outcome === "self_cancelled"
            ? "customer_portal.requests.toast.cancelled"
            : "customer_portal.requests.toast.sent",
        ),
      );
      void client.invalidateQueries({ queryKey: portalKeys.all(orgSlug) });
      onOpenChange(false);
    },
  });
  const schema = useMemo(
    () =>
      z.object({
        message:
          type === "change"
            ? z
                .string()
                .trim()
                .min(3, t("customer_portal.requests.validation.message"))
                .max(2000)
            : z.string().max(2000),
        starts_at: z.string(),
        ends_at: z.string(),
      }),
    [t, type],
  );
  const c = reservation.cancellation;

  return (
    <AppDialog
      open={type !== null}
      onOpenChange={(open) => {
        if (!open && !create.isPending) {
          create.reset();
          onOpenChange(false);
        }
      }}
      title={t(
        type === "cancel"
          ? "customer_portal.requests.cancel_title"
          : "customer_portal.requests.change_title",
      )}
      description={t(
        type === "cancel"
          ? "customer_portal.requests.cancel_description"
          : "customer_portal.requests.change_description",
      )}
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={create.isPending}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            variant={type === "cancel" ? "destructive" : "default"}
            disabled={create.isPending}
          >
            {t(
              type === "cancel" && c.self_cancel
                ? "customer_portal.requests.cancel_now"
                : "customer_portal.requests.send",
            )}
          </Button>
        </>
      }
    >
      {type ? (
        <AppForm<Values>
          id={FORM_ID}
          className="space-y-4"
          schema={schema}
          defaultValues={{ message: "", starts_at: "", ends_at: "" }}
          onSubmit={async (values) => {
            await create.mutateAsync(values).catch(() => undefined);
          }}
        >
          {type === "cancel" ? (
            <Alert>
              <Info className="size-4" />
              <AlertTitle>{t("customer_portal.requests.policy")}</AlertTitle>
              <AlertDescription className="space-y-1">
                <p className="whitespace-pre-line">
                  {c.policy_text || t("customer_portal.requests.no_policy")}
                </p>
                {c.fee ? (
                  <p className="font-medium">
                    {t("customer_portal.requests.fee", {
                      fee: moneyText(c.fee, locale),
                    })}
                  </p>
                ) : null}
                <p>
                  {t(
                    c.self_cancel
                      ? "customer_portal.requests.self_cancel_hint"
                      : "customer_portal.requests.staff_hint",
                  )}
                </p>
              </AlertDescription>
            </Alert>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2">
              <AppDatePicker
                name="starts_at"
                withTime
                label={t("customer_portal.requests.starts_at")}
              />
              <AppDatePicker
                name="ends_at"
                withTime
                label={t("customer_portal.requests.ends_at")}
              />
            </div>
          )}
          <AppTextarea
            name="message"
            rows={3}
            label={t(
              type === "cancel"
                ? "customer_portal.requests.reason"
                : "customer_portal.requests.message",
            )}
          />
          {create.error ? (
            <Alert variant="destructive">
              <AlertTriangle className="size-4" />
              <AlertDescription>{errorText(t, create.error)}</AlertDescription>
            </Alert>
          ) : null}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
