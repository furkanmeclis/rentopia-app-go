"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import {
  CheckCircle2,
  Clock,
  CreditCard,
  Loader2,
  ShieldCheck,
  XCircle,
} from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";

import { AppWordmark } from "@/components/brand";
import { Loading } from "@/components/common/loading";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import {
  PUBLIC_RESULT_KEYS,
  PUBLIC_STATE_BODY_KEYS,
  PUBLIC_STATE_TITLE_KEYS,
} from "@/features/payments/constants";
import { publicPayService } from "@/features/payments/services/payments.service";
import type { PublicPayState } from "@/features/payments/types";
import { moneyText } from "@/features/pricing/lib/format";
import { reasonText } from "@/features/reservations/constants";
import { ApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Poll for at most ~5 minutes after the customer returns. */
const POLL_MS = 3000;
const POLL_LIMIT = 100;

function StateIcon({ state }: { state: PublicPayState }) {
  switch (state) {
    case "paid":
      return <CheckCircle2 className="size-10 text-emerald-600" />;
    case "processing":
      return <Loader2 className="text-muted-foreground size-10 animate-spin" />;
    case "failed":
    case "cancelled":
      return <XCircle className="text-destructive size-10" />;
    case "expired":
      return <Clock className="text-muted-foreground size-10" />;
    default:
      return <CreditCard className="text-primary size-10" />;
  }
}

/**
 * Public pay-by-link page (`/pay/{token}`, TEC-707). Shows the organization,
 * the reservation reference and the amount, sends the customer to the
 * provider's hosted 3DS checkout (no card field here) and, after the
 * return, polls the API until a verified result exists. The return itself
 * is never trusted: only what the API reports from a verified webhook or
 * provider retrieve is shown as paid.
 */
export function PayPage({ token }: { token: string }) {
  const { t, locale } = useLocale();
  const params = useSearchParams();
  const returned = params.get("returned") === "1";
  const [polls, setPolls] = useState(0);

  const query = useQuery({
    queryKey: ["public", "pay", token, polls],
    queryFn: () => publicPayService.get(token, polls > 0 || returned),
    retry: false,
    placeholderData: (previous) => previous,
  });
  const state = query.data?.state;
  const polling =
    (state === "processing" || (returned && state === "ready")) &&
    polls < POLL_LIMIT;

  useEffect(() => {
    if (!polling) return;
    const id = window.setTimeout(() => setPolls((n) => n + 1), POLL_MS);
    return () => window.clearTimeout(id);
  }, [polling, polls]);

  const checkout = useMutation({
    mutationFn: () => publicPayService.checkout(token, locale),
    onSuccess: (out) => {
      window.location.assign(out.payment_page_url);
    },
  });

  let body: React.ReactNode;
  if (query.isLoading) {
    body = <Loading />;
  } else if (query.isError || !query.data) {
    body = (
      <Alert variant="destructive">
        <AlertTitle>{t("payments.pay.not_found_title")}</AlertTitle>
        <AlertDescription>{t("payments.pay.not_found_body")}</AlertDescription>
      </Alert>
    );
  } else {
    const link = query.data;
    body = (
      <Card>
        <CardHeader className="items-center text-center">
          <StateIcon state={link.state} />
          <CardTitle className="text-xl">
            {t(PUBLIC_STATE_TITLE_KEYS[link.state])}
          </CardTitle>
          <CardDescription>
            {link.state === "paid" && link.result
              ? reasonText(t, PUBLIC_RESULT_KEYS, link.result)
              : t(PUBLIC_STATE_BODY_KEYS[link.state])}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3 text-sm">
          <div className="flex justify-between gap-4">
            <span className="text-muted-foreground">
              {t("payments.pay.organization")}
            </span>
            <span className="font-medium">{link.organization_name}</span>
          </div>
          <div className="flex justify-between gap-4">
            <span className="text-muted-foreground">
              {t("payments.pay.reference")}
            </span>
            <code className="font-medium">{link.reference}</code>
          </div>
          <div className="flex justify-between gap-4">
            <span className="text-muted-foreground">
              {t("payments.pay.vehicle")}
            </span>
            <span className="text-end">{link.product_title}</span>
          </div>
          <div className="flex justify-between gap-4">
            <span className="text-muted-foreground">
              {t("payments.pay.dates")}
            </span>
            <span className="text-end tabular-nums">
              {datetime(link.starts_at, "d MMM yyyy HH:mm", locale)} →{" "}
              {datetime(link.ends_at, "d MMM yyyy HH:mm", locale)}
            </span>
          </div>
          <Separator />
          <div className="flex items-center justify-between gap-4 text-base">
            <span>{t("payments.pay.amount")}</span>
            <span className="font-semibold tabular-nums">
              {moneyText(link.amount, locale)}
            </span>
          </div>
          {link.state === "ready" || link.state === "failed" ? (
            <p className="text-muted-foreground text-xs">
              {t("payments.pay.expires", {
                expires: datetime(link.expires_at, "d MMM yyyy HH:mm", locale),
              })}
            </p>
          ) : null}
          {checkout.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {checkout.error instanceof ApiError
                  ? checkout.error.message
                  : t("payments.pay.checkout_failed")}
              </AlertDescription>
            </Alert>
          ) : null}
          {link.state === "processing" && !polling ? (
            <Alert>
              <AlertDescription>
                {t("payments.pay.still_processing")}
              </AlertDescription>
            </Alert>
          ) : null}
        </CardContent>
        {link.state === "ready" || link.state === "failed" ? (
          <CardFooter className="flex-col gap-3">
            <Button
              type="button"
              className="w-full"
              disabled={checkout.isPending || checkout.isSuccess}
              onClick={() => checkout.mutate()}
            >
              {checkout.isPending || checkout.isSuccess ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <CreditCard className="size-4" />
              )}
              {link.state === "failed"
                ? t("payments.pay.retry")
                : t("payments.pay.pay_now")}
            </Button>
            <p className="text-muted-foreground flex items-center gap-1.5 text-xs">
              <ShieldCheck className="size-3.5" />
              {t("payments.pay.secure_note")}
            </p>
          </CardFooter>
        ) : null}
      </Card>
    );
  }

  return (
    <div className="bg-muted relative flex min-h-svh flex-col items-center justify-center p-6 md:p-10">
      <div className="absolute top-4 right-4 flex items-center gap-1">
        <LocaleSwitch />
        <ThemeSwitch />
      </div>
      <div className="flex w-full max-w-md flex-col gap-6">
        <div className="self-center">
          <AppWordmark />
        </div>
        {body}
      </div>
    </div>
  );
}
