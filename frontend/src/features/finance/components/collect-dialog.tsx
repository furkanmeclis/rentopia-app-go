"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import {
  AppDatePicker,
  AppInput,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  OpenItemsAllocator,
  manualPlan,
  type AllocationMode,
} from "@/features/finance/components/open-items-allocator";
import {
  BRANCH_METHODS,
  METHOD_LABEL_KEYS,
  METHOD_REFERENCE_LABEL_KEYS,
} from "@/features/finance/constants";
import {
  useCashAccountsList,
  useCreateCollection,
  useOpenItems,
} from "@/features/finance/hooks/use-finance";
import {
  centsOf,
  errorText,
  fromCents,
  isPositiveAmount,
  normalizeAmount,
  todayISO,
} from "@/features/finance/lib/format";
import type {
  CashAccount,
  CollectionMethod,
  FinanceOpenItem,
} from "@/features/finance/types";
import { useLocale } from "@/providers/locale-provider";

const DEFAULT = "default";

type Values = {
  method: string;
  amount: string;
  collection_date: string;
  cash_account_uuid: string;
  branch_uuid: string;
  reference_no: string;
  payer_name: string;
  note: string;
  allocation_mode: string;
};

type CollectDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The customer account (omit for a rental without an account yet). */
  accountUuid?: string;
  /** Narrows the open items to one rental and links the collection. */
  rentalUuid?: string;
  /** Shown under the title (account / rental reference). */
  subtitle?: string;
  payerName?: string;
};

/** The till for cash, a bank / clearing account for transfers and POS. */
function accountsFor(method: string, accounts: CashAccount[]): CashAccount[] {
  if (method === "cash") return accounts.filter((a) => a.kind === "cash");
  if (method === "other") return accounts;
  return accounts.filter((a) => a.kind !== "cash");
}

/**
 * "Tahsilat al": money taken at the branch (cash, bank transfer / EFT,
 * physical POS) for a customer account or a rental. Shows the open items
 * (rent, charges) with an oldest-first preview or amounts per item, and the
 * method-specific fields (till / bank account, receipt / transfer / slip
 * reference). What is not allocated stays the customer's credit.
 */
