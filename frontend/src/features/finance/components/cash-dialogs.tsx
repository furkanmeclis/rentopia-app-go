"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import {
  AppDatePicker,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  CASH_KIND_LABEL_KEYS,
  MANUAL_MOVEMENT_KINDS,
  MOVEMENT_KIND_LABEL_KEYS,
} from "@/features/finance/constants";
import {
  useCashAccountsList,
  useCategories,
  useRecordMovement,
  useReverseMovement,
  useSaveCashAccount,
  useTransfer,
} from "@/features/finance/hooks/use-finance";
import {
  amountText,
  errorText,
  isPositiveAmount,
  normalizeAmount,
  todayISO,
} from "@/features/finance/lib/format";
import type { CashAccount, CashMovement } from "@/features/finance/types";
import { useLocale } from "@/providers/locale-provider";

const NONE = "none";

function useBranchOptions(enabled: boolean, withHeadOffice: boolean) {
  const { t } = useLocale();
  const branches = useBranchesList(
    { limit: 100, offset: 0, sort: "code" },
    enabled,
  );
  return useMemo(
    () => [
      ...(withHeadOffice
        ? [{ value: NONE, label: t("finance.head_office") }]
        : []),
      ...(branches.data?.items ?? []).map((b) => ({
        value: b.uuid,
        label: `${b.code} · ${b.name}`,
      })),
    ],
    [branches.data?.items, t, withHeadOffice],
  );
}

export function useCategoryOptions(enabled: boolean, withNone: boolean) {
  const { t } = useLocale();
  const categories = useCategories(true, enabled);
  return useMemo(
    () => [
      ...(withNone
        ? [{ value: NONE, label: t("finance.fields.no_category") }]
        : []),
      ...(categories.data?.items ?? []).map((c) => ({
        value: c.uuid,
        label: c.name,
      })),
    ],
    [categories.data?.items, t, withNone],
  );
}

type CashAccountValues = {
  code: string;
  name: string;
  kind: string;
  branch_uuid: string;
  opening_balance: string;
  opening_date: string;
  bank_name: string;
  iban: string;
  note: string;
};

