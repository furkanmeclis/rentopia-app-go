"use client";

import { useEffect, useMemo, useState } from "react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import {
  AppCombobox,
  AppDatePicker,
  AppInput,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Separator } from "@/components/ui/separator";
import { Amount } from "@/features/finance/components/finance-cells";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import { useCashAccountsList } from "@/features/finance/hooks/use-finance";
import {
  amountText,
  errorText,
  isPositiveAmount,
  normalizeAmount,
  todayISO,
} from "@/features/finance/lib/format";
import { InvoiceDueChip } from "@/features/suppliers/components/supplier-cells";
import {
  PAYMENT_METHODS,
  PAYMENT_METHOD_LABEL_KEYS,
} from "@/features/suppliers/constants";
import {
  useCreatePayment,
  useSupplierInvoices,
  useSupplierOptions,
  useVoidPayment,
} from "@/features/suppliers/hooks/use-suppliers";
import type {
  SupplierInvoice,
  SupplierPayment,
} from "@/features/suppliers/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type AllocationValues = { invoice_uuid: string; amount: string };

type PaymentValues = {
  supplier_uuid: string;
  cash_account_uuid: string;
  amount: string;
  payment_date: string;
  method: string;
  reference: string;
  description: string;
  allocation_mode: string;
  allocations: AllocationValues[];
};

/**
 * Manual allocation: one amount per open invoice of the supplier (empty =
 * not paid by this payment).
 */
function AllocationsField({ invoices }: { invoices: SupplierInvoice[] }) {
  const { t, locale } = useLocale();
  const { control } = useFormContext<PaymentValues>();
  const { fields, replace } = useFieldArray({ control, name: "allocations" });
  const [allocations, amount] = useWatch({
    control,
    name: ["allocations", "amount"],
  });
  const key = invoices.map((i) => i.uuid).join(",");
  useEffect(() => {
    replace(invoices.map((i) => ({ invoice_uuid: i.uuid, amount: "" })));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
  const allocated = (allocations ?? []).reduce(
    (sum, a) => sum + (Number(normalizeAmount(a.amount ?? "")) || 0),
    0,
  );
  const paid = Number(normalizeAmount(amount ?? "")) || 0;
  if (invoices.length === 0) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("suppliers.payments.no_open_invoices")}
      </p>
    );
  }
  return (
    <div className="space-y-2">
      <ul className="divide-y rounded-md border">
        {fields.map((field, index) => {
          const inv = invoices.find((i) => i.uuid === field.invoice_uuid);
          if (!inv) return null;
          return (
            <li
              key={field.id}
              className="grid gap-2 px-3 py-2 sm:grid-cols-[1fr_9rem] sm:items-center"
            >
              <div className="min-w-0 space-y-0.5">
                <div className="flex flex-wrap items-center gap-2 text-sm font-medium">
                  {inv.invoice_no}
                  <InvoiceDueChip state={inv.due_state} />
                </div>
                <div className="text-muted-foreground text-xs">
                  {t("suppliers.payments.invoice_line", {
                    due: date(inv.due_date, "dd.MM.yyyy", locale),
                    open: amountText(inv.open_amount, locale, inv.currency),
                  })}
                </div>
              </div>
              <AppInput
                name={`allocations.${index}.amount`}
                inputMode="decimal"
                placeholder="0,00"
                aria-label={t("suppliers.payments.allocate_to", {
                  number: inv.invoice_no,
                })}
              />
            </li>
          );
        })}
      </ul>
      <div className="text-muted-foreground flex flex-wrap justify-between gap-2 text-xs">
        <span>
          {t("suppliers.payments.allocated", {
            amount: amountText(allocated.toFixed(2), locale),
          })}
        </span>
        <span className={allocated > paid ? "text-destructive" : undefined}>
          {t("suppliers.payments.advance", {
            amount: amountText(
              Math.max(0, paid - allocated).toFixed(2),
              locale,
            ),
          })}
        </span>
      </div>
    </div>
  );
}

