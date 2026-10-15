"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Pencil, PencilLine, Trash2 } from "lucide-react";
import { useMemo } from "react";

import { EntityRowActions, type EntityRowAction } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import {
  ConsentChips,
  CustomerName,
  CustomerTypeChip,
  MaskedValue,
  TagChips,
} from "@/features/customers/components/customer-cells";
import {
  CONSENT_FACET_LABEL_KEYS,
  CUSTOMER_TYPES,
  CUSTOMER_TYPE_LABEL_KEYS,
} from "@/features/customers/constants";
import { countryName } from "@/features/customers/lib/format";
import type { Customer, CustomerFacets } from "@/features/customers/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export type CustomerRowActionHandlers = {
  onView: (customer: Customer) => void;
  onQuickEdit: (customer: Customer) => void;
  onEdit: (customer: Customer) => void;
  onDelete: (customer: Customer) => void;
};

function CustomerRowActions({
  customer,
  handlers,
}: {
  customer: Customer;
  handlers: CustomerRowActionHandlers;
}) {
  const { t } = useLocale();
  const write = permissions.tenant.customers.write;
  const editable = !customer.anonymized;
  const actions: EntityRowAction[] = [
    {
      id: "view",
      label: t("customers.actions.view"),
      icon: Eye,
      onSelect: () => handlers.onView(customer),
    },
    ...(editable
      ? [
          {
            id: "quick-edit",
            label: t("customers.actions.quick_edit"),
            icon: PencilLine,
            permission: write,
            onSelect: () => handlers.onQuickEdit(customer),
          },
          {
            id: "edit",
            label: t("customers.actions.edit"),
            icon: Pencil,
            permission: write,
            onSelect: () => handlers.onEdit(customer),
          },
        ]
      : []),
    {
      id: "delete",
      label: t("customers.actions.delete"),
      icon: Trash2,
      permission: write,
      variant: "destructive",
      onSelect: () => handlers.onDelete(customer),
    },
  ];
  return <EntityRowActions actions={actions} />;
}

/**
 * Columns follow `GET /v1/tenant/customers/meta`. Identity, phone and
 * e-mail are the masked values; type, nationality, tag and consent are
 * server-side faceted filters.
 */
export function useCustomersColumns({
  handlers,
  facets,
}: {
  handlers: CustomerRowActionHandlers;
  facets?: CustomerFacets;
}) {
  const { t, locale } = useLocale();

  return useMemo(
    () =>
      [
        createColumn<Customer>({
          accessorKey: "display_name",
          labelKey: "customers.columns.display_name",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => <CustomerName customer={row.original} />,
        }),
        createColumn<Customer>({
          accessorKey: "customer_type",
          labelKey: "customers.columns.customer_type",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: CUSTOMER_TYPES.map((value) => {
            const count = facets?.customer_type.find(
              (f) => f.value === value,
            )?.count;
            return {
              value,
              labelKey: CUSTOMER_TYPE_LABEL_KEYS[value],
              label:
                count != null
                  ? `${t(CUSTOMER_TYPE_LABEL_KEYS[value])} (${count})`
                  : t(CUSTOMER_TYPE_LABEL_KEYS[value]),
            };
          }),
          cell: ({ row }) => (
            <CustomerTypeChip type={row.original.customer_type} />
          ),
        }),
        createColumn<Customer>({
          accessorKey: "identity_masked",
          labelKey: "customers.columns.identity_masked",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <MaskedValue value={row.original.identity_masked} />
          ),
        }),
        createColumn<Customer>({
          accessorKey: "phone_masked",
          labelKey: "customers.columns.phone_masked",
          enableSorting: false,
          cell: ({ row }) => <MaskedValue value={row.original.phone_masked} />,
        }),
        createColumn<Customer>({
          accessorKey: "email_masked",
          labelKey: "customers.columns.email_masked",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => <MaskedValue value={row.original.email_masked} />,
        }),
        createColumn<Customer>({
          id: "nationality",
          accessorFn: (row) => row.nationality ?? "",
          labelKey: "customers.columns.nationality",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: (facets?.nationality ?? []).map((facet) => ({
            value: facet.value,
            label: `${countryName(facet.value, locale)} (${facet.count})`,
          })),
          cell: ({ row }) =>
            row.original.nationality
              ? countryName(row.original.nationality, locale)
              : "—",
        }),
        createColumn<Customer>({
          id: "tag",
          accessorFn: (row) => row.tags.join(", "),
          labelKey: "customers.columns.tag",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.tag ?? []).map((facet) => ({
            value: facet.value,
            label: `${facet.value} (${facet.count})`,
          })),
          cell: ({ row }) => <TagChips tags={row.original.tags} />,
        }),
        createColumn<Customer>({
          id: "consent",
          accessorFn: (row) => row.consent_channels.join(", "),
          labelKey: "customers.columns.consent",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (facets?.consent ?? []).map((facet) => ({
            value: facet.value,
            label: `${
              CONSENT_FACET_LABEL_KEYS[facet.value]
                ? t(CONSENT_FACET_LABEL_KEYS[facet.value])
                : facet.value
            } (${facet.count})`,
          })),
          cell: ({ row }) => (
            <ConsentChips channels={row.original.consent_channels} />
          ),
        }),
        createColumn<Customer>({
          accessorKey: "drivers_count",
          labelKey: "customers.columns.drivers_count",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <Badge variant="secondary" className="tabular-nums">
              {row.original.drivers_count}
            </Badge>
          ),
        }),
        createColumn<Customer>({
          id: "billing_city",
          accessorFn: (row) => row.billing.city,
          labelKey: "customers.columns.billing_city",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.billing.city || "—",
        }),
        createColumn<Customer>({
          id: "anonymized",
          accessorFn: (row) => String(row.anonymized),
          labelKey: "customers.columns.anonymized",
          enableSorting: false,
          defaultHidden: true,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "true",
              labelKey: "customers.anonymized_filter.true",
              label: t("customers.anonymized_filter.true"),
            },
            {
              value: "false",
              labelKey: "customers.anonymized_filter.false",
              label: t("customers.anonymized_filter.false"),
            },
          ],
          cell: ({ row }) =>
            row.original.anonymized
              ? t("customers.anonymized_filter.true")
              : t("customers.anonymized_filter.false"),
        }),
        createColumn<Customer>({
          accessorKey: "created_at",
          labelKey: "customers.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<Customer>({
          accessorKey: "updated_at",
          labelKey: "customers.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.updated_at, undefined, locale),
        }),
        createColumn<Customer>({
          accessorKey: "uuid",
          labelKey: "customers.columns.uuid",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <code className="text-muted-foreground text-xs">
              {row.original.uuid}
            </code>
          ),
        }),
        createColumn<Customer>({
          id: "actions",
          labelKey: "customers.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <CustomerRowActions customer={row.original} handlers={handlers} />
          ),
        }),
      ] as ColumnDef<Customer, unknown>[],
    [facets, handlers, locale, t],
  );
}
