"use client";

import { useMutation } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowLeft,
  KeyRound,
  Loader2,
  MessageSquare,
} from "lucide-react";
import { useEffect, useState } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { portalService } from "@/features/customer-portal/services/portal.service";
import type { PortalInfo } from "@/features/customer-portal/types";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

/** Generic error texts: nothing tells which detail was wrong. */
function problemText(t: (key: string) => string, error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "OTP_INVALID":
        return t("customer_portal.sign_in.errors.invalid");
      case "RATE_LIMITED":
        return t("customer_portal.sign_in.errors.rate_limited");
      case "CODE_UNAVAILABLE":
        return t("customer_portal.sign_in.errors.unavailable");
      case "VALIDATION_ERROR":
        return t("customer_portal.sign_in.errors.format");
    }
  }
  return t("customer_portal.sign_in.errors.generic");
}

/**
 * Sign-in of the customer portal: reservation reference + the phone number
 * or e-mail address on the reservation → one-time code → session (an
 * HttpOnly cookie set by the BFF). The page never says whether the
 * reference exists or which half did not match.
 */
export function PortalSignIn({
  orgSlug,
  info,
  onSignedIn,
}: {
  orgSlug: string;
  info?: PortalInfo;
  onSignedIn: () => void;
}) {
  const { t } = useLocale();
  const [reference, setReference] = useState("");
  const [contact, setContact] = useState("");
  const [code, setCode] = useState("");
  const [step, setStep] = useState<"details" | "code">("details");
  const [channel, setChannel] = useState<"sms" | "email">("sms");
  const [wait, setWait] = useState(0);

  useEffect(() => {
    if (wait <= 0) return;
    const id = window.setTimeout(() => setWait((n) => n - 1), 1000);
    return () => window.clearTimeout(id);
  }, [wait]);

  const start = useMutation({
    mutationFn: () => portalService.start(orgSlug, { reference, contact }),
    onSuccess: (out) => {
      setChannel(out.channel);
      setStep("code");
      setCode("");
      setWait(out.resend_after_seconds);
    },
  });
  const verify = useMutation({
    mutationFn: () =>
      portalService.verify(orgSlug, { reference, contact, code: code.trim() }),
    onSuccess: () => onSignedIn(),
  });
  const problem = step === "code" ? (verify.error ?? start.error) : start.error;
  const detailsValid =
    reference.trim().length >= 3 && contact.trim().length >= 5;
  const codeValid = /^\d{6}$/.test(code.trim());
  const noChannel =
    info && !info.channels.sms && !info.channels.email ? true : false;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          <KeyRound className="size-5" />
          {t("customer_portal.sign_in.title")}
        </CardTitle>
        <CardDescription>
          {step === "details"
            ? t("customer_portal.sign_in.description")
            : t(
                channel === "email"
                  ? "customer_portal.sign_in.code_sent_email"
                  : "customer_portal.sign_in.code_sent_sms",
              )}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {noChannel ? (
          <Alert>
            <AlertTriangle className="size-4" />
            <AlertDescription>
              {t("customer_portal.sign_in.errors.unavailable")}
            </AlertDescription>
          </Alert>
        ) : null}
        {problem ? (
          <Alert variant="destructive">
            <AlertTriangle className="size-4" />
            <AlertDescription>{problemText(t, problem)}</AlertDescription>
          </Alert>
        ) : null}
        {step === "details" ? (
          <form
            id="portal-start"
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (detailsValid) start.mutate();
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="portal-reference">
                {t("customer_portal.sign_in.reference")}
              </Label>
              <Input
                id="portal-reference"
                value={reference}
                onChange={(e) => setReference(e.target.value.toUpperCase())}
                autoComplete="off"
                autoCapitalize="characters"
                placeholder="ABC-2026-000123"
                className="font-mono"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="portal-contact">
                {t("customer_portal.sign_in.contact")}
              </Label>
              <Input
                id="portal-contact"
                value={contact}
                onChange={(e) => setContact(e.target.value)}
                autoComplete="tel email"
                inputMode="email"
                placeholder={t("customer_portal.sign_in.contact_placeholder")}
              />
              <p className="text-muted-foreground text-xs">
                {t("customer_portal.sign_in.contact_hint")}
              </p>
            </div>
          </form>
        ) : (
          <form
            id="portal-verify"
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              if (codeValid) verify.mutate();
            }}
          >
            <div className="space-y-2">
              <Label htmlFor="portal-code">
                {t("customer_portal.sign_in.code")}
              </Label>
              <Input
                id="portal-code"
                value={code}
                onChange={(e) =>
                  setCode(e.target.value.replace(/\D/g, "").slice(0, 6))
                }
                inputMode="numeric"
                autoComplete="one-time-code"
                pattern="[0-9]*"
                maxLength={6}
                placeholder="••••••"
                className="text-center font-mono text-lg tracking-[0.5em]"
                autoFocus
              />
            </div>
            <Button
              type="button"
              variant="outline"
              className="w-full"
              disabled={start.isPending || wait > 0}
              onClick={() => start.mutate()}
            >
              {start.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <MessageSquare className="size-4" />
              )}
              {wait > 0
                ? t("customer_portal.sign_in.resend_in", { seconds: wait })
                : t("customer_portal.sign_in.resend")}
            </Button>
          </form>
        )}
      </CardContent>
      <CardFooter className="flex flex-col gap-2">
        {step === "details" ? (
          <Button
            type="submit"
            form="portal-start"
            className="w-full"
            disabled={!detailsValid || start.isPending || noChannel}
          >
            {start.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : null}
            {t("customer_portal.sign_in.send_code")}
          </Button>
        ) : (
          <>
            <Button
              type="submit"
              form="portal-verify"
              className="w-full"
              disabled={!codeValid || verify.isPending}
            >
              {verify.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : null}
              {t("customer_portal.sign_in.verify")}
            </Button>
            <Button
              type="button"
              variant="ghost"
              className="w-full"
              onClick={() => {
                setStep("details");
                verify.reset();
                start.reset();
              }}
            >
              <ArrowLeft className="size-4" />
              {t("customer_portal.sign_in.back")}
            </Button>
          </>
        )}
      </CardFooter>
    </Card>
  );
}
