"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCheck, Eye, RotateCcw } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDrawer,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { PermissionGuard } from "@/components/common/permission-guard";
import { ReconcileJobDialog } from "@/features/payments/components/reconcile-job-dialog";
import {
  ATTEMPT_OPERATION_LABEL_KEYS,
  ATTEMPT_OUTCOME_LABEL_KEYS,
  ATTEMPT_OUTCOME_TONES,
  ATTEMPT_TRIGGER_LABEL_KEYS,
  INTEGRATION_JOBS_RESOURCE,
  INTEGRATION_JOB_KIND_LABEL_KEYS,
  INTEGRATION_JOB_STATUSES,
  INTEGRATION_JOB_STATUS_LABEL_KEYS,
  INTEGRATION_JOB_STATUS_TONES,
} from "@/features/payments/constants";
import {
  useIntegrationJob,
  useIntegrationJobs,
  useRetryIntegrationJob,
} from "@/features/payments/hooks/use-payments";
import type {
  IntegrationAccount,
  IntegrationAttempt,
  IntegrationJob,
  IntegrationJobDetail,
} from "@/features/payments/types";
import { reasonText } from "@/features/reservations/constants";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const DATE = "d MMM yyyy HH:mm:ss";

function filterValues(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  return [];
}

/**
 * Jobs of one integration account (TEC-751): every outbound call and
 * inbound notification with its state; live over the
 * `tenant.integration_jobs` channel. A row opens the job drawer.
 */
export function IntegrationJobsSection({
  account,
  onOpenJob,
}: {
  account: IntegrationAccount;
  onOpenJob: (uuid: string) => void;
}) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 10,
    realtime: tenantLiveList(organization, INTEGRATION_JOBS_RESOURCE),
  });
  const status = filterValues(
    listState.columnFilters.find((f) => f.id === "status")?.value,
  );
  const params = {
    limit: listState.params.limit,
    offset: listState.params.offset,
    sort: listState.params.sort,
    status: status.length ? status.join(",") : undefined,
  };
  const query = useIntegrationJobs(account.uuid, params);
  const total = query.data?.total ?? 0;
  const [reconcileTarget, setReconcileTarget] = useState<string | null>(null);
  const manage = permissions.tenant.integrations.manage;

  const columns = useMemo(
    () =>
      [
        createColumn<IntegrationJob>({
          accessorKey: "kind",
          labelKey: "payments.integrations.jobs.columns.kind",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span className="font-medium">
                {reasonText(
                  t,
                  INTEGRATION_JOB_KIND_LABEL_KEYS,
                  row.original.kind,
                )}
              </span>
              <span className="text-muted-foreground font-mono text-xs">
                {row.original.uuid.slice(0, 8)}
              </span>
            </div>
          ),
        }),
        createColumn<IntegrationJob>({
          accessorKey: "status",
          labelKey: "payments.integrations.jobs.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: INTEGRATION_JOB_STATUSES.map((s) => ({
            value: s,
            label: t(INTEGRATION_JOB_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(INTEGRATION_JOB_STATUS_LABEL_KEYS[row.original.status])}
              tone={INTEGRATION_JOB_STATUS_TONES[row.original.status]}
            />
          ),
        }),
        createColumn<IntegrationJob>({
          accessorKey: "attempts",
          labelKey: "payments.integrations.jobs.columns.attempts",
          enableSorting: false,
          cell: ({ row }) =>
            `${row.original.attempts} / ${row.original.max_attempts}`,
        }),
        createColumn<IntegrationJob>({
          accessorKey: "last_error",
          labelKey: "payments.integrations.jobs.columns.last_error",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.last_error ? (
              <span className="text-destructive line-clamp-2 text-xs">
                {row.original.last_error}
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<IntegrationJob>({
          accessorKey: "next_attempt_at",
          labelKey: "payments.integrations.jobs.columns.next_attempt_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.next_attempt_at
              ? datetime(row.original.next_attempt_at, DATE, locale)
              : "—",
        }),
        createColumn<IntegrationJob>({
          accessorKey: "created_at",
          labelKey: "payments.integrations.jobs.columns.created_at",
          enableSorting: true,
          cell: ({ row }) => datetime(row.original.created_at, DATE, locale),
        }),
        createColumn<IntegrationJob>({
          accessorKey: "updated_at",
          labelKey: "payments.integrations.jobs.columns.updated_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => datetime(row.original.updated_at, DATE, locale),
        }),
        createColumn<IntegrationJob>({
          id: "actions",
          labelKey: "payments.integrations.jobs.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "open",
                  label: t("payments.integrations.jobs.actions.open"),
                  icon: Eye,
                  onSelect: () => onOpenJob(row.original.uuid),
                },
                ...(row.original.reconcilable
                  ? [
                      {
                        id: "reconcile",
                        label: t(
                          "payments.integrations.jobs.actions.reconcile",
                        ),
                        icon: CheckCheck,
                        permission: manage,
                        separatorBefore: true,
                        onSelect: () => setReconcileTarget(row.original.uuid),
                      },
                    ]
                  : []),
              ]}
            />
          ),
        }),
      ] as ColumnDef<IntegrationJob, unknown>[],
    [locale, manage, onOpenJob, t],
  );

  return (
    <EntitySectionCard
      title={t("payments.integrations.jobs.title", {
        account: account.display_name,
      })}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("payments.integrations.jobs.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("payments.integrations.jobs.empty_title")}
        emptyDescription={t("payments.integrations.jobs.empty_description")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 10)),
        )}
        state={listState.tableState}
        onRowClick={(row) => onOpenJob(row.uuid)}
        features={{
          persistKey: "tenant-integration-jobs-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
      <ReconcileJobDialog
        jobUuid={reconcileTarget}
        onClose={() => setReconcileTarget(null)}
      />
    </EntitySectionCard>
  );
}

