"use client";

import { Download, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import {
  AppCombobox,
  AppDatePicker,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { FileUploader } from "@/components/upload/file-uploader";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import { useCategoryOptions } from "@/features/finance/components/cash-dialogs";
import { AttachmentBadge } from "@/features/finance/components/finance-cells";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  useCashAccountsList,
  useRemoveAttachment,
  useSaveExpense,
  useUploadAttachment,
  useVoidExpense,
} from "@/features/finance/hooks/use-finance";
import { financeService } from "@/features/finance/services/finance.service";
import {
  amountText,
  errorText,
  isPositiveAmount,
  normalizeAmount,
  todayISO,
} from "@/features/finance/lib/format";
import type { Expense } from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useVehiclesList } from "@/features/vehicles/hooks/use-vehicles";
import { download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const NONE = "none";
const VAT_RATES = ["0", "1", "10", "20"];

type ExpenseValues = {
  category_uuid: string;
  expense_date: string;
  total_amount: string;
  vat_rate: string;
  description: string;
  supplier_name: string;
  document_no: string;
  branch_uuid: string;
  vehicle_uuid: string;
  cash_account_uuid: string;
};

/**
 * Record / edit an expense. Amounts, VAT and the paying account are fixed
 * once recorded (void and record again); the receipt or invoice is stored
 * through the API (private storage, streamed back by the API).
 */
export function ExpenseDialog({
  expense,
  open,
  onOpenChange,
}: {
  expense: Expense | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const [problem, setProblem] = useState<string | null>(null);
  const [file, setFile] = useState<File | null>(null);
  const save = useSaveExpense();
  const upload = useUploadAttachment();
  const remove = useRemoveAttachment();
  const categories = useCategoryOptions(open, false);
  const branches = useBranchesList(
    { limit: 100, offset: 0, sort: "code" },
    open,
  );
  const vehicles = useVehiclesList(
    { limit: 200, offset: 0, sort: "plate" },
    open,
  );
  const cash = useCashAccountsList(
    { limit: 100, offset: 0, status: "active", sort: "code" },
    open && !expense,
  );
  const editing = Boolean(expense);
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
  const cashOptions = useMemo(
    () => [
      { value: NONE, label: t("finance.fields.unpaid") },
      ...(cash.data?.items ?? []).map((a) => ({
        value: a.uuid,
        label: `${a.code} · ${a.name} (${amountText(a.balance, locale, a.currency)})`,
      })),
    ],
    [cash.data?.items, locale, t],
  );
  const close = (next: boolean) => {
    if (!next) {
      setProblem(null);
      setFile(null);
    }
    onOpenChange(next);
  };
  const pending = save.isPending || upload.isPending;

  return (
    <FinanceFormDialog<ExpenseValues>
      open={open}
      onOpenChange={close}
      formId="finance-expense-form"
      title={
        editing
          ? t("finance.expenses.edit_title")
          : t("finance.expenses.create_title")
      }
      description={t("finance.expenses.form_description")}
      submitLabel={t("form.save")}
      pending={pending}
      problem={problem}
      schema={z.object({
        category_uuid: z.string().min(1, t("finance.validation.required")),
        expense_date: z.string().min(1, t("finance.validation.required")),
        total_amount: editing
          ? z.string()
          : z
              .string()
              .trim()
              .refine(isPositiveAmount, t("finance.validation.amount")),
        vat_rate: z.string(),
        description: z.string().max(1000),
        supplier_name: z.string().max(255),
        document_no: z.string().max(64),
        branch_uuid: z.string(),
        vehicle_uuid: z.string(),
        cash_account_uuid: z.string(),
      })}
      defaultValues={{
        category_uuid: expense?.category.uuid ?? "",
        expense_date: expense?.expense_date ?? todayISO(),
        total_amount: expense?.total_amount ?? "",
        vat_rate: expense ? String(Number(expense.vat_rate)) : "20",
        description: expense?.description ?? "",
        supplier_name: expense?.supplier_name ?? "",
        document_no: expense?.document_no ?? "",
        branch_uuid: expense?.branch?.uuid ?? NONE,
        vehicle_uuid: expense?.vehicle?.uuid ?? NONE,
        cash_account_uuid: expense?.cash_account?.uuid ?? NONE,
      }}
      onSubmit={async (v) => {
        const branch = v.branch_uuid !== NONE ? v.branch_uuid : undefined;
        const vehicle = v.vehicle_uuid !== NONE ? v.vehicle_uuid : undefined;
        try {
          const saved = await save.mutateAsync({
            uuid: expense?.uuid,
            body: editing
              ? {
                  category_uuid: v.category_uuid,
                  expense_date: v.expense_date,
                  description: v.description.trim(),
                  supplier_name: v.supplier_name.trim(),
                  document_no: v.document_no.trim(),
                  branch_uuid: branch,
                  clear_branch: !branch ? true : undefined,
                  vehicle_uuid: vehicle,
                  clear_vehicle: !vehicle ? true : undefined,
                }
              : {
                  category_uuid: v.category_uuid,
                  expense_date: v.expense_date,
                  total_amount: normalizeAmount(v.total_amount),
                  vat_rate: v.vat_rate,
                  description: v.description.trim() || undefined,
                  supplier_name: v.supplier_name.trim() || undefined,
                  document_no: v.document_no.trim() || undefined,
                  branch_uuid: branch,
                  vehicle_uuid: vehicle,
                  cash_account_uuid:
                    v.cash_account_uuid !== NONE
                      ? v.cash_account_uuid
                      : undefined,
                },
          });
          if (file) {
            await upload.mutateAsync({ uuid: saved.uuid, file });
          }
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppSelect
          name="category_uuid"
          label={t("finance.fields.category")}
          placeholder={t("finance.fields.choose_category")}
          options={categories}
        />
        <AppDatePicker name="expense_date" label={t("finance.fields.date")} />
        <AppInput
          name="total_amount"
          label={t("finance.fields.total_with_vat")}
          inputMode="decimal"
          disabled={editing}
        />
        <AppSelect
          name="vat_rate"
          label={t("finance.fields.vat_rate")}
          disabled={editing}
          options={VAT_RATES.map((r) => ({ value: r, label: `%${r}` }))}
        />
        <AppInput
          name="supplier_name"
          label={t("finance.fields.supplier_name")}
        />
        <AppInput name="document_no" label={t("finance.fields.document_no")} />
        <AppSelect
          name="branch_uuid"
          label={t("finance.fields.branch")}
          options={branchOptions}
        />
        <AppCombobox
          name="vehicle_uuid"
          label={t("finance.fields.vehicle")}
          options={vehicleOptions}
        />
      </div>
      {!editing ? (
        <AppSelect
          name="cash_account_uuid"
          label={t("finance.fields.paid_from")}
          description={t("finance.fields.paid_from_hint")}
          options={cashOptions}
        />
      ) : null}
      <AppTextarea name="description" label={t("finance.fields.description")} />
      <div className="space-y-2">
        <div className="text-sm font-medium">
          {t("finance.fields.attachment")}
        </div>
        {expense?.attachment ? (
          <div className="flex flex-wrap items-center gap-2">
            <AttachmentBadge name={expense.attachment.name} />
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={async () => {
                const out = await financeService.downloadAttachment(
                  slug,
                  expense.uuid,
                );
                download(
                  out.blob,
                  out.filename ?? expense.attachment?.name ?? "receipt",
                );
              }}
            >
              <Download className="size-4" />
              {t("finance.actions.download")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="text-destructive"
              disabled={remove.isPending}
              onClick={() => remove.mutate(expense.uuid)}
            >
              <Trash2 className="size-4" />
              {t("finance.actions.remove_attachment")}
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
          {t("finance.fields.attachment_hint")}
        </p>
      </div>
    </FinanceFormDialog>
  );
}

export function VoidExpenseDialog({
  expense,
  onOpenChange,
}: {
  expense: Expense | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const voidExpense = useVoidExpense();
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<{ reason: string }>
      open={Boolean(expense)}
      onOpenChange={close}
      formId="finance-void-expense-form"
      title={t("finance.expenses.void_title")}
      description={
        expense
          ? t("finance.expenses.void_description", {
              amount: amountText(
                expense.total_amount,
                locale,
                expense.currency,
              ),
            })
          : undefined
      }
      submitLabel={t("finance.actions.void")}
      destructive
      pending={voidExpense.isPending}
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
        if (!expense) return;
        try {
          await voidExpense.mutateAsync({
            uuid: expense.uuid,
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