/** Create / edit a till or a bank account. */
export function CashAccountDialog({
  account,
  open,
  onOpenChange,
}: {
  account: CashAccount | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const save = useSaveCashAccount();
  const branches = useBranchOptions(open, true);
  const editing = Boolean(account);
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<CashAccountValues>
      open={open}
      onOpenChange={close}
      formId="finance-cash-account-form"
      title={
        editing ? t("finance.cash.edit_title") : t("finance.cash.create_title")
      }
      description={t("finance.cash.form_description")}
      submitLabel={t("form.save")}
      pending={save.isPending}
      problem={problem}
      schema={z.object({
        code: z
          .string()
          .trim()
          .regex(
            /^[A-Za-z0-9][A-Za-z0-9_.-]{0,31}$/,
            t("finance.validation.code"),
          ),
        name: z
          .string()
          .trim()
          .min(1, t("finance.validation.required"))
          .max(128),
        kind: z.string(),
        branch_uuid: z.string(),
        opening_balance: z
          .string()
          .trim()
          .refine(
            (v) =>
              v === "" || /^-?\d{1,12}([.,]\d{1,2})?$/.test(normalizeAmount(v)),
            t("finance.validation.amount"),
          ),
        opening_date: z.string(),
        bank_name: z.string().max(128),
        iban: z.string().max(42),
        note: z.string().max(2000),
      })}
      defaultValues={{
        code: account?.code ?? "",
        name: account?.name ?? "",
        kind: account?.kind ?? "cash",
        branch_uuid: account?.branch?.uuid ?? NONE,
        opening_balance: account?.opening_balance ?? "0",
        opening_date: account?.opening_date ?? todayISO(),
        bank_name: account?.bank_name ?? "",
        iban: account?.iban ?? "",
        note: account?.note ?? "",
      }}
      onSubmit={async (v) => {
        const branch = v.branch_uuid !== NONE ? v.branch_uuid : undefined;
        try {
          await save.mutateAsync({
            uuid: account?.uuid,
            body: {
              code: v.code.trim(),
              name: v.name.trim(),
              kind: editing ? undefined : (v.kind as "cash"),
              branch_uuid: branch,
              clear_branch: editing && !branch ? true : undefined,
              opening_balance:
                editing && account && account.movement_count > 0
                  ? undefined
                  : normalizeAmount(v.opening_balance) || "0",
              opening_date:
                editing && account && account.movement_count > 0
                  ? undefined
                  : v.opening_date || undefined,
              bank_name: v.bank_name.trim(),
              iban: v.iban.trim(),
              note: v.note.trim(),
            },
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput name="code" label={t("finance.fields.code")} />
        <AppInput name="name" label={t("finance.fields.name")} />
        <AppSelect
          name="kind"
          label={t("finance.fields.kind")}
          disabled={editing}
          options={(["cash", "bank"] as const).map((k) => ({
            value: k,
            label: t(CASH_KIND_LABEL_KEYS[k]),
          }))}
        />
        <AppSelect
          name="branch_uuid"
          label={t("finance.fields.branch")}
          options={branches}
        />
        <AppInput
          name="opening_balance"
          label={t("finance.fields.opening_balance")}
          inputMode="decimal"
          disabled={Boolean(account && account.movement_count > 0)}
        />
        <AppDatePicker
          name="opening_date"
          label={t("finance.fields.opening_date")}
          disabled={Boolean(account && account.movement_count > 0)}
        />
        <AppInput name="bank_name" label={t("finance.fields.bank_name")} />
        <AppInput name="iban" label={t("finance.fields.iban")} />
      </div>
      <AppTextarea name="note" label={t("finance.fields.note")} />
    </FinanceFormDialog>
  );
}

type TransferValues = {
  from: string;
  to: string;
  amount: string;
  movement_date: string;
  description: string;
};

/** Transfer between two accounts (both movements in one transaction). */
export function TransferDialog({
  from,
  open,
  onOpenChange,
}: {
  from?: CashAccount | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const transfer = useTransfer();
  const accounts = useCashAccountsList(
    { limit: 100, offset: 0, status: "active", sort: "code" },
    open,
  );
  const options = (accounts.data?.items ?? []).map((a) => ({
    value: a.uuid,
    label: `${a.code} · ${a.name} (${amountText(a.balance, locale, a.currency)})`,
  }));
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<TransferValues>
      open={open}
      onOpenChange={close}
      formId="finance-transfer-form"
      title={t("finance.transfer.title")}
      description={t("finance.transfer.description")}
      submitLabel={t("finance.actions.transfer")}
      pending={transfer.isPending}
      problem={problem}
      schema={z
        .object({
          from: z.string().min(1, t("finance.validation.required")),
          to: z.string().min(1, t("finance.validation.required")),
          amount: z
            .string()
            .trim()
            .refine(isPositiveAmount, t("finance.validation.amount")),
          movement_date: z.string(),
          description: z.string().max(500),
        })
        .refine((v) => v.from !== v.to, {
          path: ["to"],
          message: t("finance.validation.same_account"),
        })}
      defaultValues={{
        from: from?.uuid ?? "",
        to: "",
        amount: "",
        movement_date: todayISO(),
        description: "",
      }}
      onSubmit={async (v) => {
        try {
          await transfer.mutateAsync({
            from_account_uuid: v.from,
            to_account_uuid: v.to,
            amount: normalizeAmount(v.amount),
            movement_date: v.movement_date || undefined,
            description: v.description.trim() || undefined,
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <AppSelect
        name="from"
        label={t("finance.fields.from_account")}
        placeholder={t("finance.fields.choose_account")}
        options={options}
      />
      <AppSelect
        name="to"
        label={t("finance.fields.to_account")}
        placeholder={t("finance.fields.choose_account")}
        options={options}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="amount"
          label={t("finance.fields.amount")}
          inputMode="decimal"
        />
        <AppDatePicker name="movement_date" label={t("finance.fields.date")} />
      </div>
      <AppTextarea name="description" label={t("finance.fields.description")} />
    </FinanceFormDialog>
  );
}

type MovementValues = {
  kind: string;
  direction: string;
  amount: string;
  movement_date: string;
  category_uuid: string;
  description: string;
  reason: string;
};

/** Record a collection deposit, refund, supplier payment or adjustment. */
export function MovementDialog({
  account,
  open,
  onOpenChange,
}: {
  account: CashAccount;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const record = useRecordMovement();
  const categories = useCategoryOptions(open, true);
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<MovementValues>
      open={open}
      onOpenChange={close}
      formId="finance-movement-form"
      title={t("finance.movement.title")}
      description={t("finance.movement.description", { account: account.name })}
      submitLabel={t("form.save")}
      pending={record.isPending}
      problem={problem}
      schema={z
        .object({
          kind: z.string(),
          direction: z.string(),
          amount: z
            .string()
            .trim()
            .refine(isPositiveAmount, t("finance.validation.amount")),
          movement_date: z.string(),
          category_uuid: z.string(),
          description: z.string().max(500),
          reason: z.string().max(500),
        })
        .refine((v) => v.kind !== "adjustment" || v.reason.trim().length >= 3, {
          path: ["reason"],
          message: t("finance.validation.reason"),
        })}
      defaultValues={{
        kind: "collection",
        direction: "in",
        amount: "",
        movement_date: todayISO(),
        category_uuid: NONE,
        description: "",
        reason: "",
      }}
      onSubmit={async (v) => {
        try {
          await record.mutateAsync({
            uuid: account.uuid,
            body: {
              kind: v.kind as "collection",
              direction:
                v.kind === "adjustment" ? (v.direction as "in") : undefined,
              amount: normalizeAmount(v.amount),
              movement_date: v.movement_date || undefined,
              category_uuid:
                v.category_uuid !== NONE ? v.category_uuid : undefined,
              description: v.description.trim() || undefined,
              reason: v.reason.trim() || undefined,
            },
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppSelect
          name="kind"
          label={t("finance.fields.movement_kind")}
          options={MANUAL_MOVEMENT_KINDS.map((k) => ({
            value: k,
            label: t(MOVEMENT_KIND_LABEL_KEYS[k]),
          }))}
        />
        <AppSelect
          name="direction"
          label={t("finance.fields.direction_adjustment")}
          options={[
            { value: "in", label: t("finance.direction.in") },
            { value: "out", label: t("finance.direction.out") },
          ]}
        />
        <AppInput
          name="amount"
          label={t("finance.fields.amount")}
          inputMode="decimal"
        />
        <AppDatePicker name="movement_date" label={t("finance.fields.date")} />
      </div>
      <AppSelect
        name="category_uuid"
        label={t("finance.fields.category")}
        options={categories}
      />
      <AppInput name="description" label={t("finance.fields.description")} />
      <AppTextarea
        name="reason"
        label={t("finance.fields.reason")}
        description={t("finance.fields.reason_adjustment_hint")}
      />
    </FinanceFormDialog>
  );
}

export function ReverseMovementDialog({
  movement,
  onOpenChange,
}: {
  movement: CashMovement | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const reverse = useReverseMovement();
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<{ reason: string }>
      open={Boolean(movement)}
      onOpenChange={close}
      formId="finance-reverse-movement-form"
      title={t("finance.reverse_movement.title")}
      description={
        movement
          ? t("finance.reverse_movement.description", {
              amount: amountText(movement.amount, locale, movement.currency),
            })
          : undefined
      }
      submitLabel={t("finance.actions.reverse")}
      destructive
      pending={reverse.isPending}
      problem={problem}
      schema={z.object({
        reason: z
          .string()
          .trim()
          .min(3, t("finance.validation.reason"))
          .max(500),
      })}
      defaultValues={{ reason: "" }}
      onSubmit={async (v) => {
        if (!movement) return;
        try {
          await reverse.mutateAsync({
            uuid: movement.uuid,
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