export function SummaryList({ value }: { value: Record<string, unknown> }) {
  const entries = Object.entries(value);
  if (!entries.length) return <span className="text-muted-foreground">—</span>;
  return (
    <div className="flex flex-col gap-0.5 font-mono text-xs">
      {entries.map(([k, v]) => (
        <span key={k} className="break-all">
          <span className="text-muted-foreground">{k}:</span>{" "}
          {typeof v === "object" ? JSON.stringify(v) : String(v)}
        </span>
      ))}
    </div>
  );
}

function Fact({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-muted-foreground text-xs">{label}</span>
      <span className="text-sm">{value}</span>
    </div>
  );
}

/**
 * Job drawer: state, references and every provider call (attempts). "Yeniden
 * dene" asks the provider first and resends only work it confirms was not
 * done.
 */
export function IntegrationJobDrawer({
  jobUuid,
  onClose,
}: {
  jobUuid: string | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const query = useIntegrationJob(jobUuid);
  const retry = useRetryIntegrationJob();
  const job = query.data;
  const [reconcileTarget, setReconcileTarget] = useState<string | null>(null);
  return (
    <EntityDrawer
      open={Boolean(jobUuid)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      size="xl"
      title={t("payments.integrations.jobs.detail_title")}
      description={t("payments.integrations.jobs.detail_description")}
      footer={
        job?.retryable || job?.reconcilable ? (
          <PermissionGuard permission={permissions.tenant.integrations.manage}>
            {job.reconcilable ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => setReconcileTarget(job.uuid)}
              >
                <CheckCheck className="size-4" />
                {t("payments.integrations.jobs.actions.reconcile")}
              </Button>
            ) : null}
            {job.retryable ? (
              <Button
                type="button"
                disabled={retry.isPending}
                onClick={() => retry.mutate(job.uuid)}
              >
                <RotateCcw className="size-4" />
                {t("payments.integrations.jobs.actions.retry")}
              </Button>
            ) : null}
          </PermissionGuard>
        ) : null
      }
    >
      {query.isLoading ? (
        <Loading />
      ) : query.isError || !job ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("payments.integrations.jobs.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <JobDetailBody job={job} />
      )}
      <ReconcileJobDialog
        jobUuid={reconcileTarget}
        onClose={() => setReconcileTarget(null)}
      />
    </EntityDrawer>
  );
}

/**
 * Every provider call of a job (submit / status check, trigger, outcome,
 * short request / response summaries, time and duration). Shared by the
 * integration job drawer and the KABİS notification detail.
 */
export function IntegrationAttemptsTable({
  attempts,
  persistKey,
}: {
  attempts: IntegrationAttempt[];
  persistKey: string;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<IntegrationAttempt>({
          accessorKey: "attempt_no",
          labelKey: "payments.integrations.jobs.attempts.columns.no",
          enableSorting: false,
          cell: ({ row }) => `#${row.original.attempt_no}`,
        }),
        createColumn<IntegrationAttempt>({
          accessorKey: "operation",
          labelKey: "payments.integrations.jobs.attempts.columns.operation",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span>
                {reasonText(
                  t,
                  ATTEMPT_OPERATION_LABEL_KEYS,
                  row.original.operation,
                )}
              </span>
              <span className="text-muted-foreground text-xs">
                {reasonText(
                  t,
                  ATTEMPT_TRIGGER_LABEL_KEYS,
                  row.original.trigger,
                )}
              </span>
            </div>
          ),
        }),
        createColumn<IntegrationAttempt>({
          accessorKey: "outcome",
          labelKey: "payments.integrations.jobs.attempts.columns.outcome",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col gap-1">
              <StatusChip
                label={reasonText(
                  t,
                  ATTEMPT_OUTCOME_LABEL_KEYS,
                  row.original.outcome,
                )}
                tone={ATTEMPT_OUTCOME_TONES[row.original.outcome] ?? "default"}
              />
              {row.original.error ? (
                <span className="text-destructive text-xs break-words">
                  {row.original.error}
                </span>
              ) : null}
            </div>
          ),
        }),
        createColumn<IntegrationAttempt>({
          id: "summary",
          labelKey: "payments.integrations.jobs.attempts.columns.summary",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col gap-1">
              <SummaryList value={row.original.request_summary} />
              <SummaryList value={row.original.response_summary} />
            </div>
          ),
        }),
        createColumn<IntegrationAttempt>({
          accessorKey: "started_at",
          labelKey: "payments.integrations.jobs.attempts.columns.started_at",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <span>{datetime(row.original.started_at, DATE, locale)}</span>
              <span className="text-muted-foreground text-xs">
                {t("payments.integrations.jobs.attempts.duration", {
                  ms: row.original.duration_ms,
                })}
              </span>
            </div>
          ),
        }),
      ] as ColumnDef<IntegrationAttempt, unknown>[],
    [locale, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={attempts}
      getRowId={(row) => row.uuid}
      emptyTitle={t("payments.integrations.jobs.attempts.empty")}
      manual={{ filtering: false, sorting: false, pagination: false }}
      features={{
        persistKey,
        rowSelection: false,
        columnFilters: false,
        facetedFilters: false,
        sorting: false,
        pagination: false,
      }}
    />
  );
}

