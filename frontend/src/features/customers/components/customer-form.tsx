"use client";

import {
  Contact,
  FileText,
  IdCard,
  ReceiptText,
  ShieldCheck,
} from "lucide-react";
import { useMemo } from "react";

import {
  AppCombobox,
  AppForm,
  AppInput,
  AppRadioGroup,
  AppSelect,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  CUSTOMER_TYPES,
  CUSTOMER_TYPE_LABEL_KEYS,
  IDENTITY_TYPES_BY_CUSTOMER,
  IDENTITY_TYPE_LABEL_KEYS,
} from "@/features/customers/constants";
import { countryOptions } from "@/features/customers/lib/format";
import {
  NO_IDENTITY,
  createCustomerFormSchema,
  type CustomerFormValues,
} from "@/features/customers/schemas/customer-form";
import type { Customer } from "@/features/customers/types";
import { useLocale } from "@/providers/locale-provider";

type CustomerFormProps = {
  mode: "create" | "edit";
  defaultValues: CustomerFormValues;
  /** The stored customer on edit (masked values as placeholders). */
  customer?: Customer;
  isSubmitting?: boolean;
  onSubmit: (values: CustomerFormValues) => Promise<void>;
  onCancel: () => void;
};

/**
 * Create / edit form: type and name, identity (TCKN / VKN checksums,
 * passport + nationality), contact, billing, tags and note. Identity
 * number, phone and e-mail are write-only: on edit the fields start empty
 * with the masked value as placeholder, and an empty field keeps it.
 */
