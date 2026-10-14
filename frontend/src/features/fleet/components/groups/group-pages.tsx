"use client";

import type { ColumnDef } from "@tanstack/react-table";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Pencil, Power, PowerOff, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDeleteDialog,
  EntityDetail,
  EntityForm,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { GroupForm } from "@/features/fleet/components/groups/group-form";
import { FleetActivityCard } from "@/features/fleet/components/shared/fleet-activity-card";
import { CodeBadge } from "@/features/fleet/components/shared/list-utils";
import { ProductsSubTable } from "@/features/fleet/components/shared/sub-tables";
import { VehiclesPlaceholder } from "@/features/fleet/components/shared/vehicles-placeholder";
import {
  BODY_TYPE_LABEL_KEYS,
  FUEL_LABEL_KEYS,
  TRANSMISSION_LABEL_KEYS,
} from "@/features/fleet/constants";
import {
  useCreateGroup,
  useDeleteGroup,
  useGroup,
  useUpdateGroup,
} from "@/features/fleet/hooks/use-fleet";
import {
  groupFormDefaults,
  groupFormToRequest,
} from "@/features/fleet/schemas/group-form";
import type { RentalGroup } from "@/features/fleet/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function Forbidden() {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_forbidden")}
      description={t("fleet.forbidden")}
    />
  );
}

export function GroupCreatePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const create = useCreateGroup();

  return (
    <EntityPage
      title={t("fleet.groups.create_title")}
      description={t("fleet.groups.create_description")}
      permission={permissions.tenant.fleet.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("fleet.groups.title"),
          href: routes.tenant.fleet.groups.root(slug),
        },
        { label: t("fleet.groups.create_title") },
      ]}
    >
      <EntityForm>
        <GroupForm
          mode="create"
          defaultValues={groupFormDefaults()}
          isSubmitting={create.isPending}
          onCancel={() => router.push(routes.tenant.fleet.groups.root(slug))}
          onSubmit={async (values) => {
            const created = await create.mutateAsync(
              groupFormToRequest(values),
            );
            router.push(routes.tenant.fleet.groups.detail(slug, created.uuid));
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}

export function GroupEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useGroup(uuid);
  const update = useUpdateGroup();
  const group = query.data;

  return (
    <EntityPage
      title={
        group
          ? t("fleet.edit_title_with_name", { name: group.name })
          : t("fleet.groups.edit_title")
      }
      description={t("fleet.groups.edit_description")}
      permission={permissions.tenant.fleet.write}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("fleet.groups.title"),
          href: routes.tenant.fleet.groups.root(slug),
        },
        {
          label: group?.name ?? t("fleet.groups.detail_title"),
          href: routes.tenant.fleet.groups.detail(slug, uuid),
        },
        { label: t("fleet.groups.edit_title") },
      ]}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("fleet.groups.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {group ? (
        <EntityForm>
          <GroupForm
            key={group.updated_at}
            mode="edit"
            defaultValues={groupFormDefaults(group)}
            isSubmitting={update.isPending}
            onCancel={() =>
              router.push(routes.tenant.fleet.groups.detail(slug, uuid))
            }
            onSubmit={async (values) => {
              await update.mutateAsync({
                uuid,
                body: groupFormToRequest(values),
              });
              router.push(routes.tenant.fleet.groups.detail(slug, uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}

type CriteriaRow = {
  uuid: string;
  kind: "model" | "type";
  label: string;
  brand: string;
};

/** Catalog membership criteria of a group (client-side DataTable). */
function CriteriaTable({ group }: { group: RentalGroup }) {
  const { t } = useLocale();
  const rows = useMemo<CriteriaRow[]>(
    () => [
      ...group.catalog_models.map((m) => ({
        uuid: m.uuid,
        kind: "model" as const,
        label: m.label,
        brand: m.brand.display_name,
      })),
      ...group.catalog_types.map((ty) => ({
        uuid: ty.uuid,
        kind: "type" as const,
        label: ty.label,
        brand: ty.brand.display_name,
      })),
    ],
    [group.catalog_models, group.catalog_types],
  );
  const columns = useMemo(
    () =>
      [
        createColumn<CriteriaRow>({
          accessorKey: "label",
          labelKey: "fleet.groups.criteria.columns.label",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.label}</span>
          ),
        }),
        createColumn<CriteriaRow>({
          accessorKey: "kind",
          labelKey: "fleet.groups.criteria.columns.kind",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: [
            {
              value: "model",
              labelKey: "fleet.groups.criteria.model",
              label: t("fleet.groups.criteria.model"),
            },
            {
              value: "type",
              labelKey: "fleet.groups.criteria.type",
              label: t("fleet.groups.criteria.type"),
            },
          ],
          cell: ({ row }) => (
            <Badge variant="outline">
              {row.original.kind === "model"
                ? t("fleet.groups.criteria.model")
                : t("fleet.groups.criteria.type")}
            </Badge>
          ),
        }),
        createColumn<CriteriaRow>({
          accessorKey: "brand",
          labelKey: "fleet.groups.criteria.columns.brand",
          enableSorting: true,
          cell: ({ row }) => row.original.brand,
        }),
      ] as ColumnDef<CriteriaRow, unknown>[],
    [t],
  );

  return (
    <EntityTable
      columns={columns}
      data={rows}
      getRowId={(row) => `${row.kind}-${row.uuid}`}
      emptyTitle={t("fleet.groups.criteria.empty_title")}
      emptyDescription={t("fleet.groups.criteria.empty_description")}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey: "tenant-fleet-group-criteria-v1",
        rowSelection: false,
      }}
    />
  );
}

