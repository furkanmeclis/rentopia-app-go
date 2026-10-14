"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Keyboard, Plus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PermissionGuard } from "@/components/common/permission-guard";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CatalogTabs,
  useCatalogBreadcrumbs,
} from "@/features/catalog/components/catalog-tabs";
import {
  ModelCreateDialog,
  useBrandModelOptions,
} from "@/features/catalog/components/model-dialogs";
import {
  CATALOG_CATEGORIES,
  firstFilterValue,
  formatYears,
} from "@/features/catalog/constants";
import {
  useCatalogBrandOptions,
  useCatalogTypes,
  usePatchType,
} from "@/features/catalog/hooks/use-catalog";
import type {
  CatalogModel,
  CatalogType,
  ListTypesParams,
} from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

const assignId = (index: number) => `catalog-queue-assign-${index}`;

/** First words of a type name as a model name suggestion. */
function suggestName(raw: string) {
  return raw.split(/\s+/).slice(0, 2).join(" ");
}

/** Whether a rule "type name starts with <name>" fits this type. */
function startsWithName(raw: string, name: string) {
  const a = raw.toLocaleUpperCase("tr").replace(/\s+/g, " ").trim();
  const b = name.toLocaleUpperCase("tr").replace(/\s+/g, " ").trim();
  return b.length > 0 && (a === b || a.startsWith(`${b} `));
}

type AssignProps = {
  type: CatalogType;
  index: number;
  saveRule: boolean;
  onAssigned: (index: number) => void;
};

/**
 * Keyboard-first model picker for one queue row: Enter opens it, typing
 * searches the display brand's models, arrows + Enter assign. With "save
 * rule" on, the model name is stored as a prefix rule when it fits.
 */
function QueueAssignCell({ type, index, saveRule, onAssigned }: AssignProps) {
  const { t } = useLocale();
  const patch = usePatchType();
  const [value, setValue] = useState("");
  const { loadOptions, options } = useBrandModelOptions({
    displayBrand: type.display_brand.uuid,
  });
  return (
    <div className="w-64" onClick={(event) => event.stopPropagation()}>
      <AsyncCombobox
        id={assignId(index)}
        value={value}
        disabled={patch.isPending}
        loadOptions={loadOptions}
        options={options}
        placeholder={t("catalog.queue.assign_placeholder")}
        searchPlaceholder={t("catalog.picker.search")}
        emptyText={t("catalog.picker.empty")}
        onValueChange={async (next) => {
          if (!next) return;
          setValue(next);
          const name = options.find((o) => o.value === next)?.label ?? "";
          const rule = saveRule && startsWithName(type.raw_name, name);
          try {
            await patch.mutateAsync({
              uuid: type.uuid,
              body: { model_uuid: next, ...(rule ? { save_rule: true } : {}) },
            });
            onAssigned(index);
          } catch {
            setValue("");
          }
        }}
      />
    </div>
  );
}