function PaymentFields({
  fixedSupplier,
  suppliers,
  cashOptions,
}: {
  fixedSupplier?: string;
  suppliers: { value: string; label: string }[];
  cashOptions: { value: string; label: string }[];
}) {
  const { t } = useLocale();
  const { control } = useFormContext<PaymentValues>();
  const [supplier, mode] = useWatch({
    control,
    name: ["supplier_uuid", "allocation_mode"],
  });
  const open = useSupplierInvoices(
    {
      limit: 100,
      offset: 0,
      sort: "due_date",
      supplier: supplier || undefined,
      status: "open,partially_paid",
    },
    Boolean(supplier),
  );
  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2">
        {!fixedSupplier ? (
          <AppCombobox
            name="supplier_uuid"
            label={t("suppliers.fields.supplier")}
            placeholder={t("suppliers.fields.choose_supplier")}
            options={suppliers}
            className="sm:col-span-2"
          />
        ) : null}
        <AppSelect
          name="cash_account_uuid"
          label={t("suppliers.fields.paid_from")}
          placeholder={t("suppliers.fields.choose_cash_account")}
          options={cashOptions}
          className="sm:col-span-2"
        />
        <AppInput
          name="amount"
          label={t("suppliers.fields.amount")}
          inputMode="decimal"
        />
        <AppDatePicker
          name="payment_date"
          label={t("suppliers.fields.payment_date")}
        />
        <AppSelect
          name="method"
          label={t("suppliers.fields.method")}
          options={PAYMENT_METHODS.map((m) => ({
            value: m,
            label: t(PAYMENT_METHOD_LABEL_KEYS[m]),
          }))}
        />
        <AppInput name="reference" label={t("suppliers.fields.reference")} />
      </div>
      <Separator />
      <AppToggleGroup
        name="allocation_mode"
        label={t("suppliers.payments.allocation")}
        description={
          mode === "auto"
            ? t("suppliers.payments.auto_hint")
            : t("suppliers.payments.manual_hint")
        }
        options={[
          { value: "auto", label: t("suppliers.payments.mode_auto") },
          { value: "manual", label: t("suppliers.payments.mode_manual") },
        ]}
      />
      {mode === "manual" ? (
        <AllocationsField invoices={open.data?.items ?? []} />
      ) : null}
      <AppTextarea
        name="description"
        label={t("suppliers.fields.description")}
      />
    </>
  );
}

/**
 * Pay a supplier from a till or a bank account. In one transaction the API
 * writes the cash movement, the ledger entry and the allocations; without
 * manual amounts the open invoices are paid oldest due date first and the
 * rest stays as an advance.
 */