/** Show page: protected attributes, catalog criteria, products, activity. */
export function GroupDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const query = useGroup(uuid);
  const update = useUpdateGroup();
  const remove = useDeleteGroup();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const group = query.data;
  const title = group?.name ?? t("fleet.groups.detail_title");
  const notGuaranteed = t("fleet.groups.form.not_guaranteed");

  return (
    <EntityPage
      title={title}
      description={t("fleet.groups.detail_description")}
      permission={permissions.tenant.fleet.read}
      forbiddenFallback={<Forbidden />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("fleet.groups.title"),
          href: routes.tenant.fleet.groups.root(slug),
        },
        { label: title },
      ]}
      actions={
        group ? (
          <PermissionGuard permission={permissions.tenant.fleet.write}>
            <EntityActions>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={update.isPending}
                onClick={() =>
                  update.mutate({
                    uuid,
                    body: { is_active: !group.is_active },
                  })
                }
              >
                {group.is_active ? (
                  <PowerOff className="size-4" />
                ) : (
                  <Power className="size-4" />
                )}
                {group.is_active
                  ? t("fleet.actions.deactivate")
                  : t("fleet.actions.activate")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2 className="size-4" />
                {t("fleet.actions.delete")}
              </Button>
              <Button asChild size="sm">
                <Link href={routes.tenant.fleet.groups.edit(slug, uuid)}>
                  <Pencil className="size-4" />
                  {t("fleet.actions.edit")}
                </Link>
              </Button>
            </EntityActions>
          </PermissionGuard>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("fleet.groups.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {group ? (
        <div className="space-y-6">
          <EntityHeader
            title={group.name}
            subtitle={
              <span className="flex flex-wrap items-center gap-2">
                <CodeBadge code={group.code} />
                <Link
                  className="underline"
                  href={routes.tenant.fleet.segments.detail(
                    slug,
                    group.segment.uuid,
                  )}
                >
                  {group.segment.name}
                </Link>
              </span>
            }
            badges={
              <StatusChip
                label={
                  group.is_active
                    ? t("fleet.active.true")
                    : t("fleet.active.false")
                }
                tone={group.is_active ? "success" : "default"}
              />
            }
            description={group.description || undefined}
          />

          <div className="grid gap-6 lg:grid-cols-3">
            <EntitySectionCard
              title={t("fleet.groups.detail.attributes")}
              className="lg:col-span-2"
            >
              <EntityDetail
                sections={[
                  {
                    id: "attributes",
                    fields: [
                      {
                        key: "transmission",
                        label: t("fleet.groups.fields.transmission"),
                        value: group.transmission
                          ? t(TRANSMISSION_LABEL_KEYS[group.transmission])
                          : notGuaranteed,
                      },
                      {
                        key: "fuel",
                        label: t("fleet.groups.fields.fuel"),
                        value: group.fuel
                          ? t(FUEL_LABEL_KEYS[group.fuel])
                          : notGuaranteed,
                      },
                      {
                        key: "body_type",
                        label: t("fleet.groups.fields.body_type"),
                        value: group.body_type
                          ? t(BODY_TYPE_LABEL_KEYS[group.body_type])
                          : notGuaranteed,
                      },
                      {
                        key: "min_seats",
                        label: t("fleet.groups.fields.min_seats"),
                        value: group.min_seats ?? notGuaranteed,
                      },
                      {
                        key: "min_luggage",
                        label: t("fleet.groups.fields.min_luggage"),
                        value: group.min_luggage ?? notGuaranteed,
                      },
                      {
                        key: "air_conditioning",
                        label: t("fleet.groups.fields.air_conditioning"),
                        value: group.air_conditioning
                          ? t("fleet.yes")
                          : notGuaranteed,
                      },
                      {
                        key: "acriss_code",
                        label: t("fleet.groups.fields.acriss_code"),
                        value: group.acriss_code ?? "—",
                      },
                      {
                        key: "updated_at",
                        label: t("fleet.fields.updated_at"),
                        value: datetime(group.updated_at, undefined, locale),
                      },
                    ],
                  },
                ]}
              />
            </EntitySectionCard>

            <EntitySectionCard title={t("fleet.groups.detail.vehicles")}>
              <VehiclesPlaceholder />
            </EntitySectionCard>
          </div>

          <EntitySectionCard
            title={t("fleet.groups.detail.criteria")}
            badge={group.catalog_models.length + group.catalog_types.length}
          >
            <CriteriaTable group={group} />
          </EntitySectionCard>

          <EntitySectionCard
            title={t("fleet.groups.detail.products")}
            badge={group.products_count}
          >
            <ProductsSubTable
              rentalGroupUuid={uuid}
              persistKey="tenant-fleet-group-products-v1"
            />
          </EntitySectionCard>

          <FleetActivityCard kind="groups" uuid={uuid} />

          <EntityDeleteDialog
            open={confirmDelete}
            entityLabel={`${group.code} · ${group.name}`}
            isPending={remove.isPending}
            onCancel={() => setConfirmDelete(false)}
            onConfirm={() =>
              remove.mutate(uuid, {
                onSuccess: () =>
                  router.push(routes.tenant.fleet.groups.root(slug)),
                onSettled: () => setConfirmDelete(false),
              })
            }
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
