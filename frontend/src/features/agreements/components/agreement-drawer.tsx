"use client";

import { useCallback, useMemo } from "react";
import { Controller, useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { EntityDrawer } from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormFieldShell,
  FormSection,
} from "@/components/forms";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import {
  INVOICE_PERIODS,
  INVOICE_PERIOD_HINT_KEYS,
  INVOICE_PERIOD_LABEL_KEYS,
} from "@/features/agreements/constants";
import {
  useCreateAgreement,
  useUpdateAgreement,
} from "@/features/agreements/hooks/use-agreements";
import type {
  Agreement,
  AgreementInput,
  InvoicePeriod,
} from "@/features/agreements/types";
import { customersService } from "@/features/customers/services/customers.service";
import { useBranchOptions } from "@/features/pricing/hooks/use-pricing-options";
import { isAmount, normalizeAmount } from "@/features/pricing/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "agreement-form";
const HEAD_OFFICE = "__head_office__";

type Values = {
  customer_uuid: string;
  name: string;
  branch_uuid: string;
  valid_from: string;
  valid_to: string;
  invoice_period: InvoicePeriod;
  payment_term_days: string;
  open_account: boolean;
  credit_limit: string;
  coupon_eligible: boolean;
  monthly_km_allowance: string;
  maintenance_interval_days: string;
  maintenance_interval_km: string;
  notes: string;
  activate: boolean;
};

const INT_RE = /^\d{0,7}$/;

function today() {
  return new Date().toISOString().slice(0, 10);
}

function defaults(a: Agreement | null): Values {
  return {
    customer_uuid: a?.customer.uuid ?? "",
    name: a?.name ?? "",
    branch_uuid: a?.branch?.uuid ?? HEAD_OFFICE,
    valid_from: a?.valid_from ?? today(),
    valid_to: a?.valid_to ?? "",
    invoice_period: a?.invoice_period ?? "monthly",
    payment_term_days: String(a?.payment_term_days ?? 30),
    open_account: a?.open_account ?? false,
    credit_limit: a?.credit_limit ?? "",
    coupon_eligible: a?.coupon_eligible ?? false,
    monthly_km_allowance: a?.monthly_km_allowance
      ? String(a.monthly_km_allowance)
      : "",
    maintenance_interval_days: a?.maintenance_interval_days
      ? String(a.maintenance_interval_days)
      : "",
    maintenance_interval_km: a?.maintenance_interval_km
      ? String(a.maintenance_interval_km)
      : "",
    notes: a?.notes ?? "",
    activate: false,
  };
}

const optInt = (v: string) => (v.trim() === "" ? 0 : Number(v));

/** Corporate customer picker (company customers only). */
function CustomerField({ disabled }: { disabled: boolean }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const {
    control,
    formState: { errors },
  } = useFormContext<Values>();
  const loadOptions = useCallback(
    async (query: string): Promise<ComboboxOption[]> => {
      try {
        const page = await customersService.list(slug, {
          limit: 20,
          offset: 0,
          q: query || undefined,
          customer_type: "corporate",
          anonymized: "false",
        });
        return page.items.map((c) => ({
          value: c.uuid,
          label: c.display_name,
          description: c.identity_masked || undefined,
        }));
      } catch {
        return [];
      }
    },
    [slug],
  );
  return (
    <FormFieldShell
      name="customer_uuid"
      label={t("agreements.fields.customer")}
      description={t("agreements.fields.customer_hint")}
      error={errors.customer_uuid?.message}
    >
      <Controller
        name="customer_uuid"
        control={control}
        render={({ field }) => (
          <AsyncCombobox
            id="customer_uuid"
            value={field.value}
            onValueChange={field.onChange}
            loadOptions={loadOptions}
            placeholder={t("agreements.fields.customer_placeholder")}
            searchPlaceholder={t("agreements.fields.customer_search")}
            emptyText={t("agreements.fields.customer_empty")}
            disabled={disabled}
          />
        )}
      />
    </FormFieldShell>
  );
}

