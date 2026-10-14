"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCircle2, FileSpreadsheet, Play } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatsCard } from "@/components/common/stats-card";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CatalogLocalTable } from "@/features/catalog/components/catalog-local-table";
import { useCatalogBreadcrumbs } from "@/features/catalog/components/catalog-tabs";
import {
  CATALOG_IMPORT_RUNNING,
  CATALOG_IMPORT_TONE,
  formatTRY,
} from "@/features/catalog/constants";
import {
  useApplyImport,
  useCatalogImport,
  useInvalidateCatalog,
} from "@/features/catalog/hooks/use-catalog";
import type {
  CatalogImport,
  CatalogImportCounts,
  CatalogImportDiff,
} from "@/features/catalog/services/catalog.service";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const COUNT_KEYS: (keyof CatalogImportCounts)[] = [
  "brands_new",
  "brands_renamed",
  "display_brands_new",
  "models_new",
  "types_new",
  "types_renamed",
  "types_reactivated",
  "types_updated",
  "types_deactivated",
  "valuations_new",
  "valuations_changed",
  "valuations_upserted",
  "unresolved_models",
];

type SampleRow = { key: string } & Record<string, unknown>;

type SampleColumn = {
  id: string;
  labelKey: string;
  cell?: (row: SampleRow) => ReactNode;
};

/** Cell renderer showing a TSB code in monospace. */
function code(id: string) {
  return function CodeCell(row: SampleRow) {
    return <code className="text-xs">{String(row[id] ?? "")}</code>;
  };
}

/** One diff sample list: a collapsible section with a client DataTable. */
function SampleTable({
  title,
  columns,
  rows,
}: {
  title: string;
  columns: SampleColumn[];
  rows: SampleRow[];
}) {
  const defs = useMemo(
    () =>
      columns.map((c, i) =>
        createColumn<SampleRow>({
          id: c.id,
          accessorFn: (row) => row[c.id] as string | number | undefined,
          labelKey: c.labelKey,
          enableSorting: true,
          gridPrimary: i === 0,
          gridSecondary: i === 1,
          cell: ({ row }) =>
            c.cell
              ? c.cell(row.original)
              : String(row.original[c.id] ?? "") || "—",
        }),
      ) as ColumnDef<SampleRow, unknown>[],
    [columns],
  );
  if (rows.length === 0) return null;
  return (
    <EntitySectionCard title={title} badge={rows.length} collapsible>
      <CatalogLocalTable
        columns={defs}
        data={rows}
        getRowId={(row) => row.key}
      />
    </EntitySectionCard>
  );
}

const TYPE_COLUMNS: SampleColumn[] = [
  { id: "brand", labelKey: "catalog.columns.brand" },
  {
    id: "code",
    labelKey: "catalog.columns.tsb_type_code",
    cell: code("code"),
  },
  { id: "name", labelKey: "catalog.columns.raw_name" },
];

function typeRows(list: CatalogImportDiff["new_types"]): SampleRow[] {
  return (list ?? []).map((s, i) => ({
    ...s,
    key: `${s.brand}-${s.code}-${i}`,
  }));
}

