"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import {
  AppDatePicker,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  COLLECTION_METHODS,
  METHOD_LABEL_KEYS,
  PURPOSE_LABEL_KEYS,
} from "@/features/finance/constants";
import {
  useAdjust,
  useCashAccountsList,
  useCollect,
  useOffset,
  useRefund,
  useReverseEntry,
  useSendStatement,
} from "@/features/finance/hooks/use-finance";
import {
  amountText,
  errorText,
  isPositiveAmount,
  normalizeAmount,
  todayISO,
} from "@/features/finance/lib/format";
import type {
  FinanceAccount,
  LedgerEntry,
  StatementSent,
} from "@/features/finance/types";
import { PaymentLinkBox } from "@/features/payments/components/payment-link-box";
import { useLocale } from "@/providers/locale-provider";

type DialogProps = {
  account: FinanceAccount;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const NONE = "none";

function useAmountSchema() {
  const { t } = useLocale();
  return z
    .string()
    .trim()
    .refine(isPositiveAmount, t("finance.validation.amount"));
}

function useReasonSchema() {
  const { t } = useLocale();
  return z
    .string()
    .trim()
    .min(3, t("finance.validation.reason"))
    .max(500, t("finance.validation.reason"));
}

/** Active cash / bank accounts as select options (with "none"). */
function useCashOptions(enabled: boolean) {
  const { t } = useLocale();
  const query = useCashAccountsList(
    { limit: 100, offset: 0, status: "active", sort: "code" },
    enabled,
  );
  return useMemo(
    () => [
      { value: NONE, label: t("finance.fields.no_cash_account") },
      ...(query.data?.items ?? []).map((a) => ({
        value: a.uuid,
        label: `${a.code} · ${a.name}`,
      })),
    ],
    [query.data?.items, t],
  );
}

type CollectionValues = {
  amount: string;
  purpose: string;
  method: string;
  entry_date: string;
  cash_account_uuid: string;
  description: string;
};

export function CollectionDialog({ account, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const collect = useCollect(account.uuid);
  const amount = useAmountSchema();
  const cash = useCashOptions(open);
  const schema = z.object({
    amount,
    purpose: z.string(),
    method: z.string(),
    entry_date: z.string(),
    cash_account_uuid: z.string(),
    description: z.string().max(500),
  });
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<CollectionValues>
      open={open}
      onOpenChange={close}
      formId="finance-collection-form"
      title={t("finance.collection.title")}
      description={t("finance.collection.description")}
      submitLabel={t("finance.actions.collect")}
      pending={collect.isPending}
      problem={problem}
      schema={schema}
      defaultValues={{
        amount:
          Number(account.balance.balance) > 0 ? account.balance.balance : "",
        purpose: "general",
        method: "bank_transfer",
        entry_date: todayISO(),
        cash_account_uuid: NONE,
        description: "",
      }}
      onSubmit={async (v) => {
        try {
          await collect.mutateAsync({
            amount: normalizeAmount(v.amount),
            purpose: v.purpose as "general",
            method: v.method as "cash",
            entry_date: v.entry_date || undefined,
            cash_account_uuid:
              v.cash_account_uuid !== NONE ? v.cash_account_uuid : undefined,
            description: v.description.trim() || undefined,
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="amount"
          label={t("finance.fields.amount")}
          inputMode="decimal"
        />
        <AppDatePicker name="entry_date" label={t("finance.fields.date")} />
        <AppSelect
          name="purpose"
          label={t("finance.fields.purpose")}
          options={(["general", "prepayment", "deposit"] as const).map((p) => ({
            value: p,
            label: t(PURPOSE_LABEL_KEYS[p]),
          }))}
        />
        <AppSelect
          name="method"
          label={t("finance.fields.method")}
          options={COLLECTION_METHODS.map((m) => ({
            value: m,
            label: t(METHOD_LABEL_KEYS[m]),
          }))}
        />
      </div>
      <AppSelect
        name="cash_account_uuid"
        label={t("finance.fields.cash_account")}
        description={t("finance.fields.cash_account_hint")}
        options={cash}
      />
      <AppTextarea name="description" label={t("finance.fields.description")} />
    </FinanceFormDialog>
  );
}

type RefundValues = {
  amount: string;
  purpose: string;
  entry_date: string;
  cash_account_uuid: string;
  reason: string;
};

export function RefundDialog({ account, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const refund = useRefund(account.uuid);
  const amount = useAmountSchema();
  const reason = useReasonSchema();
  const cash = useCashOptions(open);
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<RefundValues>
      open={open}
      onOpenChange={close}
      formId="finance-refund-form"
      title={t("finance.refund.title")}
      description={t("finance.refund.description")}
      submitLabel={t("finance.actions.refund")}
      pending={refund.isPending}
      problem={problem}
      schema={z.object({
        amount,
        purpose: z.string(),
        entry_date: z.string(),
        cash_account_uuid: z.string(),
        reason,
      })}
      defaultValues={{
        amount: "",
        purpose:
          Number(account.balance.deposit_held) > 0 ? "deposit" : "general",
        entry_date: todayISO(),
        cash_account_uuid: NONE,
        reason: "",
      }}
      onSubmit={async (v) => {
        try {
          await refund.mutateAsync({
            amount: normalizeAmount(v.amount),
            purpose: v.purpose as "general",
            entry_date: v.entry_date || undefined,
            cash_account_uuid:
              v.cash_account_uuid !== NONE ? v.cash_account_uuid : undefined,
            reason: v.reason.trim(),
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="amount"
          label={t("finance.fields.amount")}
          inputMode="decimal"
        />
        <AppDatePicker name="entry_date" label={t("finance.fields.date")} />
      </div>
      <AppSelect
        name="purpose"
        label={t("finance.fields.purpose")}
        options={(["general", "deposit", "prepayment"] as const).map((p) => ({
          value: p,
          label: t(PURPOSE_LABEL_KEYS[p]),
        }))}
      />
      <AppSelect
        name="cash_account_uuid"
        label={t("finance.fields.cash_account_out")}
        options={cash}
      />
      <AppTextarea name="reason" label={t("finance.fields.reason")} />
    </FinanceFormDialog>
  );
}

type AdjustmentValues = {
  direction: string;
  amount: string;
  purpose: string;
  due_date: string;
  reason: string;
};

export function AdjustmentDialog({ account, open, onOpenChange }: DialogProps) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const adjust = useAdjust(account.uuid);
  const amount = useAmountSchema();
  const reason = useReasonSchema();
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<AdjustmentValues>
      open={open}
      onOpenChange={close}
      formId="finance-adjustment-form"
      title={t("finance.adjustment.title")}
      description={t("finance.adjustment.description")}
      submitLabel={t("finance.actions.adjust")}
      pending={adjust.isPending}
      problem={problem}
      schema={z.object({
        direction: z.string(),
        amount,
        purpose: z.string(),
        due_date: z.string(),
        reason,
      })}
      defaultValues={{
        direction: "debit",
        amount: "",
        purpose: "general",
        due_date: "",
        reason: "",
      }}
      onSubmit={async (v) => {
        try {
          await adjust.mutateAsync({
            direction: v.direction as "debit",
            amount: normalizeAmount(v.amount),
            purpose: v.purpose as "general",
            due_date: v.due_date || undefined,
            reason: v.reason.trim(),
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppSelect
          name="direction"
          label={t("finance.fields.direction")}
          options={[
            { value: "debit", label: t("finance.direction.debit") },
            { value: "credit", label: t("finance.direction.credit") },
          ]}
        />
        <AppInput
          name="amount"
          label={t("finance.fields.amount")}
          inputMode="decimal"
        />
        <AppSelect
          name="purpose"
          label={t("finance.fields.purpose")}
          options={(
            ["general", "rent", "charge", "prepayment", "deposit"] as const
          ).map((p) => ({ value: p, label: t(PURPOSE_LABEL_KEYS[p]) }))}
        />
        <AppDatePicker
          name="due_date"
          label={t("finance.fields.due_date")}
          description={t("finance.fields.due_date_hint")}
        />
      </div>
      <AppTextarea name="reason" label={t("finance.fields.reason")} />
    </FinanceFormDialog>
  );
}

type OffsetValues = { amount: string; purpose: string; reason: string };

export function OffsetDialog({ account, open, onOpenChange }: DialogProps) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const offset = useOffset(account.uuid);
  const amount = useAmountSchema();
  const reason = useReasonSchema();
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<OffsetValues>
      open={open}
      onOpenChange={close}
      formId="finance-offset-form"
      title={t("finance.offset.title")}
      description={t("finance.offset.description", {
        amount: amountText(
          account.balance.deposit_held,
          locale,
          account.currency,
        ),
      })}
      submitLabel={t("finance.actions.offset")}
      pending={offset.isPending}
      problem={problem}
      schema={z.object({ amount, purpose: z.string(), reason })}
      defaultValues={{
        amount: account.balance.deposit_held,
        purpose: "rent",
        reason: "",
      }}
      onSubmit={async (v) => {
        try {
          await offset.mutateAsync({
            amount: normalizeAmount(v.amount),
            purpose: v.purpose as "rent",
            reason: v.reason.trim(),
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="amount"
          label={t("finance.fields.amount")}
          inputMode="decimal"
        />
        <AppSelect
          name="purpose"
          label={t("finance.fields.offset_target")}
          options={(["rent", "charge", "general"] as const).map((p) => ({
            value: p,
            label: t(PURPOSE_LABEL_KEYS[p]),
          }))}
        />
      </div>
      <AppTextarea name="reason" label={t("finance.fields.reason")} />
    </FinanceFormDialog>
  );
}

export function ReverseEntryDialog({
  entry,
  onOpenChange,
}: {
  entry: LedgerEntry | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const reverse = useReverseEntry();
  const reason = useReasonSchema();
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<{ reason: string }>
      open={Boolean(entry)}
      onOpenChange={close}
      formId="finance-reverse-form"
      title={t("finance.reverse.title")}
      description={
        entry
          ? t("finance.reverse.description", {
              amount: amountText(entry.amount, locale, entry.currency),
            })
          : undefined
      }
      submitLabel={t("finance.actions.reverse")}
      destructive
      pending={reverse.isPending}
      problem={problem}
      schema={z.object({ reason })}
      defaultValues={{ reason: "" }}
      onSubmit={async (v) => {
        if (!entry) return;
        try {
          await reverse.mutateAsync({
            uuid: entry.uuid,
            reason: v.reason.trim(),
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <AppTextarea name="reason" label={t("finance.fields.reason")} />
    </FinanceFormDialog>
  );
}

export function SendStatementDialog({
  account,
  open,
  onOpenChange,
  canPayLink,
}: DialogProps & { canPayLink: boolean }) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const [sent, setSent] = useState<StatementSent | null>(null);
  const send = useSendStatement(account.uuid);
  const close = (next: boolean) => {
    if (!next) {
      setProblem(null);
      setSent(null);
    }
    onOpenChange(next);
  };
  const due = Number(account.balance.balance) > 0;
  return (
    <FinanceFormDialog<{ pay_link: boolean }>
      open={open}
      onOpenChange={close}
      formId="finance-statement-form"
      title={t("finance.statement_send.title")}
      description={t("finance.statement_send.description", {
        balance: amountText(account.balance.balance, locale, account.currency),
      })}
      submitLabel={t("finance.actions.send_statement")}
      pending={send.isPending}
      problem={problem}
      schema={z.object({ pay_link: z.boolean() })}
      defaultValues={{ pay_link: canPayLink && due }}
      onSubmit={async (v) => {
        try {
          const out = await send.mutateAsync({ pay_link: v.pay_link });
          setSent(out);
          setProblem(null);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.send_failed")));
        }
      }}
    >
      {canPayLink && due ? (
        <AppSwitch
          name="pay_link"
          label={t("finance.statement_send.pay_link")}
          description={t("finance.statement_send.pay_link_hint")}
        />
      ) : null}
      {sent ? (
        <div className="space-y-3">
          <Alert>
            <AlertDescription>
              {t("finance.statement_send.sent", { phone: sent.phone_masked })}
            </AlertDescription>
          </Alert>
          {sent.pay_link_url &&
          sent.pay_link_amount &&
          sent.pay_link_expires_at ? (
            <PaymentLinkBox
              url={sent.pay_link_url}
              amount={sent.pay_link_amount}
              expiresAt={sent.pay_link_expires_at}
            />
          ) : null}
        </div>
      ) : null}
    </FinanceFormDialog>
  );
}