export function CollectDialog({
  open,
  onOpenChange,
  accountUuid,
  rentalUuid,
  subtitle,
  payerName,
}: CollectDialogProps) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const [manual, setManual] = useState<Record<string, string>>({});
  // One key per opened dialog: a double submit records one collection.
  const [key, setKey] = useState(() => crypto.randomUUID());
  const create = useCreateCollection();
  const openItems = useOpenItems(accountUuid, rentalUuid, true, open);
  const items = useMemo(
    () => openItems.data?.items ?? [],
    [openItems.data?.items],
  );
  const currency = openItems.data?.currency ?? "TRY";
  const openTotal = items.reduce((sum, it) => sum + centsOf(it.open), 0);
  const accounts = useCashAccountsList(
    { limit: 100, offset: 0, status: "active", sort: "code" },
    open,
  );
  const branches = useBranchesList(
    { limit: 100, offset: 0, sort: "code" },
    open,
  );

  const schema = z
    .object({
      method: z.string(),
      amount: z
        .string()
        .trim()
        .refine(isPositiveAmount, t("finance.validation.amount")),
      collection_date: z.string(),
      cash_account_uuid: z.string(),
      branch_uuid: z.string(),
      reference_no: z.string().max(64),
      payer_name: z.string().max(255),
      note: z.string().max(1000),
      allocation_mode: z.string(),
    })
    .refine((v) => v.method === "other" || v.cash_account_uuid !== "", {
      path: ["cash_account_uuid"],
      message: t("finance.collect.cash_account_required"),
    });

  const close = (next: boolean) => {
    if (!next) {
      setProblem(null);
      setManual({});
      setKey(crypto.randomUUID());
    }
    onOpenChange(next);
  };

  return (
    <FinanceFormDialog<Values>
      open={open}
      onOpenChange={close}
      formId="finance-collect-form"
      title={t("finance.collect.title")}
      description={subtitle ?? t("finance.collect.description")}
      submitLabel={t("finance.collect.submit")}
      pending={create.isPending}
      problem={problem}
      schema={schema}
      className="max-h-[92dvh] overflow-y-auto sm:max-w-3xl"
      defaultValues={{
        method: "cash",
        amount: openTotal > 0 ? fromCents(openTotal).replace(".", ",") : "",
        collection_date: todayISO(),
        cash_account_uuid: "",
        branch_uuid: DEFAULT,
        reference_no: "",
        payer_name: payerName ?? "",
        note: "",
        allocation_mode: items.length > 0 ? "auto" : "none",
      }}
      onSubmit={async (v) => {
        const amount = normalizeAmount(v.amount);
        const mode = v.allocation_mode as AllocationMode;
        const plan = mode === "manual" ? manualPlan(manual) : {};
        if (mode === "manual") {
          const sum = Object.values(plan).reduce((a, b) => a + b, 0);
          if (sum === 0) {
            setProblem(t("finance.collect.manual_required"));
            return;
          }
          if (sum > centsOf(amount)) {
            setProblem(t("finance.collect.over_allocated"));
            return;
          }
        }
        try {
          await create.mutateAsync({
            account_uuid: accountUuid,
            rental_uuid: rentalUuid,
            amount,
            method: v.method as "cash",
            collection_date: v.collection_date || undefined,
            cash_account_uuid: v.cash_account_uuid || undefined,
            branch_uuid: v.branch_uuid !== DEFAULT ? v.branch_uuid : undefined,
            reference_no: v.reference_no.trim() || undefined,
            payer_name: v.payer_name.trim() || undefined,
            note: v.note.trim() || undefined,
            allocation_mode: mode,
            allocations:
              mode === "manual"
                ? Object.entries(plan)
                    .filter(([, cents]) => cents > 0)
                    .map(([entry_uuid, cents]) => ({
                      entry_uuid,
                      amount: fromCents(cents),
                    }))
                : undefined,
            idempotency_key: key,
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <CollectFields
        items={items}
        currency={currency}
        loadingItems={openItems.isLoading}
        accounts={accounts.data?.items ?? []}
        branches={(branches.data?.items ?? []).map((b) => ({
          value: b.uuid,
          label: `${b.code} · ${b.name}`,
        }))}
        manual={manual}
        onManualChange={setManual}
        hasAccount={Boolean(accountUuid)}
      />
    </FinanceFormDialog>
  );
}

function CollectFields({
  items,
  currency,
  loadingItems,
  accounts,
  branches,
  manual,
  onManualChange,
  hasAccount,
}: {
  items: FinanceOpenItem[];
  currency: string;
  loadingItems: boolean;
  accounts: CashAccount[];
  branches: { value: string; label: string }[];
  manual: Record<string, string>;
  onManualChange: (next: Record<string, string>) => void;
  hasAccount: boolean;
}) {
  const { t } = useLocale();
  const { control, setValue, getValues } = useFormContext<Values>();
  const method = useWatch({ control, name: "method" }) as CollectionMethod;
  const mode = useWatch({ control, name: "allocation_mode" }) as AllocationMode;
  const amount = useWatch({ control, name: "amount" });
  const cashAccount = useWatch({ control, name: "cash_account_uuid" });
  const amountCents = centsOf(normalizeAmount(amount ?? ""));
  const choices = useMemo(
    () => accountsFor(method, accounts),
    [accounts, method],
  );

  // Keep the chosen account valid for the method (one choice: preselect).
  useEffect(() => {
    if (cashAccount && !choices.some((a) => a.uuid === cashAccount)) {
      setValue("cash_account_uuid", "");
    } else if (!cashAccount && choices.length === 1 && method !== "other") {
      setValue("cash_account_uuid", choices[0].uuid);
    }
  }, [cashAccount, choices, method, setValue]);

  // The open items arrive after the form opened: suggest their total.
  const suggested = useRef(false);
  useEffect(() => {
    if (suggested.current || items.length === 0) return;
    suggested.current = true;
    const open = items.reduce((sum, it) => sum + centsOf(it.open), 0);
    if (!getValues("amount") && open > 0) {
      setValue("amount", fromCents(open).replace(".", ","));
    }
    setValue("allocation_mode", "auto");
  }, [getValues, items, setValue]);

  return (
    <div className="space-y-5">
      <AppToggleGroup
        name="method"
        label={t("finance.fields.method")}
        options={BRANCH_METHODS.map((m) => ({
          value: m,
          label: t(METHOD_LABEL_KEYS[m]),
        }))}
        className="[&_[role=group]]:flex-wrap"
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <AppInput
          name="amount"
          label={t("finance.fields.amount")}
          inputMode="decimal"
        />
        <AppDatePicker
          name="collection_date"
          label={t("finance.fields.date")}
        />
        <AppSelect
          name="cash_account_uuid"
          label={
            method === "cash"
              ? t("finance.collect.till")
              : t("finance.collect.bank_account")
          }
          placeholder={t("finance.collect.choose_account")}
          description={
            choices.length === 0
              ? t("finance.collect.no_cash_accounts")
              : undefined
          }
          options={choices.map((a) => ({
            value: a.uuid,
            label: `${a.code} · ${a.name}`,
          }))}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <AppInput
          name="reference_no"
          label={t(
            METHOD_REFERENCE_LABEL_KEYS[method] ??
              METHOD_REFERENCE_LABEL_KEYS.other,
          )}
        />
        <AppInput name="payer_name" label={t("finance.fields.payer_name")} />
        <AppSelect
          name="branch_uuid"
          label={t("finance.fields.branch")}
          options={[
            { value: DEFAULT, label: t("finance.collect.default_branch") },
            ...branches,
          ]}
        />
      </div>
      <AppTextarea name="note" label={t("finance.fields.note")} />

      <div className="space-y-3 border-t pt-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold">
              {t("finance.collect.open_items")}
            </h3>
            <p className="text-muted-foreground text-xs">
              {hasAccount
                ? t("finance.collect.open_items_hint")
                : t("finance.collect.no_account_hint")}
            </p>
          </div>
          <AppToggleGroup
            name="allocation_mode"
            options={[
              { value: "auto", label: t("finance.collect.mode_auto") },
              { value: "manual", label: t("finance.collect.mode_manual") },
              { value: "none", label: t("finance.collect.mode_none") },
            ]}
            className="[&_[role=group]]:flex-wrap"
          />
        </div>
        <OpenItemsAllocator
          items={items}
          currency={currency}
          mode={mode}
          amountCents={amountCents}
          manual={manual}
          onManualChange={onManualChange}
          isLoading={hasAccount && loadingItems}
          persistKey="tenant-finance-collect-items-v1"
        />
      </div>
    </div>
  );
}
