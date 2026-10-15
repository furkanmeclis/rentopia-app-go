"use client";

import { useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppSwitch, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { PaymentLinkBox } from "@/features/payments/components/payment-link-box";
import { useAcceptReservation } from "@/features/payments/hooks/use-payments";
import type { ReservationAcceptResult } from "@/features/payments/types";
import { moneyText } from "@/features/pricing/lib/format";
import { errorData } from "@/features/reservations/lib/errors";
import type { Quote } from "@/features/reservations/types";
import { usePermission } from "@/providers/permission-provider";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "reservation-accept-form";

type Values = { send_payment_link: boolean; note: string };

/**
 * Staff accept of a request / review (approval queue, reservation detail):
 * capacity at the current availability and price, the payment window and,
 * optionally, a pay-by-link link for the prepayment due. A changed price
 * is shown with the new total and accepted explicitly.
 */
export function AcceptDialog({
  reservation,
  onClose,
}: {
  reservation: { uuid: string; reference: string; total: string } | null;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canLink = can(permissions.tenant.payments.createLink);
  const accept = useAcceptReservation();
  const [replacement, setReplacement] = useState<Quote | null>(null);
  const [result, setResult] = useState<ReservationAcceptResult | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const [values, setValues] = useState<Values | null>(null);

  const close = () => {
    setReplacement(null);
    setResult(null);
    setProblem(null);
    setValues(null);
    onClose();
  };

  const submit = async (v: Values, quoteUuid?: string) => {
    if (!reservation) return;
    setProblem(null);
    setValues(v);
    try {
      const out = await accept.mutateAsync({
        uuid: reservation.uuid,
        send_payment_link: canLink && v.send_payment_link,
        note: v.note.trim() || undefined,
        quote_uuid: quoteUuid,
      });
      setReplacement(null);
      if (out.payment_link || out.payment_link_error) {
        setResult(out);
      } else {
        close();
      }
    } catch (error) {
      if (error instanceof ApiError && error.code === "QUOTE_PRICE_CHANGED") {
        setReplacement(errorData<Quote>(error));
        return;
      }
      setProblem(
        error instanceof ApiError ? error.message : t("payments.accept.failed"),
      );
    }
  };

  return (
    <AppDialog
      open={Boolean(reservation)}
      onOpenChange={(open) => {
        if (!open) close();
      }}
      title={t("payments.accept.title", {
        reference: reservation?.reference ?? "",
      })}
      description={t("payments.accept.description")}
      className="sm:max-w-lg"
      footer={
        result ? (
          <Button type="button" onClick={close}>
            {t("payments.accept.done")}
          </Button>
        ) : (
          <>
            <Button type="button" variant="outline" onClick={close}>
              {t("form.cancel")}
            </Button>
            {replacement && values ? (
              <Button
                type="button"
                disabled={accept.isPending}
                onClick={() => void submit(values, replacement.uuid)}
              >
                {t("payments.accept.accept_new_price")}
              </Button>
            ) : (
              <Button type="submit" form={FORM_ID} disabled={accept.isPending}>
                {t("payments.accept.confirm")}
              </Button>
            )}
          </>
        )
      }
    >
      {result ? (
        <div className="space-y-3">
          {result.payment_link ? (
            <PaymentLinkBox
              url={result.payment_link.url}
              amount={result.payment_link.amount}
              expiresAt={result.payment_link.expires_at}
            />
          ) : (
            <Alert variant="destructive">
              <AlertTitle>{t("payments.accept.link_failed")}</AlertTitle>
              <AlertDescription>{result.payment_link_error}</AlertDescription>
            </Alert>
          )}
        </div>
      ) : reservation ? (
        <div className="space-y-4">
          {replacement ? (
            <Alert>
              <AlertTitle>
                {t("payments.accept.price_changed_title")}
              </AlertTitle>
              <AlertDescription>
                {t("payments.accept.price_changed_body", {
                  old: moneyText(reservation.total, locale),
                  new: moneyText(replacement.total, locale),
                })}
              </AlertDescription>
            </Alert>
          ) : null}
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AppForm<Values>
            id={FORM_ID}
            schema={z.object({
              send_payment_link: z.boolean(),
              note: z.string().max(500),
            })}
            defaultValues={{ send_payment_link: canLink, note: "" }}
            onSubmit={(v) => submit(v)}
          >
            <div className="space-y-4">
              <AppSwitch
                name="send_payment_link"
                label={t("payments.accept.send_link")}
                description={t(
                  canLink
                    ? "payments.accept.send_link_hint"
                    : "payments.accept.send_link_forbidden",
                )}
                disabled={!canLink || Boolean(replacement)}
              />
              <AppTextarea
                name="note"
                label={t("payments.accept.note")}
                disabled={Boolean(replacement)}
              />
            </div>
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}
