"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Switch } from "@/components/ui/switch";
import {
  useSetCatalogOnline,
  useSiteCatalog,
} from "@/features/site/hooks/use-site";
import type {
  SiteCatalogBranch,
  SiteCatalogExtra,
  SiteCatalogKind,
  SiteCatalogSubgroup,
} from "@/features/site/types";
import { useLocale } from "@/providers/locale-provider";

type Row = { uuid: string; show_online: boolean };

function useOnlineColumn<T extends Row>(
  kind: SiteCatalogKind,
  canManage: boolean,
  nameField: keyof T & string,
) {
  const { t } = useLocale();
  const { mutate } = useSetCatalogOnline();
  return useMemo(
    () =>
      createColumn<T>({
        id: "show_online",
        accessorFn: (row) => String(row.show_online),
        labelKey: "site.catalog.columns.show_online",
        enableSorting: false,
        enableHiding: false,
        cell: ({ row }) => (
          <Switch
            checked={row.original.show_online}
            disabled={!canManage}
            aria-label={t("site.catalog.toggle", {
              name: String(row.original[nameField] ?? ""),
            })}
            onCheckedChange={(checked) =>
              mutate({ kind, uuid: row.original.uuid, showOnline: checked })
            }
          />
        ),
      }),
    [canManage, kind, mutate, nameField, t],
  );
}

function activeChip(t: (k: string) => string, active: boolean) {
  return (
    <StatusChip
      label={t(active ? "site.catalog.active" : "site.catalog.inactive")}
      tone={active ? "success" : "default"}
    />
  );
}

/**
 * Katalog: which branches, vehicle subgroups and extras the branded site
 * and the widget offer (live, no publish). Hidden items disappear from the
 * public API for publishable-key callers (listings, availability, quotes);
 * integrations with secret keys are not affected, and extras keep their
 * sales channels.
 */
export function SiteCatalogTab({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const catalog = useSiteCatalog();

  const branchOnline = useOnlineColumn<SiteCatalogBranch>(
    "branches",
    canManage,
    "name",
  );
  const subgroupOnline = useOnlineColumn<SiteCatalogSubgroup>(
    "products",
    canManage,
    "title",
  );
  const extraOnline = useOnlineColumn<SiteCatalogExtra>(
    "extras",
    canManage,
    "name",
  );

  const branchColumns = useMemo(
    () =>
      [
        createColumn<SiteCatalogBranch>({
          accessorKey: "name",
          labelKey: "site.catalog.columns.branch",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.name}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.code}
              </span>
            </span>
          ),
        }),
        createColumn<SiteCatalogBranch>({
          accessorKey: "city",
          labelKey: "site.catalog.columns.city",
          enableSorting: false,
          gridSecondary: true,
        }),
        createColumn<SiteCatalogBranch>({
          id: "active",
          accessorFn: (row) => String(row.active),
          labelKey: "site.catalog.columns.active",
          enableSorting: false,
          cell: ({ row }) => activeChip(t, row.original.active),
        }),
        branchOnline,
      ] as ColumnDef<SiteCatalogBranch, unknown>[],
    [branchOnline, t],
  );

  const subgroupColumns = useMemo(
    () =>
      [
        createColumn<SiteCatalogSubgroup>({
          accessorKey: "title",
          labelKey: "site.catalog.columns.subgroup",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.title}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.code}
              </span>
            </span>
          ),
        }),
        createColumn<SiteCatalogSubgroup>({
          accessorKey: "segment",
          labelKey: "site.catalog.columns.segment",
          enableSorting: false,
          gridSecondary: true,
        }),
        createColumn<SiteCatalogSubgroup>({
          id: "status",
          accessorFn: (row) => row.status,
          labelKey: "site.catalog.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`site.catalog.statuses.${row.original.status}`)}
              tone={
                row.original.status === "published" && row.original.active
                  ? "success"
                  : "default"
              }
            />
          ),
        }),
        subgroupOnline,
      ] as ColumnDef<SiteCatalogSubgroup, unknown>[],
    [subgroupOnline, t],
  );

  const extraColumns = useMemo(
    () =>
      [
        createColumn<SiteCatalogExtra>({
          accessorKey: "name",
          labelKey: "site.catalog.columns.extra",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.name}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.code}
              </span>
            </span>
          ),
        }),
        createColumn<SiteCatalogExtra>({
          id: "status",
          accessorFn: (row) => row.status,
          labelKey: "site.catalog.columns.status",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`site.catalog.statuses.${row.original.status}`)}
              tone={row.original.status === "published" ? "success" : "default"}
            />
          ),
        }),
        extraOnline,
      ] as ColumnDef<SiteCatalogExtra, unknown>[],
    [extraOnline, t],
  );

  const shared = {
    isLoading: catalog.isLoading,
    isError: catalog.isError,
    errorDescription: t("site.catalog.load_failed"),
    onRetry: () => void catalog.refetch(),
    manual: { filtering: false, sorting: false, pagination: false },
    toolbarExtra: (
      <EntityToolbar
        onRefresh={() => void catalog.refetch()}
        refreshDisabled={catalog.isFetching}
      />
    ),
  };

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <p className="text-muted-foreground text-sm">
          {t("site.catalog.description")}
        </p>
        <p className="text-muted-foreground text-xs">
          {t("site.catalog.show_online_hint")}
        </p>
      </div>
      <EntitySectionCard
        title={t("site.catalog.branches")}
        badge={catalog.data?.branches.length}
      >
        <EntityTable
          {...shared}
          columns={branchColumns}
          data={catalog.data?.branches ?? []}
          getRowId={(row) => row.uuid}
          emptyTitle={t("site.catalog.branches_empty")}
          features={{
            persistKey: "tenant-site-catalog-branches-v1",
            rowSelection: false,
          }}
        />
      </EntitySectionCard>
      <EntitySectionCard
        title={t("site.catalog.subgroups")}
        badge={catalog.data?.subgroups.length}
      >
        <EntityTable
          {...shared}
          columns={subgroupColumns}
          data={catalog.data?.subgroups ?? []}
          getRowId={(row) => row.uuid}
          emptyTitle={t("site.catalog.subgroups_empty")}
          features={{
            persistKey: "tenant-site-catalog-subgroups-v1",
            rowSelection: false,
          }}
        />
      </EntitySectionCard>
      <EntitySectionCard
        title={t("site.catalog.extras")}
        badge={catalog.data?.extras.length}
      >
        <p className="text-muted-foreground mb-3 text-xs">
          {t("site.catalog.extras_hint")}
        </p>
        <EntityTable
          {...shared}
          columns={extraColumns}
          data={catalog.data?.extras ?? []}
          getRowId={(row) => row.uuid}
          emptyTitle={t("site.catalog.extras_empty")}
          features={{
            persistKey: "tenant-site-catalog-extras-v1",
            rowSelection: false,
          }}
        />
      </EntitySectionCard>
    </div>
  );
}
