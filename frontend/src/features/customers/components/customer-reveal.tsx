"use client";

import { Eye, EyeOff, ShieldCheck } from "lucide-react";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { MaskedValue } from "@/features/customers/components/customer-cells";
import { useRevealCustomer } from "@/features/customers/hooks/use-customers";
import type { CustomerRevealed } from "@/features/customers/types";
import { useLocale } from "@/providers/locale-provider";

type RevealState = {
  revealed: CustomerRevealed | null;
  /** Seconds left before the values are masked again. */
  secondsLeft: number;
  open: () => void;
  hide: () => void;
};

const RevealContext = createContext<RevealState | null>(null);

/** Revealed values of the current customer page (memory only, never cached). */
export function useCustomerReveal(): RevealState {
  const ctx = useContext(RevealContext);
  if (!ctx) {
    return { revealed: null, secondsLeft: 0, open: () => {}, hide: () => {} };
  }
  return ctx;
}

const FORM_ID = "customer-reveal-form";

/**
 * Holds the plaintext of one reveal for `display_seconds` and renders the
 * reason dialog. Values live in component state only (not in the query
 * cache, not in storage) and are dropped when the timer ends, on "Gizle" or
 * when the page unmounts.
 */
export function CustomerRevealProvider({
  customerUuid,
  children,
}: {
  customerUuid: string;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const reveal = useRevealCustomer();
  const [revealed, setRevealed] = useState<CustomerRevealed | null>(null);
  const [expiresAt, setExpiresAt] = useState(0);
  const [now, setNow] = useState(() => Date.now());
  const [dialogOpen, setDialogOpen] = useState(false);

  useEffect(() => {
    if (!revealed) return;
    const timer = window.setInterval(() => {
      const current = Date.now();
      setNow(current);
      if (current >= expiresAt) {
        setRevealed(null);
      }
    }, 1000);
    return () => window.clearInterval(timer);
  }, [expiresAt, revealed]);

  const hide = useCallback(() => setRevealed(null), []);
  const open = useCallback(() => setDialogOpen(true), []);
  const secondsLeft = revealed
    ? Math.max(0, Math.ceil((expiresAt - now) / 1000))
    : 0;

  const schema = useMemo(
    () =>
      z.object({
        reason: z
          .string()
          .trim()
          .min(3, t("customers.reveal.reason_short"))
          .max(500),
      }),
    [t],
  );

  const value = useMemo<RevealState>(
    () => ({ revealed, secondsLeft, open, hide }),
    [hide, open, revealed, secondsLeft],
  );

  return (
    <RevealContext.Provider value={value}>
      {children}
      <AppDialog
        open={dialogOpen}
        onOpenChange={(next) => {
          if (!reveal.isPending) setDialogOpen(next);
        }}
        title={t("customers.reveal.title")}
        description={t("customers.reveal.description")}
        className="sm:max-w-lg"
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setDialogOpen(false)}
              disabled={reveal.isPending}
            >
              {t("form.cancel")}
            </Button>
            <Button type="submit" form={FORM_ID} disabled={reveal.isPending}>
              <Eye className="size-4" />
              {t("customers.reveal.confirm")}
            </Button>
          </>
        }
      >
        {dialogOpen ? (
          <AppForm<{ reason: string }>
            id={FORM_ID}
            className="space-y-4"
            schema={schema}
            defaultValues={{ reason: "" }}
            onSubmit={async (values) => {
              const out = await reveal.mutateAsync({
                uuid: customerUuid,
                reason: values.reason.trim(),
              });
              const started = Date.now();
              setNow(started);
              setExpiresAt(started + out.display_seconds * 1000);
              setRevealed(out);
              setDialogOpen(false);
            }}
          >
            <Alert>
              <ShieldCheck className="size-4" />
              <AlertDescription>
                {t("customers.reveal.audit_note")}
              </AlertDescription>
            </Alert>
            <AppTextarea
              name="reason"
              label={t("customers.reveal.reason")}
              placeholder={t("customers.reveal.reason_placeholder")}
            />
          </AppForm>
        ) : null}
      </AppDialog>
    </RevealContext.Provider>
  );
}

/** Banner while values are revealed: countdown and "hide now". */
export function RevealBanner() {
  const { t } = useLocale();
  const { revealed, secondsLeft, hide } = useCustomerReveal();
  if (!revealed) return null;
  return (
    <Alert className="border-amber-500/40 bg-amber-500/5">
      <Eye className="size-4" />
      <AlertTitle>{t("customers.reveal.banner_title")}</AlertTitle>
      <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
        <span>
          {t("customers.reveal.banner_description", { seconds: secondsLeft })}
        </span>
        <Button type="button" size="sm" variant="outline" onClick={hide}>
          <EyeOff className="size-4" />
          {t("customers.reveal.hide")}
        </Button>
      </AlertDescription>
    </Alert>
  );
}

/** A sensitive value: the plaintext while revealed, else the mask. */
export function SensitiveValue({
  masked,
  plain,
}: {
  masked: string | null | undefined;
  plain: string | null | undefined;
}) {
  if (plain) {
    return (
      <span className="font-mono text-sm font-medium tabular-nums">
        {plain}
      </span>
    );
  }
  return <MaskedValue value={masked} />;
}