export function PaymentDialog({
  supplierUuid,
  invoice,
  open,
  onOpenChange,
}: {
  supplierUuid?: string;
  /** Pay this invoice (prefills supplier, amount and a manual allocation). */
  invoice?: SupplierInvoice | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const create = useCreatePayment();
  const fixedSupplier = invoice?.supplier.uuid ?? supplierUuid;
  const suppliers = useSupplierOptions(open && !fixedSupplier);
  const cash = useCashAccountsList(
    { limit: 100, offset: 0, status: "active", sort: "code" },
    open,
  );
  const cashOptions = useMemo(
    () =>
      (cash.data?.items ?? []).map((a) => ({
        value: a.uuid,
        label: `${a.code} · ${a.name} (${amountText(a.balance, locale, a.currency)})`,
      })),
    [cash.data?.items, locale],
  );
  const schema = useMemo(
    () =>
      z
        .object({
          supplier_uuid: z.string().min(1, t("suppliers.validation.supplier")),
          cash_account_uuid: z
            .string()
            .min(1, t("suppliers.validation.cash_account")),
          amount: z
            .string()
            .trim()
            .refine(isPositiveAmount, t("suppliers.validation.amount")),
          payment_date: z.string().min(1, t("suppliers.validation.required")),
          method: z.string(),
          reference: z.string().max(64),
          description: z.string().max(500),
          allocation_mode: z.string(),
          allocations: z.array(
            z.object({ invoice_uuid: z.string(), amount: z.string() }),
          ),
        })
        .superRefine((v, ctx) => {
          if (v.allocation_mode !== "manual") return;
          let sum = 0;
          v.allocations.forEach((a, i) => {
            if (a.amount.trim() === "") return;
            if (!isPositiveAmount(a.amount)) {
              ctx.addIssue({
                code: "custom",
                path: ["allocations", i, "amount"],
                message: t("suppliers.validation.amount"),
              });
              return;
            }
            sum += Number(normalizeAmount(a.amount));
          });
          if (sum > Number(normalizeAmount(v.amount)) + 0.0001) {
            ctx.addIssue({
              code: "custom",
              path: ["amount"],
              message: t("suppliers.validation.allocations_exceed"),
            });
          }
        }),
    [t],
  );
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<PaymentValues>
      open={open}
      onOpenChange={close}
      formId="supplier-payment-form"
      title={t("suppliers.payments.create_title")}
      description={
        invoice
          ? t("suppliers.payments.for_invoice", {
              number: invoice.invoice_no,
              supplier: invoice.supplier.name ?? "",
            })
          : t("suppliers.payments.form_description")
      }
      submitLabel={t("suppliers.actions.pay")}
      pending={create.isPending}
      problem={problem}
      className="sm:max-w-2xl"
      schema={schema}
      defaultValues={{
        supplier_uuid: fixedSupplier ?? "",
        cash_account_uuid: "",
        amount: invoice ? invoice.open_amount : "",
        payment_date: todayISO(),
        method: "bank_transfer",
        reference: "",
        description: "",
        allocation_mode: "auto",
        allocations: [],
      }}
      onSubmit={async (v) => {
        const manual = v.allocation_mode === "manual";
        try {
          await create.mutateAsync({
            supplier_uuid: v.supplier_uuid,
            cash_account_uuid: v.cash_account_uuid,
            amount: normalizeAmount(v.amount),
            payment_date: v.payment_date,
            method: v.method as SupplierPayment["method"],
            reference: v.reference.trim() || undefined,
            description: v.description.trim() || undefined,
            allocations: manual
              ? v.allocations
                  .filter((a) => a.amount.trim() !== "")
                  .map((a) => ({
                    invoice_uuid: a.invoice_uuid,
                    amount: normalizeAmount(a.amount),
                  }))
              : invoice
                ? [
                    {
                      invoice_uuid: invoice.uuid,
                      // The rest of a larger payment stays as an advance.
                      amount: Math.min(
                        Number(normalizeAmount(v.amount)),
                        Number(invoice.open_amount),
                      ).toFixed(2),
                    },
                  ]
                : undefined,
            auto_allocate: manual ? false : undefined,
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("suppliers.errors.save_failed")));
        }
      }}
    >
      {invoice ? (
        <div className="bg-muted/40 flex flex-wrap items-center justify-between gap-2 rounded-md p-3 text-sm">
          <span>
            {invoice.invoice_no} ·{" "}
            {date(invoice.due_date, "dd.MM.yyyy", locale)}
          </span>
          <Amount value={invoice.open_amount} currency={invoice.currency} />
        </div>
      ) : null}
      <PaymentFields
        fixedSupplier={fixedSupplier}
        suppliers={suppliers}
        cashOptions={cashOptions}
      />
    </FinanceFormDialog>
  );
}

export function VoidPaymentDialog({
  payment,
  onOpenChange,
}: {
  payment: SupplierPayment | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const voidPayment = useVoidPayment();
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<{ reason: string }>
      open={Boolean(payment)}
      onOpenChange={close}
      formId="supplier-payment-void-form"
      title={t("suppliers.payments.void_title")}
      description={
        payment
          ? t("suppliers.payments.void_description", {
              amount: amountText(payment.amount, locale, payment.currency),
              supplier: payment.supplier.name ?? "",
            })
          : undefined
      }
      submitLabel={t("suppliers.actions.void")}
      destructive
      pending={voidPayment.isPending}
      problem={problem}
      schema={z.object({
        reason: z
          .string()
          .trim()
          .min(3, t("suppliers.validation.reason"))
          .max(500),
      })}
      defaultValues={{ reason: "" }}
      onSubmit={async (v) => {
        if (!payment) return;
        try {
          await voidPayment.mutateAsync({
            uuid: payment.uuid,
            reason: v.reason.trim(),
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("suppliers.errors.save_failed")));
        }
      }}
    >
      <AppTextarea name="reason" label={t("suppliers.fields.reason")} />
    </FinanceFormDialog>
  );
}
