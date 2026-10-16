"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ExternalLink, Plus, Trash2, Wrench } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { PeriodStatusChip } from "@/features/agreements/components/agreement-cells";
import {
  AgreementContactDialog,
  AgreementDriverDialog,
  AgreementTariffDialog,
} from "@/features/agreements/components/agreement-dialogs";
import {
  CONTACT_ROLE_LABEL_KEYS,
  ENTRY_KIND_LABEL_KEYS,
} from "@/features/agreements/constants";
import {
  useAgreementBilling,
  useAgreementRentals,
  useAgreementStatement,
  useAgreementTariffs,
  useRemoveAgreementContact,
  useRemoveAgreementDriver,
} from "@/features/agreements/hooks/use-agreements";
import type {
  Agreement,
  AgreementContact,
  AgreementDriver,
  AgreementPeriod,
  AgreementRentalRow,
  AgreementStatementRow,
  AgreementTariff,
} from "@/features/agreements/types";
import { STATUS_LABEL_KEYS, STATUS_TONES } from "@/features/pricing/constants";
import { moneyText } from "@/features/pricing/lib/format";
import { RentalStatusChip } from "@/features/rentals/components/rental-cells";
import { ReservationStatusChip } from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function SectionHint({ text }: { text: string }) {
  return <p className="text-muted-foreground mb-3 text-sm">{text}</p>;
}

const CLIENT_TABLE = {
  manual: { filtering: false, sorting: false, pagination: false },
} as const;

function tableFeatures(persistKey: string) {
  return {
    persistKey,
    rowSelection: false,
    columnFilters: false,
    facetedFilters: false,
    sorting: false,
  };
}

