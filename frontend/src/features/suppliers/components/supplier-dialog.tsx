"use client";

import { Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useFieldArray, useFormContext } from "react-hook-form";
import { z } from "zod";

import {
  AppInput,
  AppSelect,
  AppSwitch,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import { errorText } from "@/features/finance/lib/format";
import {
  SUPPLIER_TYPES,
  SUPPLIER_TYPE_LABEL_KEYS,
} from "@/features/suppliers/constants";
import { useSaveSupplier } from "@/features/suppliers/hooks/use-suppliers";
import { validTaxNumber } from "@/features/suppliers/lib/validation";
import type { Supplier } from "@/features/suppliers/types";
import { useLocale } from "@/providers/locale-provider";

type ContactValues = {
  name: string;
  role: string;
  phone: string;
  email: string;
};

type SupplierValues = {
  title: string;
  code: string;
  supplier_type: string;
  tax_number: string;
  tax_office: string;
  phone: string;
  email: string;
  website: string;
  city: string;
  address: string;
  billing_address: string;
  billing_email: string;
  e_invoice: boolean;
  bank_name: string;
  iban: string;
  payment_term_days: string;
  note: string;
  contacts: ContactValues[];
};

const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

function ContactsField() {
  const { t } = useLocale();
  const { control } = useFormContext<SupplierValues>();
  const { fields, append, remove } = useFieldArray({
    control,
    name: "contacts",
  });
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <div className="text-sm font-medium">
          {t("suppliers.fields.contacts")}
        </div>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={fields.length >= 10}
          onClick={() => append({ name: "", role: "", phone: "", email: "" })}
        >
          <Plus className="size-4" />
          {t("suppliers.actions.add_contact")}
        </Button>
      </div>
      {fields.length === 0 ? (
        <p className="text-muted-foreground text-xs">
          {t("suppliers.fields.contacts_empty")}
        </p>
      ) : null}
      {fields.map((field, index) => (
        <div
          key={field.id}
          className="grid gap-3 rounded-md border p-3 sm:grid-cols-2"
        >
          <AppInput
            name={`contacts.${index}.name`}
            label={t("suppliers.fields.contact_name")}
          />
          <AppInput
            name={`contacts.${index}.role`}
            label={t("suppliers.fields.contact_role")}
          />
          <AppInput
            name={`contacts.${index}.phone`}
            label={t("suppliers.fields.phone")}
            inputMode="tel"
          />
          <div className="flex items-end gap-2">
            <AppInput
              name={`contacts.${index}.email`}
              label={t("suppliers.fields.email")}
              inputMode="email"
              className="flex-1"
            />
            <Button
              type="button"
              size="icon"
              variant="ghost"
              className="text-destructive shrink-0"
              aria-label={t("suppliers.actions.remove_contact")}
              onClick={() => remove(index)}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}

/**
 * Create / edit a supplier card: identity and tax data (VKN / TCKN checked
 * here and by the API), contact persons, billing and bank data, terms.
 */
export function SupplierDialog({
  supplier,
  open,
  onOpenChange,
  onSaved,
}: {
  supplier: Supplier | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved?: (supplier: Supplier) => void;
}) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const save = useSaveSupplier();
  const editing = Boolean(supplier);
  const schema = useMemo(
    () =>
      z.object({
        title: z
          .string()
          .trim()
          .min(1, t("suppliers.validation.title"))
          .max(255),
        code: z.string().max(32),
        supplier_type: z.string(),
        tax_number: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || validTaxNumber(v.replace(/\s/g, "")),
            t("suppliers.validation.tax_number"),
          ),
        tax_office: z.string().max(128),
        phone: z.string().max(32),
        email: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || EMAIL.test(v),
            t("suppliers.validation.email"),
          ),
        website: z.string().max(255),
        city: z.string().max(64),
        address: z.string().max(1000),
        billing_address: z.string().max(1000),
        billing_email: z
          .string()
          .trim()
          .refine(
            (v) => v === "" || EMAIL.test(v),
            t("suppliers.validation.email"),
          ),
        e_invoice: z.boolean(),
        bank_name: z.string().max(128),
        iban: z.string().max(42),
        payment_term_days: z
          .string()
          .trim()
          .regex(/^\d{1,3}$/, t("suppliers.validation.term"))
          .refine((v) => Number(v) <= 365, t("suppliers.validation.term")),
        note: z.string().max(2000),
        contacts: z
          .array(
            z.object({
              name: z
                .string()
                .trim()
                .min(1, t("suppliers.validation.contact_name"))
                .max(128),
              role: z.string().max(128),
              phone: z.string().max(32),
              email: z
                .string()
                .trim()
                .refine(
                  (v) => v === "" || EMAIL.test(v),
                  t("suppliers.validation.email"),
                ),
            }),
          )
          .max(10),
      }),
    [t],
  );
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };

  return (
    <FinanceFormDialog<SupplierValues>
      open={open}
      onOpenChange={close}
      formId="supplier-form"
      title={editing ? t("suppliers.edit_title") : t("suppliers.create_title")}
      description={t("suppliers.form_description")}
      submitLabel={t("form.save")}
      pending={save.isPending}
      problem={problem}
      className="sm:max-w-2xl"
      schema={schema}
      defaultValues={{
        title: supplier?.title ?? "",
        code: supplier?.code ?? "",
        supplier_type: supplier?.supplier_type ?? "service",
        tax_number: supplier?.tax_number ?? "",
        tax_office: supplier?.tax_office ?? "",
        phone: supplier?.phone ?? "",
        email: supplier?.email ?? "",
        website: supplier?.website ?? "",
        city: supplier?.city ?? "",
        address: supplier?.address ?? "",
        billing_address: supplier?.billing_address ?? "",
        billing_email: supplier?.billing_email ?? "",
        e_invoice: supplier?.e_invoice ?? false,
        bank_name: supplier?.bank_name ?? "",
        iban: supplier?.iban ?? "",
        payment_term_days: String(supplier?.payment_term_days ?? 30),
        note: supplier?.note ?? "",
        contacts: (supplier?.contacts ?? []).map((c) => ({ ...c })),
      }}
      onSubmit={async (v) => {
        try {
          const saved = await save.mutateAsync({
            uuid: supplier?.uuid,
            body: {
              title: v.title.trim(),
              code: v.code.trim() || (editing ? undefined : ""),
              supplier_type: v.supplier_type as Supplier["supplier_type"],
              tax_number: v.tax_number.replace(/\s/g, ""),
              tax_office: v.tax_office.trim(),
              phone: v.phone.trim(),
              email: v.email.trim(),
              website: v.website.trim(),
              city: v.city.trim(),
              address: v.address.trim(),
              billing_address: v.billing_address.trim(),
              billing_email: v.billing_email.trim(),
              e_invoice: v.e_invoice,
              bank_name: v.bank_name.trim(),
              iban: v.iban.trim(),
              payment_term_days: Number(v.payment_term_days),
              note: v.note.trim(),
              contacts: v.contacts.map((c) => ({
                name: c.name.trim(),
                role: c.role.trim(),
                phone: c.phone.trim(),
                email: c.email.trim(),
              })),
            },
          });
          onSaved?.(saved);
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("suppliers.errors.save_failed")));
        }
      }}
    >
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="title"
          label={t("suppliers.fields.title")}
          className="sm:col-span-2"
        />
        <AppSelect
          name="supplier_type"
          label={t("suppliers.fields.supplier_type")}
          options={SUPPLIER_TYPES.map((type) => ({
            value: type,
            label: t(SUPPLIER_TYPE_LABEL_KEYS[type]),
          }))}
        />
        <AppInput
          name="code"
          label={t("suppliers.fields.code")}
          description={editing ? undefined : t("suppliers.fields.code_hint")}
        />
        <AppInput
          name="tax_number"
          label={t("suppliers.fields.tax_number")}
          inputMode="numeric"
          description={t("suppliers.fields.tax_number_hint")}
        />
        <AppInput name="tax_office" label={t("suppliers.fields.tax_office")} />
        <AppInput
          name="phone"
          label={t("suppliers.fields.phone")}
          inputMode="tel"
        />
        <AppInput
          name="email"
          label={t("suppliers.fields.email")}
          inputMode="email"
        />
        <AppInput name="city" label={t("suppliers.fields.city")} />
        <AppInput name="website" label={t("suppliers.fields.website")} />
      </div>
      <AppTextarea name="address" label={t("suppliers.fields.address")} />
      <Separator />
      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="payment_term_days"
          label={t("suppliers.fields.payment_term_days")}
          inputMode="numeric"
          description={t("suppliers.fields.payment_term_hint")}
        />
        <AppInput
          name="billing_email"
          label={t("suppliers.fields.billing_email")}
          inputMode="email"
        />
        <AppInput name="bank_name" label={t("suppliers.fields.bank_name")} />
        <AppInput name="iban" label={t("suppliers.fields.iban")} />
      </div>
      <AppTextarea
        name="billing_address"
        label={t("suppliers.fields.billing_address")}
      />
      <AppSwitch
        name="e_invoice"
        label={t("suppliers.fields.e_invoice")}
        description={t("suppliers.fields.e_invoice_hint")}
      />
      <Separator />
      <ContactsField />
      <AppTextarea name="note" label={t("suppliers.fields.note")} />
    </FinanceFormDialog>
  );
}