function JobDetailBody({ job }: { job: IntegrationJobDetail }) {
  const { t, locale } = useLocale();
  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Fact
          label={t("payments.integrations.jobs.columns.kind")}
          value={reasonText(t, INTEGRATION_JOB_KIND_LABEL_KEYS, job.kind)}
        />
        <Fact
          label={t("payments.integrations.jobs.columns.status")}
          value={
            <StatusChip
              label={t(INTEGRATION_JOB_STATUS_LABEL_KEYS[job.status])}
              tone={INTEGRATION_JOB_STATUS_TONES[job.status]}
            />
          }
        />
        <Fact
          label={t("payments.integrations.jobs.columns.attempts")}
          value={`${job.attempts} / ${job.max_attempts}`}
        />
        <Fact
          label={t("payments.integrations.jobs.fields.priority")}
          value={t(
            job.priority === "critical"
              ? "payments.integrations.jobs.priorities.critical"
              : "payments.integrations.jobs.priorities.normal",
          )}
        />
        <Fact
          label={t("payments.integrations.jobs.fields.provider_ref")}
          value={
            <span className="font-mono text-xs break-all">
              {job.provider_ref || "—"}
            </span>
          }
        />
        <Fact
          label={t("payments.integrations.jobs.columns.next_attempt_at")}
          value={
            job.next_attempt_at
              ? datetime(job.next_attempt_at, DATE, locale)
              : "—"
          }
        />
        <Fact
          label={t("payments.integrations.jobs.columns.created_at")}
          value={datetime(job.created_at, DATE, locale)}
        />
        <Fact
          label={t("payments.integrations.jobs.fields.completed_at")}
          value={
            job.completed_at ? datetime(job.completed_at, DATE, locale) : "—"
          }
        />
      </div>
      {job.status === "needs_verification" ? (
        <p className="bg-muted rounded-md p-3 text-sm">
          {t("payments.integrations.jobs.needs_verification_note")}
        </p>
      ) : null}
      {job.reconciliation ? (
        <div className="bg-muted grid grid-cols-1 gap-4 rounded-md p-3 sm:grid-cols-2">
          <Fact
            label={t("payments.integrations.jobs.reconciliation.at")}
            value={datetime(job.reconciliation.at, DATE, locale)}
          />
          <Fact
            label={t("payments.integrations.jobs.reconciliation.by")}
            value={
              job.reconciliation.by
                ? `${job.reconciliation.by.name} ${job.reconciliation.by.surname}`.trim() ||
                  job.reconciliation.by.email
                : t("payments.integrations.jobs.reconciliation.operator")
            }
          />
          <Fact
            label={t("payments.integrations.jobs.reconciliation.note")}
            value={
              <span className="break-words">{job.reconciliation.note}</span>
            }
          />
          <Fact
            label={t("payments.integrations.jobs.reconciliation.external_ref")}
            value={
              <span className="font-mono text-xs break-all">
                {job.reconciliation.external_ref || "—"}
              </span>
            }
          />
        </div>
      ) : null}
      {job.last_error ? (
        <Fact
          label={t("payments.integrations.jobs.columns.last_error")}
          value={
            <span className="text-destructive break-words">
              {job.last_error}
            </span>
          }
        />
      ) : null}
      <Fact
        label={t("payments.integrations.jobs.fields.payload")}
        value={<SummaryList value={job.payload} />}
      />
      <IntegrationAttemptsTable
        attempts={job.attempt_log}
        persistKey="tenant-integration-attempts-v1"
      />
    </div>
  );
}