/** Authorised contacts and permitted drivers. */
export function AgreementPeopleTab({ agreement }: { agreement: Agreement }) {
  const { t } = useLocale();
  const removeContact = useRemoveAgreementContact();
  const removeDriver = useRemoveAgreementDriver();
  const [contactOpen, setContactOpen] = useState(false);
  const [driverOpen, setDriverOpen] = useState(false);
  const writable = agreement.status !== "terminated";

  const contactColumns = useMemo(
    () =>
      [
        createColumn<AgreementContact>({
          accessorKey: "full_name",
          labelKey: "agreements.contacts.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.full_name}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.title || "—"}
              </span>
            </span>
          ),
        }),
        createColumn<AgreementContact>({
          accessorKey: "role",
          labelKey: "agreements.contacts.columns.role",
          enableSorting: false,
          cell: ({ row }) => (
            <Badge variant="outline">
              {t(CONTACT_ROLE_LABEL_KEYS[row.original.role])}
            </Badge>
          ),
        }),
        createColumn<AgreementContact>({
          id: "reach",
          accessorFn: (row) => row.phone_masked,
          labelKey: "agreements.contacts.columns.reach",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-muted-foreground flex flex-col font-mono text-xs">
              <span>{row.original.phone_masked || "—"}</span>
              <span>{row.original.email_masked || "—"}</span>
            </span>
          ),
        }),
        createColumn<AgreementContact>({
          accessorKey: "note",
          labelKey: "agreements.contacts.columns.note",
          enableSorting: false,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<AgreementContact>({
          id: "actions",
          labelKey: "agreements.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "remove",
                  label: t("agreements.contacts.remove"),
                  icon: Trash2,
                  variant: "destructive",
                  permission: permissions.tenant.agreements.write,
                  disabled: !writable || removeContact.isPending,
                  onSelect: () =>
                    removeContact.mutate({
                      uuid: agreement.uuid,
                      contact: row.original.uuid,
                    }),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<AgreementContact, unknown>[],
    [agreement.uuid, removeContact, t, writable],
  );

  const driverColumns = useMemo(
    () =>
      [
        createColumn<AgreementDriver>({
          accessorKey: "name",
          labelKey: "agreements.drivers.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.name}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.kind === "driver"
                  ? row.original.owner
                  : t("agreements.drivers.all_drivers_of")}
              </span>
            </span>
          ),
        }),
        createColumn<AgreementDriver>({
          accessorKey: "kind",
          labelKey: "agreements.drivers.columns.kind",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.kind === "driver"
              ? t("agreements.drivers.kind_driver")
              : t("agreements.drivers.kind_customer"),
        }),
        createColumn<AgreementDriver>({
          id: "licence",
          accessorFn: (row) => row.licence_masked ?? "",
          labelKey: "agreements.drivers.columns.licence",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.licence_masked || "—"}
            </span>
          ),
        }),
        createColumn<AgreementDriver>({
          accessorKey: "note",
          labelKey: "agreements.contacts.columns.note",
          enableSorting: false,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<AgreementDriver>({
          id: "actions",
          labelKey: "agreements.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "remove",
                  label: t("agreements.drivers.remove"),
                  icon: Trash2,
                  variant: "destructive",
                  permission: permissions.tenant.agreements.write,
                  disabled: !writable || removeDriver.isPending,
                  onSelect: () =>
                    removeDriver.mutate({
                      uuid: agreement.uuid,
                      driver: row.original.uuid,
                    }),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<AgreementDriver, unknown>[],
    [agreement.uuid, removeDriver, t, writable],
  );

  return (
    <div className="space-y-4">
      <EntitySectionCard
        title={t("agreements.contacts.title")}
        badge={agreement.contacts.length}
        actions={
          writable ? (
            <PermissionGuard permission={permissions.tenant.agreements.write}>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setContactOpen(true)}
              >
                <Plus className="size-3.5" />
                {t("agreements.contacts.add")}
              </Button>
            </PermissionGuard>
          ) : null
        }
      >
        <SectionHint text={t("agreements.contacts.description")} />
        <EntityTable
          columns={contactColumns}
          data={agreement.contacts}
          getRowId={(row) => row.uuid}
          emptyTitle={t("agreements.contacts.empty")}
          emptyDescription={t("agreements.contacts.empty_description")}
          {...CLIENT_TABLE}
          features={tableFeatures("tenant-agreement-contacts-v1")}
        />
      </EntitySectionCard>
      <EntitySectionCard
        title={t("agreements.drivers.title")}
        badge={agreement.drivers.length}
        actions={
          writable ? (
            <PermissionGuard permission={permissions.tenant.agreements.write}>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setDriverOpen(true)}
              >
                <Plus className="size-3.5" />
                {t("agreements.drivers.add")}
              </Button>
            </PermissionGuard>
          ) : null
        }
      >
        <SectionHint text={t("agreements.drivers.description")} />
        <EntityTable
          columns={driverColumns}
          data={agreement.drivers}
          getRowId={(row) => row.uuid}
          emptyTitle={t("agreements.drivers.empty")}
          emptyDescription={t("agreements.drivers.empty_description")}
          {...CLIENT_TABLE}
          features={tableFeatures("tenant-agreement-drivers-v1")}
        />
      </EntitySectionCard>
      <AgreementContactDialog
        agreement={agreement}
        open={contactOpen}
        onOpenChange={setContactOpen}
      />
      <AgreementDriverDialog
        agreement={agreement}
        open={driverOpen}
        onOpenChange={setDriverOpen}
      />
    </div>
  );
}

/** The agreement's rate plans (scope agreement). */
export function AgreementTariffsTab({ agreement }: { agreement: Agreement }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useAgreementTariffs(agreement.uuid);
  const [open, setOpen] = useState(false);
  const columns = useMemo(
    () =>
      [
        createColumn<AgreementTariff>({
          id: "product",
          accessorFn: (row) => row.rental_product.code,
          labelKey: "agreements.tariffs.columns.product",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">
              {row.original.rental_product.code} ·{" "}
              {row.original.rental_product.title}
            </span>
          ),
        }),
        createColumn<AgreementTariff>({
          accessorKey: "status",
          labelKey: "agreements.tariffs.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex items-center gap-1.5">
              <StatusChip
                label={t(STATUS_LABEL_KEYS[row.original.status])}
                tone={STATUS_TONES[row.original.status]}
              />
              {row.original.has_draft ? (
                <Badge variant="outline">{t("agreements.tariffs.draft")}</Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<AgreementTariff>({
          accessorKey: "price_from",
          labelKey: "agreements.tariffs.columns.price_from",
          enableSorting: false,
          cell: ({ row }) => moneyText(row.original.price_from, locale),
        }),
        createColumn<AgreementTariff>({
          accessorKey: "band_count",
          labelKey: "agreements.tariffs.columns.bands",
          enableSorting: false,
          cell: ({ row }) => row.original.band_count,
        }),
        createColumn<AgreementTariff>({
          id: "version",
          accessorFn: (row) => row.active_version ?? 0,
          labelKey: "agreements.tariffs.columns.version",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.active_version
              ? `v${row.original.active_version}`
              : "—",
        }),
        createColumn<AgreementTariff>({
          id: "actions",
          labelKey: "agreements.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("agreements.tariffs.open"),
                  icon: ExternalLink,
                  permission: permissions.tenant.pricing.read,
                  onSelect: () =>
                    router.push(
                      routes.tenant.pricing.plans.detail(
                        slug,
                        row.original.uuid,
                      ),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<AgreementTariff, unknown>[],
    [locale, router, slug, t],
  );
  return (
    <EntitySectionCard
      title={t("agreements.tariffs.title")}
      actions={
        agreement.status !== "terminated" ? (
          <PermissionGuard permission={permissions.tenant.pricing.write}>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setOpen(true)}
            >
              <Plus className="size-3.5" />
              {t("agreements.tariffs.add")}
            </Button>
          </PermissionGuard>
        ) : null
      }
    >
      <SectionHint text={t("agreements.tariffs.description")} />
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.pricing.plans.detail(slug, row.uuid))
        }
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("agreements.tariffs.empty")}
        emptyDescription={t("agreements.tariffs.empty_description")}
        {...CLIENT_TABLE}
        features={tableFeatures("tenant-agreement-tariffs-v1")}
      />
      <AgreementTariffDialog
        agreement={agreement}
        open={open}
        onOpenChange={setOpen}
      />
    </EntitySectionCard>
  );
}

/** Reservations and rentals sold under the agreement. */
export function AgreementRentalsTab({ agreement }: { agreement: Agreement }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useAgreementRentals(agreement.uuid);
  const columns = useMemo(
    () =>
      [
        createColumn<AgreementRentalRow>({
          accessorKey: "reference",
          labelKey: "agreements.rentals.columns.reference",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-mono text-xs font-medium">
                {row.original.reference}
              </span>
              <span className="text-muted-foreground text-xs">
                {row.original.customer.display_name}
              </span>
            </span>
          ),
        }),
        createColumn<AgreementRentalRow>({
          accessorKey: "status",
          labelKey: "agreements.rentals.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1">
              <ReservationStatusChip
                status={row.original.status as "confirmed"}
              />
              {row.original.rental ? (
                <RentalStatusChip
                  status={row.original.rental.status as "active"}
                />
              ) : null}
              {row.original.open_account ? (
                <Badge variant="outline">
                  {t("agreements.rentals.open_account")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<AgreementRentalRow>({
          id: "range",
          accessorFn: (row) => row.starts_at,
          labelKey: "agreements.rentals.columns.range",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-xs tabular-nums">
              {datetime(row.original.starts_at, "d MMM yyyy HH:mm", locale)} →{" "}
              {datetime(row.original.ends_at, "d MMM yyyy HH:mm", locale)}
            </span>
          ),
        }),
        createColumn<AgreementRentalRow>({
          id: "vehicle",
          accessorFn: (row) => row.plate,
          labelKey: "agreements.rentals.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col text-xs">
              <span className="font-mono font-medium">
                {row.original.plate || "—"}
              </span>
              <span className="text-muted-foreground">
                {row.original.product.code}
              </span>
            </span>
          ),
        }),
        createColumn<AgreementRentalRow>({
          id: "billing",
          accessorFn: (row) => row.rental?.rent_billing ?? "",
          labelKey: "agreements.rentals.columns.billing",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.rental
              ? row.original.rental.rent_billing === "periodic"
                ? t("agreements.rentals.billing_periodic")
                : t("agreements.rentals.billing_handover")
              : "—",
        }),
        createColumn<AgreementRentalRow>({
          accessorKey: "total",
          labelKey: "agreements.rentals.columns.total",
          enableSorting: false,
          cell: ({ row }) => moneyText(row.original.total, locale),
        }),
        createColumn<AgreementRentalRow>({
          accessorKey: "rent_accrued",
          labelKey: "agreements.rentals.columns.accrued",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col text-xs">
              <span>{moneyText(row.original.rent_accrued, locale)}</span>
              {Number(row.original.rent_open) > 0 ? (
                <span className="text-muted-foreground">
                  {t("agreements.rentals.open_amount", {
                    amount: moneyText(row.original.rent_open, locale),
                  })}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<AgreementRentalRow>({
          id: "actions",
          labelKey: "agreements.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "reservation",
                  label: t("agreements.rentals.open_reservation"),
                  icon: ExternalLink,
                  permission: permissions.tenant.reservations.read,
                  onSelect: () =>
                    router.push(
                      routes.tenant.reservations.detail(
                        slug,
                        row.original.reservation_uuid,
                      ),
                    ),
                },
                ...(row.original.rental
                  ? [
                      {
                        id: "rental",
                        label: t("agreements.rentals.open_rental"),
                        icon: ExternalLink,
                        permission: permissions.tenant.rentals.read,
                        onSelect: () =>
                          router.push(
                            routes.tenant.rentals.detail(
                              slug,
                              row.original.rental!.uuid,
                            ),
                          ),
                      },
                    ]
                  : []),
              ]}
            />
          ),
        }),
      ] as ColumnDef<AgreementRentalRow, unknown>[],
    [locale, router, slug, t],
  );
  return (
    <EntitySectionCard
      title={t("agreements.rentals.title")}
      badge={query.data?.items.length}
    >
      <SectionHint text={t("agreements.rentals.description")} />
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.reservation_uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("agreements.rentals.empty")}
        emptyDescription={t("agreements.rentals.empty_description")}
        {...CLIENT_TABLE}
        features={tableFeatures("tenant-agreement-rentals-v1")}
      />
    </EntitySectionCard>
  );
}

/** Billing plans of the agreement's periodically billed rentals. */
export function AgreementBillingTab({ agreement }: { agreement: Agreement }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const query = useAgreementBilling(agreement.uuid);
  const columns = useMemo(
    () =>
      [
        createColumn<AgreementPeriod>({
          id: "rental",
          accessorFn: (row) => row.rental.name,
          labelKey: "agreements.billing.columns.rental",
          enableSorting: false,
          cell: ({ row }) => (
            <Link
              className="font-mono text-xs underline-offset-2 hover:underline"
              href={routes.tenant.rentals.detail(
                slug,
                row.original.rental.uuid,
              )}
            >
              {row.original.rental.name}
            </Link>
          ),
        }),
        createColumn<AgreementPeriod>({
          accessorKey: "no",
          labelKey: "agreements.billing.columns.no",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-1 tabular-nums">
              {row.original.no}
              {row.original.revision > 1 ? (
                <Badge variant="outline" className="px-1 py-0 text-[10px]">
                  {t("agreements.billing.revision", {
                    revision: row.original.revision,
                  })}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<AgreementPeriod>({
          id: "range",
          accessorFn: (row) => row.starts_on,
          labelKey: "agreements.billing.columns.range",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1 text-xs tabular-nums">
              {date(row.original.starts_on, undefined, locale)} –{" "}
              {date(row.original.ends_on, undefined, locale)}
              {row.original.partial ? (
                <Badge variant="warning" className="px-1 py-0 text-[10px]">
                  {t("agreements.billing.partial")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<AgreementPeriod>({
          accessorKey: "days",
          labelKey: "agreements.billing.columns.days",
          enableSorting: false,
          cell: ({ row }) => row.original.days,
        }),
        createColumn<AgreementPeriod>({
          accessorKey: "amount",
          labelKey: "agreements.billing.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.amount, locale)}
            </span>
          ),
        }),
        createColumn<AgreementPeriod>({
          id: "km",
          accessorFn: (row) => row.km_allowance ?? 0,
          labelKey: "agreements.billing.columns.km",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex items-center gap-1 text-xs">
              {row.original.km_allowance != null
                ? t("agreements.billing.km", {
                    km: row.original.km_allowance.toLocaleString(locale),
                  })
                : "—"}
              {row.original.maintenance_due ? (
                <Badge
                  variant="outline"
                  className="gap-1 px-1 py-0 text-[10px]"
                >
                  <Wrench className="size-3" aria-hidden />
                  {t("agreements.billing.maintenance")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<AgreementPeriod>({
          accessorKey: "status",
          labelKey: "agreements.billing.columns.status",
          enableSorting: false,
          cell: ({ row }) => <PeriodStatusChip status={row.original.status} />,
        }),
        createColumn<AgreementPeriod>({
          id: "entry",
          accessorFn: (row) => row.entry_uuid ?? "",
          labelKey: "agreements.billing.columns.entry",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.entry_uuid ? (
              <span className="flex flex-col text-xs">
                <span className="font-mono">
                  {row.original.entry_uuid.slice(0, 8)}
                </span>
                <span className="text-muted-foreground">
                  {row.original.due_date
                    ? t("agreements.billing.due", {
                        date: date(row.original.due_date, undefined, locale),
                      })
                    : ""}
                </span>
              </span>
            ) : (
              "—"
            ),
        }),
      ] as ColumnDef<AgreementPeriod, unknown>[],
    [locale, slug, t],
  );
  const data = query.data;
  return (
    <EntitySectionCard title={t("agreements.billing.title")}>
      <SectionHint text={t("agreements.billing.description")} />
      {data ? (
        <div className="mb-4 flex flex-wrap gap-2 text-sm">
          <Badge variant="success">
            {t("agreements.billing.posted_total", {
              amount: moneyText(data.posted, locale),
            })}
          </Badge>
          <Badge variant="outline">
            {t("agreements.billing.planned_total", {
              amount: moneyText(data.planned, locale),
            })}
          </Badge>
        </div>
      ) : null}
      <EntityTable
        columns={columns}
        data={data?.periods ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("agreements.billing.empty")}
        emptyDescription={t("agreements.billing.empty_description")}
        {...CLIENT_TABLE}
        features={tableFeatures("tenant-agreement-billing-v1")}
      />
    </EntitySectionCard>
  );
}

/** Ledger entries of the agreement's sales with a running balance. */
export function AgreementStatementTab({ agreement }: { agreement: Agreement }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const query = useAgreementStatement(agreement.uuid);
  const columns = useMemo(
    () =>
      [
        createColumn<AgreementStatementRow>({
          accessorKey: "entry_date",
          labelKey: "agreements.statement.columns.date",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col text-xs tabular-nums">
              <span>{date(row.original.entry_date, undefined, locale)}</span>
              <span className="text-muted-foreground">
                {t("agreements.billing.due", {
                  date: date(row.original.due_date, undefined, locale),
                })}
              </span>
            </span>
          ),
        }),
        createColumn<AgreementStatementRow>({
          accessorKey: "kind",
          labelKey: "agreements.statement.columns.kind",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1">
              {t(
                ENTRY_KIND_LABEL_KEYS[row.original.kind] ??
                  "agreements.entry_kinds.other",
              )}
              {row.original.periodic ? (
                <Badge variant="outline" className="px-1 py-0 text-[10px]">
                  {t("agreements.statement.periodic")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<AgreementStatementRow>({
          id: "reference",
          accessorFn: (row) => row.reservation.name,
          labelKey: "agreements.statement.columns.reference",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col text-xs">
              <span className="font-mono">{row.original.reservation.name}</span>
              <span className="text-muted-foreground">
                {row.original.description || row.original.reason || ""}
              </span>
            </span>
          ),
        }),
        createColumn<AgreementStatementRow>({
          accessorKey: "debit",
          labelKey: "agreements.statement.columns.debit",
          enableSorting: false,
          cell: ({ row }) =>
            Number(row.original.debit) > 0
              ? moneyText(row.original.debit, locale)
              : "",
        }),
        createColumn<AgreementStatementRow>({
          accessorKey: "credit",
          labelKey: "agreements.statement.columns.credit",
          enableSorting: false,
          cell: ({ row }) =>
            Number(row.original.credit) > 0
              ? moneyText(row.original.credit, locale)
              : "",
        }),
        createColumn<AgreementStatementRow>({
          accessorKey: "balance",
          labelKey: "agreements.statement.columns.balance",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {moneyText(row.original.balance, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<AgreementStatementRow, unknown>[],
    [locale, t],
  );
  const data = query.data;
  return (
    <EntitySectionCard
      title={t("agreements.statement.title")}
      actions={
        agreement.account ? (
          <PermissionGuard
            permission={permissions.tenant.finance.accounts.read}
          >
            <Button asChild size="sm" variant="outline">
              <Link
                href={routes.tenant.finance.account(
                  slug,
                  agreement.account.uuid,
                )}
              >
                <ExternalLink className="size-3.5" />
                {t("agreements.statement.open_account")}
              </Link>
            </Button>
          </PermissionGuard>
        ) : null
      }
    >
      <SectionHint text={t("agreements.statement.description")} />
      {data ? (
        <div className="mb-4 flex flex-wrap gap-2 text-sm">
          <Badge variant="outline">
            {t("agreements.statement.debit_total", {
              amount: moneyText(data.debit, locale),
            })}
          </Badge>
          <Badge variant="outline">
            {t("agreements.statement.credit_total", {
              amount: moneyText(data.credit, locale),
            })}
          </Badge>
          <Badge variant={Number(data.balance) > 0 ? "warning" : "success"}>
            {t("agreements.statement.balance_total", {
              amount: moneyText(data.balance, locale),
            })}
          </Badge>
        </div>
      ) : null}
      <EntityTable
        columns={columns}
        data={data?.rows ?? []}
        getRowId={(row) => row.entry_uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("agreements.statement.empty")}
        emptyDescription={t("agreements.statement.empty_description")}
        {...CLIENT_TABLE}
        features={tableFeatures("tenant-agreement-statement-v1")}
      />
    </EntitySectionCard>
  );
}
