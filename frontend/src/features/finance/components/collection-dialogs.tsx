"use client";

import { useMemo, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import {
  AppDatePicker,
  AppInput,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  OpenItemsAllocator,
  manualPlan,
  type AllocationMode,
} from "@/features/finance/components/open-items-allocator";
import {
  useAllocateCollection,
  useCashAccountsList,
  useOpenItems,
  useRefundCollection,
  useReleaseAllocation,
  useVoidCollection,
} from "@/features/finance/hooks/use-finance";
import {
  amountText,
  centsOf,
  errorText,
  fromCents,
  isPositiveAmount,
  normalizeAmount,
  todayISO,
} from "@/features/finance/lib/format";
import type {
  FinanceCollection,
  FinanceCollectionAllocation,
} from "@/features/finance/types";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

type Props = {
  collection: FinanceCollection;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function useReason() {
  const { t } = useLocale();
  return z
    .string()
    .trim()
    .min(3, t("finance.validation.reason"))
    .max(500, t("finance.validation.reason"));
}

/** Allocate what is left of a collection (its credit) to open items. */
export function AllocateDialog({ collection, open, onOpenChange }: Props) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const [manual, setManual] = useState<Record<string, string>>({});
  const [key, setKey] = useState(() => crypto.randomUUID());
  const allocate = useAllocateCollection(collection.uuid);
  const items = useOpenItems(collection.account.uuid, undefined, true, open);
  const credit = centsOf(collection.unallocated_amount);
  const close = (next: boolean) => {
    if (!next) {
      setProblem(null);
      setManual({});
      setKey(crypto.randomUUID());
    }
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<{ mode: string }>
      open={open}
      onOpenChange={close}
      formId="finance-allocate-form"
      title={t("finance.allocate.title")}
      description={t("finance.allocate.description", {
        amount: amountText(
          collection.unallocated_amount,
          locale,
          collection.currency,
        ),
      })}
      submitLabel={t("finance.allocate.submit")}
      pending={allocate.isPending}
      problem={problem}
      schema={z.object({ mode: z.string() })}
      className="max-h-[92dvh] overflow-y-auto sm:max-w-3xl"
      defaultValues={{ mode: "auto" }}
      onSubmit={async (v) => {
        const mode = v.mode as AllocationMode;
        const plan = manualPlan(manual);
        if (mode === "manual") {
          const sum = Object.values(plan).reduce((a, b) => a + b, 0);
          if (sum === 0) {
            setProblem(t("finance.collect.manual_required"));
            return;
          }
          if (sum > credit) {
            setProblem(t("finance.collect.over_allocated"));
            return;
          }
        }
        try {
          await allocate.mutateAsync({
            mode: mode === "manual" ? "manual" : "auto",
            items:
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
      <AllocateFields
        collection={collection}
        manual={manual}
        onManualChange={setManual}
        loading={items.isLoading}
        items={items.data?.items ?? []}
      />
    </FinanceFormDialog>
  );
}

function AllocateFields({
  collection,
  manual,
  onManualChange,
  loading,
  items,
}: {
  collection: FinanceCollection;
  manual: Record<string, string>;
  onManualChange: (next: Record<string, string>) => void;
  loading: boolean;
  items: NonNullable<ReturnType<typeof useOpenItems>["data"]>["items"];
}) {
  const { t } = useLocale();
  const { control } = useFormContext<{ mode: string }>();
  const mode = useWatch({ control, name: "mode" }) as AllocationMode;
  return (
    <div className="space-y-4">
      <AppToggleGroup
        name="mode"
        label={t("finance.allocate.mode")}
        options={[
          { value: "auto", label: t("finance.collect.mode_auto") },
          { value: "manual", label: t("finance.collect.mode_manual") },
        ]}
      />
      <OpenItemsAllocator
        items={items}
        currency={collection.currency}
        mode={mode}
        amountCents={centsOf(collection.unallocated_amount)}
        manual={manual}
        onManualChange={onManualChange}
        isLoading={loading}
        persistKey="tenant-finance-allocate-items-v1"
      />
    </div>
  );
}

type RefundValues = {
  amount: string;
  reason: string;
  cash_account_uuid: string;
  refund_date: string;
};

/**
 * Refund a collection: online ones through the provider (the result decides,
 * nothing is booked when it fails), manual ones out of the till / bank
 * account.
 */
export function RefundCollectionDialog({
  collection,
  open,
  onOpenChange,
}: Props) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const refund = useRefundCollection(collection.uuid);
  const reason = useReason();
  const online = Boolean(collection.payment_uuid);
  const accounts = useCashAccountsList(
    { limit: 100, offset: 0, status: "active", sort: "code" },
    open && !online,
  );
  const options = useMemo(
    () =>
      (accounts.data?.items ?? []).map((a) => ({
        value: a.uuid,
        label: `${a.code} · ${a.name}`,
      })),
    [accounts.data?.items],
  );
  const max = centsOf(collection.refundable);
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<RefundValues>
      open={open}
      onOpenChange={close}
      formId="finance-collection-refund-form"
      title={t("finance.collection_refund.title")}
      description={
        online
          ? t("finance.collection_refund.online_description")
          : t("finance.collection_refund.description")
      }
      submitLabel={t("finance.actions.refund")}
      pending={refund.isPending}
      problem={problem}
      destructive
      schema={z.object({
        amount: z
          .string()
          .trim()
          .refine(isPositiveAmount, t("finance.validation.amount"))
          .refine(
            (v) => centsOf(normalizeAmount(v)) <= max,
            t("finance.collection_refund.too_much", {
              amount: amountText(
                collection.refundable,
                locale,
                collection.currency,
              ),
            }),
          ),
        reason,
        cash_account_uuid: z.string(),
        refund_date: z.string(),
      })}
      defaultValues={{
        amount: fromCents(max).replace(".", ","),
        reason: "",
        cash_account_uuid: collection.cash_account?.uuid ?? "",
        refund_date: todayISO(),
      }}
      onSubmit={async (v) => {
        try {
          await refund.mutateAsync({
            amount: normalizeAmount(v.amount),
            reason: v.reason.trim(),
            cash_account_uuid:
              !online && v.cash_account_uuid ? v.cash_account_uuid : undefined,
            refund_date: !online ? v.refund_date || undefined : undefined,
          });
          close(false);
        } catch (error) {
          setProblem(
            error instanceof ApiError && error.status === 502
              ? t("finance.collection_refund.provider_failed")
              : errorText(error, t("finance.errors.save_failed")),
          );
        }
      }}
    >
      {Number(collection.allocated_amount) > 0 ? (
        <Alert>
          <AlertDescription>
            {t("finance.collection_refund.allocation_hint")}
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="amount"
          label={t("finance.fields.amount")}
          inputMode="decimal"
        />
        {!online ? (
          <AppDatePicker name="refund_date" label={t("finance.fields.date")} />
        ) : null}
      </div>
      {!online ? (
        <AppSelect
          name="cash_account_uuid"
          label={t("finance.collection_refund.cash_account")}
          placeholder={t("finance.collect.choose_account")}
          options={options}
        />
      ) : null}
      <AppTextarea name="reason" label={t("finance.fields.reason")} />
    </FinanceFormDialog>
  );
}

/** Void a manual collection recorded by mistake. */
export function VoidCollectionDialog({
  collection,
  open,
  onOpenChange,
}: Props) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const voidIt = useVoidCollection(collection.uuid);
  const reason = useReason();
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<{ reason: string }>
      open={open}
      onOpenChange={close}
      formId="finance-collection-void-form"
      title={t("finance.collection_void.title")}
      description={t("finance.collection_void.description")}
      submitLabel={t("finance.actions.void")}
      pending={voidIt.isPending}
      problem={problem}
      destructive
      schema={z.object({ reason })}
      defaultValues={{ reason: "" }}
      onSubmit={async (v) => {
        try {
          await voidIt.mutateAsync({ reason: v.reason.trim() });
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

/** Take the collection's allocation off one open item. */
export function ReleaseAllocationDialog({
  collection,
  allocation,
  onOpenChange,
}: {
  collection: FinanceCollection;
  allocation: FinanceCollectionAllocation | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const release = useReleaseAllocation(collection.uuid);
  const reason = useReason();
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<{ reason: string }>
      open={allocation !== null}
      onOpenChange={close}
      formId="finance-release-form"
      title={t("finance.release.title")}
      description={
        allocation
          ? t("finance.release.description", {
              amount: amountText(
                allocation.amount,
                locale,
                collection.currency,
              ),
            })
          : undefined
      }
      submitLabel={t("finance.actions.release")}
      pending={release.isPending}
      problem={problem}
      schema={z.object({ reason })}
      defaultValues={{ reason: "" }}
      onSubmit={async (v) => {
        if (!allocation) return;
        try {
          await release.mutateAsync({
            entry_uuid: allocation.entry_uuid,
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
