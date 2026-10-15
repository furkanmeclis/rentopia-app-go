"use client";

import { Eye } from "lucide-react";

import { EntityDetail, EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  ConsentChips,
  CustomerTypeChip,
  TagChips,
} from "@/features/customers/components/customer-cells";
import { CustomerContactsCard } from "@/features/customers/components/customer-contacts-card";
import {
  RevealBanner,
  SensitiveValue,
  useCustomerReveal,
} from "@/features/customers/components/customer-reveal";
import { IDENTITY_TYPE_LABEL_KEYS } from "@/features/customers/constants";
import { countryName } from "@/features/customers/lib/format";
import type { Customer } from "@/features/customers/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * "Genel" tab: party and identity (masked; "Göster" asks a reason and shows
 * the values for a limited time), contact, billing, tags and note, and the
 * contacts of a corporate customer.
 */
export function CustomerGeneralTab({ customer }: { customer: Customer }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const { revealed, open } = useCustomerReveal();
  const canReveal =
    can(permissions.tenant.customers.reveal) && !customer.anonymized;
  const corporate = customer.customer_type === "corporate";

  const revealButton =
    canReveal && !revealed ? (
      <Button type="button" size="sm" variant="outline" onClick={open}>
        <Eye className="size-4" />
        {t("customers.reveal.button")}
      </Button>
    ) : null;

  return (
    <div className="space-y-6">
      <RevealBanner />
      <div className="grid gap-6 lg:grid-cols-2">
        <EntitySectionCard
          title={t("customers.detail.identity")}
          actions={revealButton}
        >
          <EntityDetail
            sections={[
              {
                id: "identity",
                fields: [
                  {
                    key: "type",
                    label: t("customers.fields.customer_type"),
                    value: <CustomerTypeChip type={customer.customer_type} />,
                  },
                  ...(corporate
                    ? [
                        {
                          key: "company_title",
                          label: t("customers.fields.company_title"),
                          value: customer.company_title || "—",
                        },
                        {
                          key: "tax_office",
                          label: t("customers.fields.tax_office"),
                          value: customer.tax_office || "—",
                        },
                      ]
                    : [
                        {
                          key: "first_name",
                          label: t("customers.fields.first_name"),
                          value: customer.first_name || "—",
                        },
                        {
                          key: "last_name",
                          label: t("customers.fields.last_name"),
                          value: customer.last_name || "—",
                        },
                      ]),
                  {
                    key: "identity_type",
                    label: t("customers.fields.identity_type"),
                    value: customer.identity_type
                      ? t(IDENTITY_TYPE_LABEL_KEYS[customer.identity_type])
                      : "—",
                  },
                  {
                    key: "identity_number",
                    label: t("customers.fields.identity_number"),
                    value: (
                      <SensitiveValue
                        masked={customer.identity_masked}
                        plain={revealed?.identity_number}
                      />
                    ),
                  },
                  {
                    key: "nationality",
                    label: corporate
                      ? t("customers.fields.country")
                      : t("customers.fields.nationality"),
                    value: customer.nationality
                      ? countryName(customer.nationality, locale)
                      : "—",
                  },
                ],
              },
            ]}
          />
        </EntitySectionCard>

        <EntitySectionCard title={t("customers.detail.contact")}>
          <EntityDetail
            sections={[
              {
                id: "contact",
                fields: [
                  {
                    key: "phone",
                    label: t("customers.fields.phone"),
                    value: (
                      <SensitiveValue
                        masked={customer.phone_masked}
                        plain={revealed?.phone}
                      />
                    ),
                  },
                  {
                    key: "email",
                    label: t("customers.fields.email"),
                    value: (
                      <SensitiveValue
                        masked={customer.email_masked}
                        plain={revealed?.email}
                      />
                    ),
                  },
                  {
                    key: "consents",
                    label: t("customers.fields.consents"),
                    value: (
                      <ConsentChips channels={customer.consent_channels} />
                    ),
                  },
                  {
                    key: "tags",
                    label: t("customers.fields.tags"),
                    value: <TagChips tags={customer.tags} />,
                  },
                  {
                    key: "updated_at",
                    label: t("customers.fields.updated_at"),
                    value: datetime(customer.updated_at, undefined, locale),
                  },
                ],
              },
            ]}
          />
        </EntitySectionCard>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <EntitySectionCard title={t("customers.detail.billing")}>
          <EntityDetail
            sections={[
              {
                id: "billing",
                fields: [
                  {
                    key: "address",
                    label: t("customers.fields.billing_address"),
                    value: customer.billing.address || "—",
                  },
                  {
                    key: "city",
                    label: t("customers.fields.billing_city"),
                    value:
                      [customer.billing.district, customer.billing.city]
                        .filter(Boolean)
                        .join(" / ") || "—",
                  },
                  {
                    key: "postal_code",
                    label: t("customers.fields.billing_postal_code"),
                    value: customer.billing.postal_code || "—",
                  },
                  {
                    key: "country",
                    label: t("customers.fields.billing_country"),
                    value: customer.billing.country
                      ? countryName(customer.billing.country, locale)
                      : "—",
                  },
                  {
                    key: "e_invoice",
                    label: t("customers.fields.e_invoice"),
                    value: customer.billing.e_invoice
                      ? t("customers.yes")
                      : t("customers.no"),
                  },
                ],
              },
            ]}
          />
        </EntitySectionCard>
        <EntitySectionCard title={t("customers.fields.note")}>
          <p className="text-sm whitespace-pre-wrap">
            {customer.note || (
              <span className="text-muted-foreground">
                {t("customers.detail.no_note")}
              </span>
            )}
          </p>
        </EntitySectionCard>
      </div>

      {corporate ? (
        <CustomerContactsCard
          customerUuid={customer.uuid}
          readOnly={customer.anonymized}
        />
      ) : null}
    </div>
  );
}
