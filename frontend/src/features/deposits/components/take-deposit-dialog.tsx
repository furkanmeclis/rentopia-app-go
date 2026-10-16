"use client";

import { useMemo, useState } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";

import {
  AppDatePicker,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  DEPOSIT_METHOD_LABEL_KEYS,
  DEPOSIT_METHODS,
} from "@/features/deposits/constants";
import { useTakeDeposit } from "@/features/deposits/hooks/use-deposits";
import type { DepositMethod, RentalDeposits } from "@/features/deposits/types";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  amountText,
  errorText,
  isPositiveAmount,
  normalizeAmount,
} from "@/features/finance/lib/format";
import { useLocale } from "@/providers/locale-provider";

type Values = {
  method: string;
  amount: string;
  cash_account_uuid: string;
  pos_reference: string;
  authorization_expires_on: string;
  note: string;
};

/** Method-dependent fields (till / bank account, POS reference, hint). */
function MethodFields({ card }: { card: RentalDeposits }) {
  const { t, locale } = useLocale();
  const method = useWatch<Values, "method">({ name: "method" });
  const kind = method === "bank_transfer" ? "bank" : "cash";
  const accounts = useMemo(
    () => card.cash_accounts.filter((a) => a.kind === kind),
    [card.cash_accounts, kind],
  );
  const options = accounts.map((a) => ({
    value: a.uuid,
    label: `${a.code} · ${a.name}`,
  }));
  return (
    <>
      {method === "cash" || method === "bank_transfer" ? (
        accounts.length === 0 ? (
          <Alert>
            <AlertDescription>
              {t(
                kind === "cash"
                  ? "deposits.take.no_till"
                  : "deposits.take.no_bank",
              )}
            </AlertDescription>
          </Alert>
        ) : (
          <AppSelect
            name="cash_account_uuid"
            label={t(
              kind === "cash"
                ? "deposits.fields.till"
                : "deposits.fields.bank_account",
            )}
            description={t("deposits.take.liability_hint")}
            options={options}
            placeholder={t("deposits.fields.choose")}
          />
        )
      ) : null}
      {method === "pos_preauth" ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <AppInput
            name="pos_reference"
            label={t("deposits.fields.pos_reference")}
            description={t("deposits.take.pos_hint")}
          />
          <AppDatePicker
            name="authorization_expires_on"
            label={t("deposits.fields.hold_until")}
          />
        </div>
      ) : null}
      {method === "online_preauth" ? (
        <Alert>
          <AlertDescription>
            {t("deposits.take.online_hint", {
              amount: amountText(card.default_amount, locale, card.currency),
            })}
          </AlertDescription>
        </Alert>
      ) : null}
    </>
  );
}

/**
 * "Depozito al": cash into a till, a bank transfer into a bank account, a
 * POS provision with its slip number, or an iyzico online pre-auth (when
 * the organization's payment account supports it). The amount defaults to
 * the product's deposit amount.
 */
export function TakeDepositDialog({
  rentalUuid,
  card,
  open,
  onOpenChange,
}: {
  rentalUuid: string;
  card: RentalDeposits;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const take = useTakeDeposit();
  const methods = DEPOSIT_METHODS.filter(
    (m) => m !== "online_preauth" || card.online_enabled,
  );
  const schema = z
    .object({
      method: z.string(),
      amount: z
        .string()
        .trim()
        .refine(isPositiveAmount, t("finance.validation.amount")),
      cash_account_uuid: z.string(),
      pos_reference: z.string().trim().max(64),
      authorization_expires_on: z.string(),
      note: z.string().max(500),
    })
    .superRefine((v, ctx) => {
      if (
        (v.method === "cash" || v.method === "bank_transfer") &&
        !v.cash_account_uuid
      ) {
        ctx.addIssue({
          code: "custom",
          path: ["cash_account_uuid"],
          message: t("deposits.validation.cash_account"),
        });
      }
      if (v.method === "pos_preauth" && !v.pos_reference) {
        ctx.addIssue({
          code: "custom",
          path: ["pos_reference"],
          message: t("deposits.validation.pos_reference"),
        });
      }
    });
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<Values>
      open={open}
      onOpenChange={close}
      formId="deposit-take-form"
      title={t("deposits.take.title")}
      description={t("deposits.take.description", {
        reference: card.reference,
      })}
      submitLabel={t("deposits.actions.take")}
      pending={take.isPending}
      problem={problem}
      schema={schema}
      defaultValues={{
        method: "cash",
        amount: Number(card.default_amount) > 0 ? card.default_amount : "",
        cash_account_uuid: "",
        pos_reference: "",
        authorization_expires_on: "",
        note: "",
      }}
      onSubmit={async (v) => {
        try {
          const method = v.method as DepositMethod;
          await take.mutateAsync({
            rental_uuid: rentalUuid,
            method,
            amount: normalizeAmount(v.amount),
            cash_account_uuid:
              method === "cash" || method === "bank_transfer"
                ? v.cash_account_uuid
                : undefined,
            pos_reference:
              method === "pos_preauth" ? v.pos_reference : undefined,
            authorization_expires_at:
              method === "pos_preauth" && v.authorization_expires_on
                ? `${v.authorization_expires_on}T23:59:00+03:00`
                : undefined,
            note: v.note.trim() || undefined,
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("deposits.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppSelect
          name="method"
          label={t("deposits.fields.method")}
          options={methods.map((m) => ({
            value: m,
            label: t(DEPOSIT_METHOD_LABEL_KEYS[m]),
          }))}
        />
        <AppInput
          name="amount"
          label={t("deposits.fields.amount")}
          inputMode="decimal"
          description={
            Number(card.default_amount) > 0
              ? t("deposits.take.default_amount")
              : t("deposits.take.no_default_amount")
          }
        />
      </div>
      <MethodFields card={card} />
      <AppTextarea name="note" label={t("deposits.fields.note")} />
    </FinanceFormDialog>
  );
}