function DiffSamples({ diff }: { diff: CatalogImportDiff }) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () => ({
      brands: [
        {
          id: "code",
          labelKey: "catalog.columns.tsb_code",
          cell: code("code"),
        },
        { id: "name", labelKey: "catalog.columns.tsb_name" },
        { id: "display_name", labelKey: "catalog.columns.display_brand" },
        {
          id: "category",
          labelKey: "catalog.columns.category",
          cell: (row: SampleRow) =>
            row.category ? t(`catalog.category.${String(row.category)}`) : "—",
        },
      ] satisfies SampleColumn[],
      renamedBrands: [
        {
          id: "code",
          labelKey: "catalog.columns.tsb_code",
          cell: code("code"),
        },
        { id: "old_name", labelKey: "catalog.diff.old_name" },
        { id: "name", labelKey: "catalog.diff.new_name" },
      ] satisfies SampleColumn[],
      models: [
        { id: "brand", labelKey: "catalog.columns.brand" },
        { id: "name", labelKey: "catalog.columns.model" },
        {
          id: "types",
          labelKey: "catalog.columns.types_count",
          cell: (row: SampleRow) => (
            <span className="tabular-nums">{String(row.types ?? 0)}</span>
          ),
        },
      ] satisfies SampleColumn[],
      newTypes: [
        ...TYPE_COLUMNS,
        { id: "model", labelKey: "catalog.columns.model" },
      ],
      renamedTypes: [
        ...TYPE_COLUMNS,
        { id: "old_name", labelKey: "catalog.diff.old_name" },
      ],
      valuations: [
        ...TYPE_COLUMNS,
        {
          id: "year",
          labelKey: "catalog.columns.year",
          cell: (row: SampleRow) => (
            <span className="tabular-nums">{String(row.year ?? "")}</span>
          ),
        },
        {
          id: "old",
          labelKey: "catalog.diff.old_value",
          cell: (row: SampleRow) => (
            <span className="tabular-nums">
              {formatTRY(row.old as number, locale)}
            </span>
          ),
        },
        {
          id: "new",
          labelKey: "catalog.diff.new_value",
          cell: (row: SampleRow) => (
            <span className="tabular-nums">
              {formatTRY(row.new as number, locale)}
            </span>
          ),
        },
      ] satisfies SampleColumn[],
    }),
    [locale, t],
  );

  return (
    <div className="space-y-4">
      <SampleTable
        title={t("catalog.diff.new_brands")}
        columns={columns.brands}
        rows={(diff.new_brands ?? []).map((b, i) => ({
          ...b,
          key: `${b.code ?? ""}-${i}`,
        }))}
      />
      <SampleTable
        title={t("catalog.diff.renamed_brands")}
        columns={columns.renamedBrands}
        rows={(diff.renamed_brands ?? []).map((b, i) => ({
          ...b,
          key: `${b.code ?? ""}-${i}`,
        }))}
      />
      <SampleTable
        title={t("catalog.diff.new_models")}
        columns={columns.models}
        rows={(diff.new_models ?? []).map((m, i) => ({
          ...m,
          key: `${m.brand}-${m.name}-${i}`,
        }))}
      />
      <SampleTable
        title={t("catalog.diff.new_types")}
        columns={columns.newTypes}
        rows={typeRows(diff.new_types)}
      />
      <SampleTable
        title={t("catalog.diff.renamed_types")}
        columns={columns.renamedTypes}
        rows={typeRows(diff.renamed_types)}
      />
      <SampleTable
        title={t("catalog.diff.deactivated_types")}
        columns={TYPE_COLUMNS}
        rows={typeRows(diff.deactivated_types)}
      />
      <SampleTable
        title={t("catalog.diff.reactivated_types")}
        columns={TYPE_COLUMNS}
        rows={typeRows(diff.reactivated_types)}
      />
      <SampleTable
        title={t("catalog.diff.unresolved_types")}
        columns={TYPE_COLUMNS}
        rows={typeRows(diff.unresolved_types)}
      />
      <SampleTable
        title={t("catalog.diff.changed_valuations")}
        columns={columns.valuations}
        rows={(diff.changed_valuations ?? []).map((v, i) => ({
          ...v,
          key: `${v.brand}-${v.code}-${v.year}-${i}`,
        }))}
      />
    </div>
  );
}

function canApply(job: CatalogImport) {
  return (
    Boolean(job.period) &&
    (job.status === "ready" ||
      job.status === "applied" ||
      job.status === "failed")
  );
}