export function CatalogUnresolvedQueuePage() {
  const { t } = useLocale();
  const router = useRouter();
  const breadcrumbs = useCatalogBreadcrumbs(t("catalog.tabs.unresolved"));
  const [saveRule, setSaveRule] = useState(true);
  const [creatingFor, setCreatingFor] = useState<CatalogType | null>(null);
  const patch = usePatchType();
  const focusNext = useRef<number | null>(null);
  const listState = useServerListState({
    initialSort: "brand",
    initialPageSize: 50,
  });
  const brands = useCatalogBrandOptions();
  const filters = listState.columnFilters;

  const params = useMemo<ListTypesParams>(
    () => ({
      ...listState.params,
      q: listState.params.q || firstFilterValue(filters, "raw_name"),
      brand: firstFilterValue(filters, "brand"),
      category: firstFilterValue(filters, "category"),
      unresolved: "true",
      is_active: "true",
    }),
    [filters, listState.params],
  );
  const listQuery = useCatalogTypes(params);
  const items = listQuery.data?.items;

  // After an assignment the row leaves the queue: move focus to the row
  // that took its place so the next type can be assigned right away.
  useEffect(() => {
    const index = focusNext.current;
    if (index == null || !items) return;
    focusNext.current = null;
    const target =
      document.getElementById(assignId(index)) ??
      document.getElementById(assignId(Math.max(0, items.length - 1)));
    target?.focus();
  }, [items]);

  const brandOptions = useMemo(
    () =>
      (brands.data ?? []).map((b) => ({
        value: b.uuid,
        label: `${b.display_name} (${b.tsb_name})`,
      })),
    [brands.data],
  );

  const columns = useMemo(
    () =>
      [
        createColumn<CatalogType>({
          accessorKey: "raw_name",
          labelKey: "catalog.columns.raw_name",
          enableSorting: true,
          filterVariant: "text",
          gridPrimary: true,
          size: 340,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.raw_name}</span>
          ),
        }),
        createColumn<CatalogType>({
          id: "brand",
          accessorFn: (row) => row.brand.uuid,
          labelKey: "catalog.columns.brand",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: brandOptions,
          gridSecondary: true,
          cell: ({ row }) => (
            <span>
              {row.original.display_brand.name}
              {row.original.brand.tsb_code ? (
                <span className="text-muted-foreground ms-1 font-mono text-xs">
                  {row.original.brand.tsb_code}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<CatalogType>({
          id: "category",
          accessorFn: () => "",
          labelKey: "catalog.columns.category",
          filterVariant: "faceted",
          defaultHidden: true,
          enableHiding: false,
          filterOptions: CATALOG_CATEGORIES.map((value) => ({
            value,
            label: value,
            labelKey: `catalog.category.${value}`,
          })),
          cell: () => null,
        }),
        createColumn<CatalogType>({
          id: "years",
          accessorFn: (row) => formatYears(row.years),
          labelKey: "catalog.columns.years",
          cell: ({ row }) => (
            <span className="tabular-nums">
              {formatYears(row.original.years)}
            </span>
          ),
        }),
        createColumn<CatalogType>({
          id: "assign",
          labelKey: "catalog.queue.assign",
          enableSorting: false,
          enableHiding: false,
          size: 280,
          cell: ({ row }) => (
            <PermissionGuard permission={permissions.catalog.write}>
              <QueueAssignCell
                type={row.original}
                index={row.index}
                saveRule={saveRule}
                onAssigned={(index) => {
                  focusNext.current = index;
                }}
              />
            </PermissionGuard>
          ),
        }),
        createColumn<CatalogType>({
          id: "actions",
          labelKey: "catalog.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "create",
                  label: t("catalog.queue.create_model"),
                  icon: Plus,
                  permission: permissions.catalog.write,
                  onSelect: () => setCreatingFor(row.original),
                },
                {
                  id: "view",
                  label: t("catalog.actions.view"),
                  icon: Eye,
                  onSelect: () =>
                    router.push(
                      routes.platform.catalog.typeDetail(row.original.uuid),
                    ),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<CatalogType, unknown>[],
    [brandOptions, router, saveRule, t],
  );

  const pageCount = Math.max(
    1,
    Math.ceil(
      (listQuery.data?.total ?? 0) / (listState.pagination.pageSize || 50),
    ),
  );

  return (
    <EntityPage
      title={t("catalog.title")}
      description={t("catalog.description")}
      permission={permissions.catalog.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.forbidden")}
        />
      }
      breadcrumbs={breadcrumbs}
    >
      <CatalogTabs active="unresolved" />
      <Alert className="mb-4">
        <Keyboard className="size-4" />
        <AlertTitle>
          {t("catalog.queue.title", { count: listQuery.data?.total ?? 0 })}
        </AlertTitle>
        <AlertDescription>{t("catalog.queue.keyboard_hint")}</AlertDescription>
      </Alert>
      <EntityTable
        columns={columns}
        data={items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("catalog.queue.empty_title")}
        emptyDescription={t("catalog.queue.empty_description")}
        pageCount={pageCount}
        pageSizeOptions={[20, 50, 100]}
        state={listState.tableState}
        features={{
          persistKey: "platform-catalog-unresolved-v1",
          rowSelection: false,
          viewMode: false,
        }}
        toolbarExtra={
          <>
            <PermissionGuard permission={permissions.catalog.write}>
              <div className="flex items-center gap-2">
                <Switch
                  id="catalog-queue-save-rule"
                  checked={saveRule}
                  onCheckedChange={setSaveRule}
                />
                <Label htmlFor="catalog-queue-save-rule" className="text-sm">
                  {t("catalog.queue.save_rule")}
                </Label>
              </div>
            </PermissionGuard>
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <ModelCreateDialog
        open={Boolean(creatingFor)}
        defaultBrand={creatingFor?.brand.uuid}
        defaultName={creatingFor ? suggestName(creatingFor.raw_name) : ""}
        description={
          creatingFor
            ? t("catalog.queue.create_description", {
                name: creatingFor.raw_name,
              })
            : undefined
        }
        onOpenChange={(open) => {
          if (!open) setCreatingFor(null);
        }}
        onCreated={async (model: CatalogModel) => {
          if (!creatingFor) return;
          const rule =
            saveRule && startsWithName(creatingFor.raw_name, model.name);
          await patch.mutateAsync({
            uuid: creatingFor.uuid,
            body: {
              model_uuid: model.uuid,
              ...(rule ? { save_rule: true } : {}),
            },
          });
        }}
      />
    </EntityPage>
  );
}