export function CustomerForm({
  mode,
  defaultValues,
  customer,
  isSubmitting,
  onSubmit,
  onCancel,
}: CustomerFormProps) {
  const { t, locale } = useLocale();
  const schema = useMemo(
    () => createCustomerFormSchema(t, mode, Boolean(customer?.identity_type)),
    [customer?.identity_type, mode, t],
  );
  const countries = useMemo(() => countryOptions(locale), [locale]);
  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "party",
          label: t("customers.form.section_party"),
          description: t("customers.form.section_party_desc"),
          icon: Contact,
        },
        {
          key: "identity",
          label: t("customers.form.section_identity"),
          description: t("customers.form.section_identity_desc"),
          icon: IdCard,
        },
        {
          key: "contact",
          label: t("customers.form.section_contact"),
          description: t("customers.form.section_contact_desc"),
          icon: ShieldCheck,
        },
        {
          key: "billing",
          label: t("customers.form.section_billing"),
          description: t("customers.form.section_billing_desc"),
          icon: ReceiptText,
        },
        {
          key: "notes",
          label: t("customers.form.section_notes"),
          description: t("customers.form.section_notes_desc"),
          icon: FileText,
        },
      ]),
    [t],
  );
  const keepHint = mode === "edit" ? t("customers.form.keep_hint") : undefined;

  return (
    <AppForm schema={schema} defaultValues={defaultValues} onSubmit={onSubmit}>
      {(form) => {
        const customerType = form.watch("customer_type");
        const identityType = form.watch("identity_type");
        const corporate = customerType === "corporate";
        const identityOptions = [
          ...IDENTITY_TYPES_BY_CUSTOMER[customerType].map((value) => ({
            value,
            label: t(IDENTITY_TYPE_LABEL_KEYS[value]),
          })),
          { value: NO_IDENTITY, label: t("customers.identity_types.none") },
        ];
        return (
          <FormLayout navItems={sections.navItems}>
            <FormSection
              id={sections.id("party")}
              title={t("customers.form.section_party")}
              description={t("customers.form.section_party_desc")}
              columns={2}
            >
              <AppRadioGroup
                name="customer_type"
                label={t("customers.fields.customer_type")}
                orientation="horizontal"
                className="sm:col-span-2"
                options={CUSTOMER_TYPES.map((value) => ({
                  value,
                  label: t(CUSTOMER_TYPE_LABEL_KEYS[value]),
                }))}
              />
              {corporate ? (
                <AppInput
                  name="company_title"
                  label={t("customers.fields.company_title")}
                  className="sm:col-span-2"
                />
              ) : (
                <>
                  <AppInput
                    name="first_name"
                    label={t("customers.fields.first_name")}
                    autoComplete="off"
                  />
                  <AppInput
                    name="last_name"
                    label={t("customers.fields.last_name")}
                    autoComplete="off"
                  />
                </>
              )}
            </FormSection>

            <FormSection
              id={sections.id("identity")}
              title={t("customers.form.section_identity")}
              description={t("customers.form.section_identity_desc")}
              columns={2}
            >
              <AppSelect
                name="identity_type"
                label={t("customers.fields.identity_type")}
                options={identityOptions}
              />
              {identityType !== NO_IDENTITY ? (
                <AppInput
                  name="identity_number"
                  label={t("customers.fields.identity_number")}
                  inputMode={identityType === "passport" ? "text" : "numeric"}
                  autoComplete="off"
                  placeholder={
                    mode === "edit" && customer?.identity_type === identityType
                      ? customer.identity_masked
                      : undefined
                  }
                  description={
                    mode === "edit" && customer?.identity_type === identityType
                      ? keepHint
                      : t("customers.form.identity_hint")
                  }
                />
              ) : (
                <div />
              )}
              <AppCombobox
                name="nationality"
                label={
                  corporate
                    ? t("customers.fields.country")
                    : t("customers.fields.nationality")
                }
                options={countries}
                placeholder={t("customers.form.select_country")}
                searchPlaceholder={t("customers.form.search_country")}
                emptyText={t("customers.form.no_country")}
              />
              {corporate ? (
                <AppInput
                  name="tax_office"
                  label={t("customers.fields.tax_office")}
                />
              ) : null}
              <Alert className="sm:col-span-2">
                <ShieldCheck className="size-4" />
                <AlertDescription>
                  {t("customers.form.encryption_note")}
                </AlertDescription>
              </Alert>
            </FormSection>

            <FormSection
              id={sections.id("contact")}
              title={t("customers.form.section_contact")}
              description={t("customers.form.section_contact_desc")}
              columns={2}
            >
              <AppInput
                name="phone"
                type="tel"
                label={t("customers.fields.phone")}
                placeholder={
                  mode === "edit" && customer?.phone_masked
                    ? customer.phone_masked
                    : "0532 000 00 00"
                }
                description={
                  mode === "edit" && customer?.phone_masked
                    ? keepHint
                    : t("customers.form.phone_hint")
                }
              />
              <AppInput
                name="email"
                type="email"
                label={t("customers.fields.email")}
                placeholder={
                  mode === "edit" && customer?.email_masked
                    ? customer.email_masked
                    : undefined
                }
                description={
                  mode === "edit" && customer?.email_masked
                    ? keepHint
                    : undefined
                }
              />
            </FormSection>

            <FormSection
              id={sections.id("billing")}
              title={t("customers.form.section_billing")}
              description={t("customers.form.section_billing_desc")}
              columns={2}
            >
              <AppTextarea
                name="billing_address"
                label={t("customers.fields.billing_address")}
                className="sm:col-span-2"
              />
              <AppInput
                name="billing_city"
                label={t("customers.fields.billing_city")}
              />
              <AppInput
                name="billing_district"
                label={t("customers.fields.billing_district")}
              />
              <AppInput
                name="billing_postal_code"
                label={t("customers.fields.billing_postal_code")}
                inputMode="numeric"
              />
              <AppCombobox
                name="billing_country"
                label={t("customers.fields.billing_country")}
                options={countries}
                placeholder={t("customers.form.select_country")}
                searchPlaceholder={t("customers.form.search_country")}
                emptyText={t("customers.form.no_country")}
              />
              <AppSwitch
                name="e_invoice"
                label={t("customers.fields.e_invoice")}
                description={t("customers.form.e_invoice_hint")}
                className="sm:col-span-2"
              />
            </FormSection>

            <FormSection
              id={sections.id("notes")}
              title={t("customers.form.section_notes")}
              description={t("customers.form.section_notes_desc")}
              columns={2}
            >
              <AppInput
                name="tags"
                label={t("customers.fields.tags")}
                description={t("customers.form.tags_hint")}
                className="sm:col-span-2"
              />
              <AppTextarea
                name="note"
                label={t("customers.fields.note")}
                className="sm:col-span-2"
              />
            </FormSection>

            <FormActions>
              <Button type="button" variant="outline" onClick={onCancel}>
                {t("form.cancel")}
              </Button>
              <Button
                type="submit"
                disabled={isSubmitting || form.formState.isSubmitting}
              >
                {mode === "create"
                  ? t("customers.actions.create")
                  : t("customers.actions.save")}
              </Button>
            </FormActions>
          </FormLayout>
        );
      }}
    </AppForm>
  );
}