function OpenAccountFields() {
  const { t } = useLocale();
  const open = useWatch<Values>({ name: "open_account" }) as boolean;
  return open ? (
    <AppInput
      name="credit_limit"
      inputMode="decimal"
      label={t("agreements.fields.credit_limit")}
      description={t("agreements.fields.credit_limit_hint")}
    />
  ) : null;
}

function InvoiceHint() {
  const { t } = useLocale();
  const period = useWatch<Values>({ name: "invoice_period" }) as InvoicePeriod;
  return (
    <p className="text-muted-foreground text-sm">
      {t(INVOICE_PERIOD_HINT_KEYS[period])}
    </p>
  );
}

/** Create / edit drawer of a corporate agreement. */
export function AgreementDrawer({
  open,
  agreement,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  agreement: Agreement | null;
  onOpenChange: (open: boolean) => void;
  onCreated?: (agreement: Agreement) => void;
}) {
  const { t } = useLocale();
  const create = useCreateAgreement();
  const update = useUpdateAgreement();
  const branches = useBranchOptions();
  const schema = useMemo(
    () =>
      z
        .object({
          customer_uuid: z.string().min(1, t("agreements.validation.customer")),
          name: z
            .string()
            .trim()
            .min(1, t("agreements.validation.name"))
            .max(255),
          branch_uuid: z.string(),
          valid_from: z.string().min(1, t("agreements.validation.valid_from")),
          valid_to: z.string(),
          invoice_period: z.enum(["monthly", "per_rental"]),
          payment_term_days: z
            .string()
            .trim()
            .refine(
              (v) => /^\d{1,3}$/.test(v) && Number(v) <= 365,
              t("agreements.validation.term"),
            ),
          open_account: z.boolean(),
          credit_limit: z
            .string()
            .refine(
              (v) => v.trim() === "" || isAmount(v),
              t("agreements.validation.amount"),
            ),
          coupon_eligible: z.boolean(),
          monthly_km_allowance: z
            .string()
            .refine(
              (v) => INT_RE.test(v.trim()),
              t("agreements.validation.int"),
            ),
          maintenance_interval_days: z
            .string()
            .refine(
              (v) => INT_RE.test(v.trim()),
              t("agreements.validation.int"),
            ),
          maintenance_interval_km: z
            .string()
            .refine(
              (v) => INT_RE.test(v.trim()),
              t("agreements.validation.int"),
            ),
          notes: z.string().max(5000),
          activate: z.boolean(),
        })
        .refine((v) => !v.open_account || v.credit_limit.trim() !== "", {
          path: ["credit_limit"],
          message: t("agreements.validation.credit_limit_required"),
        })
        .refine((v) => v.valid_to === "" || v.valid_to >= v.valid_from, {
          path: ["valid_to"],
          message: t("agreements.validation.valid_to"),
        }),
    [t],
  ) as unknown as z.ZodType<Values>;

  const branchOptions = [
    { value: HEAD_OFFICE, label: t("agreements.head_office") },
    ...branches.options,
  ];

  return (
    <EntityDrawer
      open={open}
      onOpenChange={onOpenChange}
      title={
        agreement
          ? t("agreements.drawer.edit_title")
          : t("agreements.drawer.create_title")
      }
      description={t("agreements.drawer.description")}
      size="lg"
      footer={
        <div className="flex w-full justify-end gap-2">
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            disabled={create.isPending || update.isPending}
          >
            {t("agreements.actions.save")}
          </Button>
        </div>
      }
    >
      {open ? (
        <AppForm<Values>
          key={agreement?.uuid ?? "new"}
          id={FORM_ID}
          className="space-y-6 px-4"
          schema={schema}
          defaultValues={defaults(agreement)}
          onSubmit={async (values) => {
            const headOffice = values.branch_uuid === HEAD_OFFICE;
            const body: AgreementInput = {
              name: values.name.trim(),
              branch_uuid: headOffice ? null : values.branch_uuid,
              valid_from: values.valid_from,
              valid_to: values.valid_to || null,
              invoice_period: values.invoice_period,
              payment_term_days: Number(values.payment_term_days),
              open_account: values.open_account,
              credit_limit: values.credit_limit.trim()
                ? normalizeAmount(values.credit_limit)
                : null,
              coupon_eligible: values.coupon_eligible,
              monthly_km_allowance: optInt(values.monthly_km_allowance),
              maintenance_interval_days: optInt(
                values.maintenance_interval_days,
              ),
              maintenance_interval_km: optInt(values.maintenance_interval_km),
              notes: values.notes.trim(),
            };
            if (agreement) {
              await update.mutateAsync({
                uuid: agreement.uuid,
                body: {
                  ...body,
                  clear_branch: headOffice && agreement.branch !== null,
                  clear_valid_to:
                    !values.valid_to && agreement.valid_to !== null,
                },
              });
            } else {
              const created = await create.mutateAsync({
                ...body,
                customer_uuid: values.customer_uuid,
                activate: values.activate,
              });
              onCreated?.(created);
            }
            onOpenChange(false);
          }}
        >
          <FormSection title={t("agreements.drawer.section_party")}>
            <CustomerField disabled={agreement !== null} />
            <AppInput
              name="name"
              label={t("agreements.fields.name")}
              placeholder={t("agreements.fields.name_placeholder")}
            />
            <AppSelect
              name="branch_uuid"
              label={t("agreements.fields.branch")}
              description={t("agreements.fields.branch_hint")}
              options={branchOptions}
            />
            <div className="grid gap-4 sm:grid-cols-2">
              <AppDatePicker
                name="valid_from"
                label={t("agreements.fields.valid_from")}
              />
              <AppDatePicker
                name="valid_to"
                label={t("agreements.fields.valid_to")}
                description={t("agreements.fields.valid_to_hint")}
              />
            </div>
          </FormSection>
          <FormSection title={t("agreements.drawer.section_payment")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <AppSelect
                name="invoice_period"
                label={t("agreements.fields.invoice_period")}
                options={INVOICE_PERIODS.map((v) => ({
                  value: v,
                  label: t(INVOICE_PERIOD_LABEL_KEYS[v]),
                }))}
              />
              <AppInput
                name="payment_term_days"
                inputMode="numeric"
                label={t("agreements.fields.payment_term_days")}
                description={t("agreements.fields.payment_term_hint")}
              />
            </div>
            <InvoiceHint />
            <AppSwitch
              name="open_account"
              label={t("agreements.fields.open_account")}
              description={t("agreements.fields.open_account_hint")}
            />
            <OpenAccountFields />
            <AppSwitch
              name="coupon_eligible"
              label={t("agreements.fields.coupon_eligible")}
              description={t("agreements.fields.coupon_eligible_hint")}
            />
          </FormSection>
          <FormSection title={t("agreements.drawer.section_long_term")}>
            <div className="grid gap-4 sm:grid-cols-3">
              <AppInput
                name="monthly_km_allowance"
                inputMode="numeric"
                label={t("agreements.fields.monthly_km_allowance")}
              />
              <AppInput
                name="maintenance_interval_days"
                inputMode="numeric"
                label={t("agreements.fields.maintenance_interval_days")}
              />
              <AppInput
                name="maintenance_interval_km"
                inputMode="numeric"
                label={t("agreements.fields.maintenance_interval_km")}
              />
            </div>
            <AppTextarea
              name="notes"
              label={t("agreements.fields.notes")}
              rows={3}
            />
            {agreement ? null : (
              <AppSwitch
                name="activate"
                label={t("agreements.fields.activate")}
                description={t("agreements.fields.activate_hint")}
              />
            )}
          </FormSection>
        </AppForm>
      ) : null}
    </EntityDrawer>
  );
}