export function CatalogImportDetailPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const query = useCatalogImport(uuid);
  const apply = useApplyImport();
  const invalidateCatalog = useInvalidateCatalog();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const job = query.data;
  const breadcrumbs = useCatalogBreadcrumbs(
    job?.period ?? t("catalog.imports.detail_title"),
  );
  const running = job ? CATALOG_IMPORT_RUNNING.includes(job.status) : false;

  // When an apply finishes, refresh the brand / model / type lists.
  const prevStatus = useRef(job?.status);
  useEffect(() => {
    if (prevStatus.current === "applying" && job?.status === "applied") {
      invalidateCatalog();
    }
    prevStatus.current = job?.status;
  }, [invalidateCatalog, job?.status]);

  const counts = job?.counts ?? {};
  const diff = job?.diff;

  return (
    <EntityPage
      title={t("catalog.imports.detail_title")}
      description={t("catalog.imports.detail_description")}
      permission={permissions.catalog.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("catalog.forbidden")}
        />
      }
      breadcrumbs={[
        ...breadcrumbs.slice(0, 2),
        {
          label: t("catalog.tabs.imports"),
          href: routes.platform.catalog.imports,
        },
        breadcrumbs[2],
      ]}
      actions={
        job ? (
          <EntityActions>
            <PermissionGuard permission={permissions.catalog.write}>
              <Button
                size="sm"
                disabled={!canApply(job) || apply.isPending}
                onClick={() => setConfirmOpen(true)}
              >
                <Play className="size-4" />
                {job.status === "applied"
                  ? t("catalog.imports.reapply")
                  : t("catalog.imports.apply")}
              </Button>
            </PermissionGuard>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("catalog.imports.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {job ? (
        <div className="space-y-6">
          <EntityHeader
            title={job.filename || job.uuid}
            subtitle={job.period ?? undefined}
            badges={
              <StatusChip
                label={t(`catalog.status.${job.status}`)}
                tone={CATALOG_IMPORT_TONE[job.status]}
              />
            }
            leading={
              <div className="bg-muted flex size-12 items-center justify-center rounded-lg">
                <FileSpreadsheet className="text-muted-foreground size-6" />
              </div>
            }
          >
            <EntityDetail
              sections={[
                {
                  id: "summary",
                  fields: [
                    {
                      key: "period",
                      label: t("catalog.columns.period"),
                      value: diff?.period_label
                        ? `${job.period ?? "—"} (${diff.period_label})`
                        : (job.period ?? "—"),
                    },
                    {
                      key: "rows",
                      label: t("catalog.counts.rows"),
                      value: counts.rows ?? "—",
                    },
                    {
                      key: "created",
                      label: t("catalog.columns.created_at"),
                      value: datetime(job.created_at),
                    },
                    {
                      key: "applied",
                      label: t("catalog.columns.applied_at"),
                      value: job.applied_at ? datetime(job.applied_at) : "—",
                    },
                  ],
                },
              ]}
            />
          </EntityHeader>

          {running ? (
            <div className="space-y-2">
              <p className="text-muted-foreground text-sm">
                {t(`catalog.imports.running_${job.status}`)}
              </p>
              <Progress
                value={job.status === "applying" ? 66 : 33}
                className="animate-pulse"
              />
            </div>
          ) : null}

          {job.status === "failed" && job.error ? (
            <Alert variant="destructive">
              <AlertTitle>{t("catalog.imports.failed_title")}</AlertTitle>
              <AlertDescription>{job.error}</AlertDescription>
            </Alert>
          ) : null}

          {job.status === "ready" ? (
            <Alert>
              <AlertTitle>{t("catalog.imports.ready_title")}</AlertTitle>
              <AlertDescription>
                {t("catalog.imports.ready_description")}
              </AlertDescription>
            </Alert>
          ) : null}

          {job.status === "applied" ? (
            <Alert>
              <CheckCircle2 className="size-4" />
              <AlertTitle>{t("catalog.imports.applied_title")}</AlertTitle>
              <AlertDescription>
                {t("catalog.imports.applied_description")}
              </AlertDescription>
            </Alert>
          ) : null}

          {job.status !== "queued" && job.status !== "processing" ? (
            <>
              <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
                {COUNT_KEYS.map((key) => (
                  <StatsCard
                    key={key}
                    title={t(`catalog.counts.${key}`)}
                    value={
                      <span className="tabular-nums">
                        {typeof counts[key] === "number"
                          ? (counts[key] as number)
                          : 0}
                      </span>
                    }
                  />
                ))}
              </div>
              {counts.unresolved_by_category &&
              Object.keys(counts.unresolved_by_category).length > 0 ? (
                <p className="text-muted-foreground text-xs">
                  {t("catalog.imports.unresolved_breakdown")}{" "}
                  {Object.entries(counts.unresolved_by_category)
                    .map(([cat, n]) => `${t(`catalog.category.${cat}`)}: ${n}`)
                    .join(" · ")}
                </p>
              ) : null}
            </>
          ) : null}

          {diff ? <DiffSamples diff={diff} /> : null}
        </div>
      ) : null}

      <AppDialog
        open={confirmOpen}
        onOpenChange={(next) => {
          if (!apply.isPending) setConfirmOpen(next);
        }}
        title={t("catalog.imports.apply_confirm_title")}
        description={t("catalog.imports.apply_confirm_description", {
          period: job?.period ?? "—",
        })}
        footer={
          <>
            <Button
              variant="outline"
              disabled={apply.isPending}
              onClick={() => setConfirmOpen(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button
              disabled={apply.isPending}
              onClick={async () => {
                await apply.mutateAsync(uuid);
                setConfirmOpen(false);
              }}
            >
              {t("catalog.imports.apply")}
            </Button>
          </>
        }
      />
    </EntityPage>
  );
}
