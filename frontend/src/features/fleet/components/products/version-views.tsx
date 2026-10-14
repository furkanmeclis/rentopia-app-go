"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityDetail,
  type EntityDetailField,
  EntityRowActions,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { routes } from "@/config/routes";
import {
  FUEL_POLICY_LABEL_KEYS,
  GUARANTEE_LABEL_KEYS,
  VARIANT_LEVEL_LABEL_KEYS,
} from "@/features/fleet/constants";
import {
  useProductVersion,
  useProductVersions,
} from "@/features/fleet/hooks/use-fleet";
import type {
  ProductConditions,
  ProductGuarantee,
  ProductVersion,
} from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useVehicle } from "@/features/vehicles/hooks/use-vehicles";
import { datetime, money } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Plate + model of the EXACT_VEHICLE vehicle, linked to its page. */
function GuaranteedVehicle({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const query = useVehicle(uuid);
  if (query.isLoading) return <span className="text-muted-foreground">…</span>;
  if (!query.data) {
    return (
      <span className="text-muted-foreground">
        {t("fleet.products.fields.vehicle_missing")}
      </span>
    );
  }
  return (
    <Link
      className="underline"
      href={routes.tenant.fleet.vehicles.detail(slug, uuid)}
    >
      {query.data.plate}
      {query.data.label ? ` · ${query.data.label}` : ""}
    </Link>
  );
}

/** Read-only guarantee of a version. */
export function GuaranteeView({ guarantee }: { guarantee: ProductGuarantee }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const fields: EntityDetailField[] = [
    {
      key: "type",
      label: t("fleet.products.fields.guarantee_type"),
      value: t(GUARANTEE_LABEL_KEYS[guarantee.type]),
    },
  ];
  if (guarantee.type === "GROUP") {
    fields.push({
      key: "group",
      label: t("fleet.products.fields.rental_group"),
      value: guarantee.rental_group ? (
        <Link
          className="underline"
          href={routes.tenant.fleet.groups.detail(
            slug,
            guarantee.rental_group.uuid,
          )}
        >
          {guarantee.rental_group.code} · {guarantee.rental_group.name}
        </Link>
      ) : (
        guarantee.label || "—"
      ),
    });
  }
  if (guarantee.type === "EXACT_VARIANT") {
    fields.push(
      {
        key: "level",
        label: t("fleet.products.fields.variant_level"),
        value: guarantee.variant_level
          ? t(VARIANT_LEVEL_LABEL_KEYS[guarantee.variant_level])
          : "—",
      },
      {
        key: "variant",
        label:
          guarantee.variant_level === "type"
            ? t("fleet.products.fields.guaranteed_type")
            : t("fleet.products.fields.guaranteed_model"),
        value:
          (guarantee.variant_level === "type"
            ? guarantee.catalog_type?.label
            : guarantee.catalog_model?.label) ||
          guarantee.label ||
          "—",
      },
    );
  }
  if (guarantee.type === "EXACT_VEHICLE") {
    fields.push({
      key: "vehicle",
      label: t("fleet.products.fields.vehicle_id"),
      value: guarantee.vehicle_id ? (
        <GuaranteedVehicle uuid={guarantee.vehicle_id} />
      ) : (
        t("fleet.products.fields.vehicle_unset")
      ),
    });
  }
  return <EntityDetail sections={[{ id: "guarantee", fields }]} />;
}

/** Read-only rental conditions of a version. */
export function ConditionsView({
  conditions,
}: {
  conditions: ProductConditions;
}) {
  const { t, locale } = useLocale();
  return (
    <EntityDetail
      sections={[
        {
          id: "conditions",
          fields: [
            {
              key: "deposit",
              label: t("fleet.products.fields.deposit_amount"),
              value: money(Number(conditions.deposit_amount), locale),
            },
            {
              key: "driver_age",
              label: t("fleet.products.fields.min_driver_age"),
              value: conditions.min_driver_age,
            },
            {
              key: "license_years",
              label: t("fleet.products.fields.min_license_years"),
              value: conditions.min_license_years,
            },
            {
              key: "km",
              label: t("fleet.products.fields.included_km_per_day"),
              value:
                conditions.included_km_per_day == null
                  ? t("fleet.products.fields.unlimited_km")
                  : t("fleet.products.km_value", {
                      km: conditions.included_km_per_day,
                    }),
            },
            {
              key: "extra_km",
              label: t("fleet.products.fields.extra_km_fee"),
              value: money(Number(conditions.extra_km_fee), locale),
            },
            {
              key: "fuel_policy",
              label: t("fleet.products.fields.fuel_policy"),
              value: t(FUEL_POLICY_LABEL_KEYS[conditions.fuel_policy]),
            },
            {
              key: "notes",
              label: t("fleet.products.fields.condition_notes"),
              value: conditions.notes || "—",
            },
          ],
        },
      ]}
    />
  );
}

function actorLabel(version: ProductVersion) {
  if (!version.created_by) return "—";
  const full =
    `${version.created_by.name} ${version.created_by.surname}`.trim();
  return full || version.created_by.email;
}

/** Read-only dialog of one version. */
function VersionDialog({
  productUuid,
  versionUuid,
  onOpenChange,
}: {
  productUuid: string;
  versionUuid: string | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const query = useProductVersion(productUuid, versionUuid);
  const version = query.data;
  return (
    <AppDialog
      open={Boolean(versionUuid)}
      onOpenChange={onOpenChange}
      title={
        version
          ? t("fleet.versions.dialog_title", { version: version.version })
          : t("fleet.versions.title")
      }
      description={t("fleet.versions.read_only")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-2xl"
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {version ? (
        <div className="space-y-6">
          <div className="text-muted-foreground flex flex-wrap items-center gap-2 text-sm">
            {version.is_current ? (
              <StatusChip label={t("fleet.versions.current")} tone="success" />
            ) : null}
            <span>{datetime(version.created_at, undefined, locale)}</span>
            <span>· {actorLabel(version)}</span>
          </div>
          {version.change_note ? (
            <p className="text-sm">{version.change_note}</p>
          ) : null}
          <section className="space-y-2">
            <h3 className="text-sm font-semibold">
              {t("fleet.products.tabs.guarantee")}
            </h3>
            <GuaranteeView guarantee={version.guarantee} />
          </section>
          <section className="space-y-2">
            <h3 className="text-sm font-semibold">
              {t("fleet.products.tabs.conditions")}
            </h3>
            <ConditionsView conditions={version.conditions} />
          </section>
        </div>
      ) : null}
    </AppDialog>
  );
}

/** Versions of a product (server-side DataTable sub-list, newest first). */
export function VersionsTable({ productUuid }: { productUuid: string }) {
  const { t, locale } = useLocale();
  const listState = useServerListState({
    initialSort: "-version",
    initialPageSize: 10,
  });
  const query = useProductVersions(productUuid, {
    limit: listState.params.limit,
    offset: listState.params.offset,
  });
  const [open, setOpen] = useState<string | null>(null);
  const total = query.data?.total ?? 0;

  const columns = useMemo(
    () =>
      [
        createColumn<ProductVersion>({
          accessorKey: "version",
          labelKey: "fleet.versions.columns.version",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex items-center gap-2">
              <Badge variant="outline" className="tabular-nums">
                v{row.original.version}
              </Badge>
              {row.original.is_current ? (
                <StatusChip
                  label={t("fleet.versions.current")}
                  tone="success"
                />
              ) : null}
            </span>
          ),
        }),
        createColumn<ProductVersion>({
          id: "guarantee",
          accessorFn: (row) => row.guarantee.type,
          labelKey: "fleet.versions.columns.guarantee",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="space-y-0.5">
              <div>{t(GUARANTEE_LABEL_KEYS[row.original.guarantee.type])}</div>
              {row.original.guarantee.label ? (
                <div className="text-muted-foreground text-xs">
                  {row.original.guarantee.label}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<ProductVersion>({
          id: "deposit",
          accessorFn: (row) => row.conditions.deposit_amount,
          labelKey: "fleet.versions.columns.deposit",
          enableSorting: false,
          cell: ({ row }) =>
            money(Number(row.original.conditions.deposit_amount), locale),
        }),
        createColumn<ProductVersion>({
          id: "driver",
          accessorFn: (row) => row.conditions.min_driver_age,
          labelKey: "fleet.versions.columns.driver",
          enableSorting: false,
          cell: ({ row }) =>
            t("fleet.versions.driver_value", {
              age: row.original.conditions.min_driver_age,
              years: row.original.conditions.min_license_years,
            }),
        }),
        createColumn<ProductVersion>({
          accessorKey: "change_note",
          labelKey: "fleet.versions.columns.change_note",
          enableSorting: false,
          cell: ({ row }) => row.original.change_note || "—",
        }),
        createColumn<ProductVersion>({
          id: "created_by",
          accessorFn: (row) => actorLabel(row),
          labelKey: "fleet.versions.columns.created_by",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => actorLabel(row.original),
        }),
        createColumn<ProductVersion>({
          accessorKey: "created_at",
          labelKey: "fleet.versions.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<ProductVersion>({
          id: "actions",
          labelKey: "fleet.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "view",
                  label: t("fleet.actions.view"),
                  icon: Eye,
                  onSelect: () => setOpen(row.original.uuid),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<ProductVersion, unknown>[],
    [locale, t],
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(version) => setOpen(version.uuid)}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("fleet.versions.empty")}
        pageCount={Math.max(
          1,
          Math.ceil(total / listState.pagination.pageSize),
        )}
        state={{
          pagination: listState.pagination,
          onPaginationChange: listState.onPaginationChange,
        }}
        features={{
          persistKey: "tenant-fleet-product-versions-v1",
          rowSelection: false,
          globalFilter: false,
          columnFilters: false,
          facetedFilters: false,
          sorting: false,
        }}
      />
      <VersionDialog
        productUuid={productUuid}
        versionUuid={open}
        onOpenChange={(next) => {
          if (!next) setOpen(null);
        }}
      />
    </>
  );
}
