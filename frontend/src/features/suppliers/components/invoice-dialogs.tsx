"use client";

import { Download, Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
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
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { FileUploader } from "@/components/upload/file-uploader";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { useCategoryOptions } from "@/features/finance/components/cash-dialogs";
import {
  Amount,
  AttachmentBadge,
} from "@/features/finance/components/finance-cells";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  amountText,
  errorText,
  isPositiveAmount,
  normalizeAmount,
  todayISO,
} from "@/features/finance/lib/format";
import { InvoiceStatusChip } from "@/features/suppliers/components/supplier-cells";
import {
  PAYMENT_METHOD_LABEL_KEYS,
  PAYMENT_STATUS_LABEL_KEYS,
  VAT_RATES,
} from "@/features/suppliers/constants";
import {
  useRemoveInvoiceFile,
  useSaveInvoice,
  useSupplierInvoice,
  useSupplierOptions,
  useUploadInvoiceFile,
  useVoidInvoice,
} from "@/features/suppliers/hooks/use-suppliers";
import { suppliersService } from "@/features/suppliers/services/suppliers.service";
import type { SupplierInvoice } from "@/features/suppliers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useVehiclesList } from "@/features/vehicles/hooks/use-vehicles";
import { date, download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const NONE = "none";

type LineValues = { description: string; net_amount: string; vat_rate: string };

type InvoiceValues = {
  supplier_uuid: string;
  invoice_no: string;
  invoice_date: string;
  due_date: string;
  category_uuid: string;
  branch_uuid: string;
  vehicle_uuid: string;
  description: string;
  mode: string;
  total_amount: string;
  vat_rate: string;
  lines: LineValues[];
};

/** Live totals of the typed lines (display only; the API computes). */
function useLineTotals() {
  const { control } = useFormContext<InvoiceValues>();
  const [mode, lines, total, rate] = useWatch({
    control,
    name: ["mode", "lines", "total_amount", "vat_rate"],
  });
  return useMemo(() => {
    if (mode === "single") {
      const gross = Number(normalizeAmount(total ?? "")) || 0;
      const net = gross / (1 + Number(rate || 0) / 100);
      return { net, vat: gross - net, total: gross };
    }
    let net = 0;
    let vat = 0;
    for (const l of lines ?? []) {
      const n = Number(normalizeAmount(l.net_amount ?? "")) || 0;
      net += n;
      vat += Math.round(n * Number(l.vat_rate || 0)) / 100;
    }
    return { net, vat, total: net + vat };
  }, [lines, mode, rate, total]);
}

function AmountsSection() {
  const { t, locale } = useLocale();
  const { control } = useFormContext<InvoiceValues>();
  const mode = useWatch({ control, name: "mode" });
  const { fields, append, remove } = useFieldArray({ control, name: "lines" });
  const totals = useLineTotals();
  const vatOptions = VAT_RATES.map((r) => ({ value: r, label: `%${r}` }));
  return (
    <div className="space-y-3">
      <AppToggleGroup
        name="mode"
        label={t("suppliers.invoices.amount_mode")}
        options={[
          { value: "single", label: t("suppliers.invoices.mode_single") },
          { value: "lines", label: t("suppliers.invoices.mode_lines") },
        ]}
      />
      {mode === "single" ? (
        <div className="grid gap-4 sm:grid-cols-2">
          <AppInput
            name="total_amount"
            label={t("suppliers.fields.total_with_vat")}
            inputMode="decimal"
          />
          <AppSelect
            name="vat_rate"
            label={t("suppliers.fields.vat_rate")}
            options={vatOptions}
          />
        </div>
      ) : (
        <div className="space-y-3">
          {fields.map((field, index) => (
            <div
              key={field.id}
              className="grid gap-3 rounded-md border p-3 sm:grid-cols-[1fr_9rem_6rem_auto]"
            >
              <AppInput
                name={`lines.${index}.description`}
                label={t("suppliers.fields.line_description")}
              />
              <AppInput
                name={`lines.${index}.net_amount`}
                label={t("suppliers.fields.net_amount")}
                inputMode="decimal"
              />
              <AppSelect
                name={`lines.${index}.vat_rate`}
                label={t("suppliers.fields.vat_rate")}
                options={vatOptions}
              />
              <div className="flex items-end">
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="text-destructive"
                  disabled={fields.length <= 1}
                  aria-label={t("suppliers.actions.remove_line")}
                  onClick={() => remove(index)}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            </div>
          ))}
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={fields.length >= 100}
            onClick={() =>
              append({ description: "", net_amount: "", vat_rate: "20" })
            }
          >
            <Plus className="size-4" />
            {t("suppliers.actions.add_line")}
          </Button>
        </div>
      )}
      <dl className="bg-muted/40 grid grid-cols-3 gap-2 rounded-md p-3 text-sm">
        <div>
          <dt className="text-muted-foreground text-xs">
            {t("suppliers.fields.net_amount")}
          </dt>
          <dd className="tabular-nums">
            {amountText(totals.net.toFixed(2), locale)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground text-xs">
            {t("suppliers.fields.vat_amount")}
          </dt>
          <dd className="tabular-nums">
            {amountText(totals.vat.toFixed(2), locale)}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground text-xs">
            {t("suppliers.fields.total_amount")}
          </dt>
          <dd className="font-semibold tabular-nums">
            {amountText(totals.total.toFixed(2), locale)}
          </dd>
        </div>
      </dl>
    </div>
  );
}

/** Read-only amounts and payments of a recorded invoice. */
function RecordedInvoice({ invoice }: { invoice: SupplierInvoice }) {
  const { t, locale } = useLocale();
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <InvoiceStatusChip status={invoice.status} />
        <span className="text-muted-foreground text-xs">
          {t("suppliers.invoices.amounts_fixed")}
        </span>
      </div>
      {invoice.lines.length > 0 ? (
        <ul className="divide-y rounded-md border text-sm">
          {invoice.lines.map((l) => (
            <li
              key={l.line_no}
              className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
            >
              <span className="min-w-0 truncate">{l.description || "—"}</span>
              <span className="text-muted-foreground text-xs">
                <Amount value={l.net_amount} /> + %{Number(l.vat_rate)}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      <dl className="bg-muted/40 grid grid-cols-2 gap-2 rounded-md p-3 text-sm sm:grid-cols-4">
        <div>
          <dt className="text-muted-foreground text-xs">
            {t("suppliers.fields.net_amount")}
          </dt>
          <dd>
            <Amount value={invoice.net_amount} currency={invoice.currency} />
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground text-xs">
            {t("suppliers.fields.vat_amount")}
          </dt>
          <dd>
            <Amount value={invoice.vat_amount} currency={invoice.currency} />
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground text-xs">
            {t("suppliers.fields.total_amount")}
          </dt>
          <dd className="font-semibold">
            <Amount value={invoice.total_amount} currency={invoice.currency} />
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground text-xs">
            {t("suppliers.fields.open_amount")}
          </dt>
          <dd>
            <Amount value={invoice.open_amount} currency={invoice.currency} />
          </dd>
        </div>
      </dl>
      {invoice.payments.length > 0 ? (
        <div className="space-y-1">
          <div className="text-sm font-medium">
            {t("suppliers.invoices.payments")}
          </div>
          <ul className="divide-y rounded-md border text-sm">
            {invoice.payments.map((p) => (
              <li
                key={p.payment_uuid}
                className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
              >
                <span className="tabular-nums">
                  {date(p.payment_date, "dd.MM.yyyy", locale)} ·{" "}
                  {t(
                    PAYMENT_METHOD_LABEL_KEYS[
                      p.method as keyof typeof PAYMENT_METHOD_LABEL_KEYS
                    ] ?? p.method,
                  )}{" "}
                  · {p.cash_account.code}
                </span>
                <span
                  className={
                    p.status === "void"
                      ? "text-muted-foreground line-through"
                      : undefined
                  }
                >
                  <Amount value={p.amount} />
                  {p.status === "void"
                    ? ` (${t(PAYMENT_STATUS_LABEL_KEYS.void)})`
                    : null}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

/**
 * Record / edit a supplier invoice. Posting writes the payable accrual on
 * the supplier's account and the expense row; amounts are fixed afterwards
 * (void and record again), the number / due date / classification stay
 * editable.
 */
export function InvoiceDialog({
  invoiceUuid,
  supplierUuid,
  open,
  onOpenChange,
}: {
  invoiceUuid: string | null;
  supplierUuid?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [problem, setProblem] = useState<string | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const detail = useSupplierInvoice(open ? invoiceUuid : null);
  const invoice = invoiceUuid ? (detail.data ?? null) : null;
  const editing = Boolean(invoiceUuid);
  const save = useSaveInvoice();
  const upload = useUploadInvoiceFile();
  const remove = useRemoveInvoiceFile();
  const suppliers = useSupplierOptions(open && !editing && !supplierUuid);
  const categories = useCategoryOptions(open, false);
  const branches = useBranchesList(
    { limit: 100, offset: 0, sort: "code" },
    open,
  );
  const vehicles = useVehiclesList(
    { limit: 200, offset: 0, sort: "plate" },
    open,
  );
  const branchOptions = useMemo(
    () => [
      { value: NONE, label: t("finance.head_office") },
      ...(branches.data?.items ?? []).map((b) => ({
        value: b.uuid,
        label: `${b.code} · ${b.name}`,
      })),
    ],
    [branches.data?.items, t],
  );
  const vehicleOptions = useMemo(
    () => [
      { value: NONE, label: t("finance.fields.no_vehicle") },
      ...(vehicles.data?.items ?? []).map((v) => ({
        value: v.uuid,
        label: v.plate,
      })),
    ],
    [t, vehicles.data?.items],
  );
  const schema = useMemo(
    () =>
      z
        .object({
          supplier_uuid: z.string().min(1, t("suppliers.validation.supplier")),
          invoice_no: z
            .string()
            .trim()
            .min(1, t("suppliers.validation.invoice_no"))
            .max(64),
          invoice_date: z.string().min(1, t("suppliers.validation.required")),
          due_date: z.string(),
          category_uuid: z.string().min(1, t("suppliers.validation.required")),
          branch_uuid: z.string(),
          vehicle_uuid: z.string(),
          description: z.string().max(1000),
          mode: z.string(),
          total_amount: z.string(),
          vat_rate: z.string(),
          lines: z.array(
            z.object({
              description: z.string().max(500),
              net_amount: z.string(),
              vat_rate: z.string(),
            }),
          ),
        })
        .superRefine((v, ctx) => {
          if (v.due_date && v.due_date < v.invoice_date) {
            ctx.addIssue({
              code: "custom",
              path: ["due_date"],
              message: t("suppliers.validation.due_before_invoice"),
            });
          }
          if (editing) return;
          if (v.mode === "single" && !isPositiveAmount(v.total_amount)) {
            ctx.addIssue({
              code: "custom",
              path: ["total_amount"],
              message: t("suppliers.validation.amount"),
            });
          }
          if (v.mode === "lines") {
            v.lines.forEach((l, i) => {
              if (!isPositiveAmount(l.net_amount)) {
                ctx.addIssue({
                  code: "custom",
                  path: ["lines", i, "net_amount"],
                  message: t("suppliers.validation.amount"),
                });
              }
            });
          }
        }),
    [editing, t],
  );
  const close = (next: boolean) => {
    if (!next) {
      setProblem(null);
      setFile(null);
    }
    onOpenChange(next);
  };
  const ready = !editing || invoice;

  return (
    <FinanceFormDialog<InvoiceValues>
      open={open && Boolean(ready)}
      onOpenChange={close}
      formId="supplier-invoice-form"
      title={
        editing
          ? t("suppliers.invoices.edit_title", {
              number: invoice?.invoice_no ?? "",
            })
          : t("suppliers.invoices.create_title")
      }
      description={
        editing
          ? invoice?.supplier.name
          : t("suppliers.invoices.form_description")
      }
      submitLabel={t("form.save")}
      pending={save.isPending || upload.isPending}
      problem={problem}
      className="sm:max-w-3xl"
      schema={schema}
      defaultValues={{
        supplier_uuid: invoice?.supplier.uuid ?? supplierUuid ?? "",
        invoice_no: invoice?.invoice_no ?? "",
        invoice_date: invoice?.invoice_date ?? todayISO(),
        due_date: invoice?.due_date ?? "",
        category_uuid: invoice?.category.uuid ?? "",
        branch_uuid: invoice?.branch?.uuid ?? NONE,
        vehicle_uuid: invoice?.vehicle?.uuid ?? NONE,
        description: invoice?.description ?? "",
        mode: "single",
        total_amount: "",
        vat_rate: "20",
        lines: [{ description: "", net_amount: "", vat_rate: "20" }],
      }}
      onSubmit={async (v) => {
        const branch = v.branch_uuid !== NONE ? v.branch_uuid : undefined;
        const vehicle = v.vehicle_uuid !== NONE ? v.vehicle_uuid : undefined;
        try {
          const saved = await save.mutateAsync({
            uuid: invoice?.uuid,
            body: editing
              ? {
                  invoice_no: v.invoice_no.trim(),
                  due_date: v.due_date || undefined,
                  category_uuid: v.category_uuid,
                  branch_uuid: branch,
                  clear_branch: !branch ? true : undefined,
                  vehicle_uuid: vehicle,
                  clear_vehicle: !vehicle ? true : undefined,
                  description: v.description.trim(),
                }
              : {
                  supplier_uuid: v.supplier_uuid,
                  invoice_no: v.invoice_no.trim(),
                  invoice_date: v.invoice_date,
                  due_date: v.due_date || undefined,
                  category_uuid: v.category_uuid,
                  branch_uuid: branch,
                  vehicle_uuid: vehicle,
                  description: v.description.trim() || undefined,
                  ...(v.mode === "single"
                    ? {
                        total_amount: normalizeAmount(v.total_amount),
                        vat_rate: v.vat_rate,
                      }
                    : {
                        lines: v.lines.map((l) => ({
                          description: l.description.trim(),
                          net_amount: normalizeAmount(l.net_amount),
                          vat_rate: l.vat_rate,
                        })),
                      }),
                },
          });
          if (file) await upload.mutateAsync({ uuid: saved.uuid, file });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("suppliers.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        {!editing && !supplierUuid ? (
          <AppCombobox
            name="supplier_uuid"
            label={t("suppliers.fields.supplier")}
            placeholder={t("suppliers.fields.choose_supplier")}
            options={suppliers}
            className="sm:col-span-2"
          />
        ) : null}
        <AppInput name="invoice_no" label={t("suppliers.fields.invoice_no")} />
        <AppSelect
          name="category_uuid"
          label={t("suppliers.fields.category")}
          placeholder={t("finance.fields.choose_category")}
          options={categories}
        />
        <AppDatePicker
          name="invoice_date"
          label={t("suppliers.fields.invoice_date")}
          disabled={editing}
        />
        <AppDatePicker
          name="due_date"
          label={t("suppliers.fields.due_date")}
          description={editing ? undefined : t("suppliers.fields.due_hint")}
        />
        <AppSelect
          name="branch_uuid"
          label={t("suppliers.fields.branch")}
          options={branchOptions}
        />
        <AppCombobox
          name="vehicle_uuid"
          label={t("suppliers.fields.vehicle")}
          options={vehicleOptions}
        />
      </div>
      <Separator />
      {invoice ? <RecordedInvoice invoice={invoice} /> : <AmountsSection />}
      <AppTextarea
        name="description"
        label={t("suppliers.fields.description")}
      />
      <div className="space-y-2">
        <div className="text-sm font-medium">
          {t("suppliers.fields.attachment")}
        </div>
        {invoice?.attachment ? (
          <div className="flex flex-wrap items-center gap-2">
            <AttachmentBadge name={invoice.attachment.name} />
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={async () => {
                const out = await suppliersService.downloadInvoiceFile(
                  slug,
                  invoice.uuid,
                );
                download(
                  out.blob,
                  out.filename ?? invoice.attachment?.name ?? "invoice",
                );
              }}
            >
              <Download className="size-4" />
              {t("suppliers.actions.download")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="text-destructive"
              disabled={remove.isPending}
              onClick={() => remove.mutate(invoice.uuid)}
            >
              <Trash2 className="size-4" />
              {t("suppliers.actions.remove_file")}
            </Button>
          </div>
        ) : null}
        <FileUploader
          multiple={false}
          accept="application/pdf,image/jpeg,image/png,image/webp"
          maxSizeBytes={10 * 1024 * 1024}
          onUpload={(files) => setFile(files[0] ?? null)}
        />
        <p className="text-muted-foreground text-xs">
          {t("suppliers.fields.attachment_hint")}
        </p>
      </div>
    </FinanceFormDialog>
  );
}

export function VoidInvoiceDialog({
  invoice,
  onOpenChange,
}: {
  invoice: SupplierInvoice | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const voidInvoice = useVoidInvoice();
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<{ reason: string }>
      open={Boolean(invoice)}
      onOpenChange={close}
      formId="supplier-invoice-void-form"
      title={t("suppliers.invoices.void_title")}
      description={
        invoice
          ? t("suppliers.invoices.void_description", {
              number: invoice.invoice_no,
              amount: amountText(
                invoice.total_amount,
                locale,
                invoice.currency,
              ),
            })
          : undefined
      }
      submitLabel={t("suppliers.actions.void")}
      destructive
      pending={voidInvoice.isPending}
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
        if (!invoice) return;
        try {
          await voidInvoice.mutateAsync({
            uuid: invoice.uuid,
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
