"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ArrowRight } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState, type ReactNode } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  INTEGRATION_JOB_KIND_LABEL_KEYS,
  INTEGRATION_JOB_STATUS_LABEL_KEYS,
  INTEGRATION_PROVIDER_LABEL_KEYS,
} from "@/features/payments/constants";
import type { IntegrationJobStatus } from "@/features/payments/types";
import { moneyText } from "@/features/pricing/lib/format";
import { OperationTaskStatusChip } from "@/features/rentals/components/rental-cells";
import {
  TASK_STATUS_LABEL_KEYS as OPERATION_STATUS_LABEL_KEYS,
  TASK_TYPE_LABEL_KEYS as OPERATION_TYPE_LABEL_KEYS,
} from "@/features/rentals/constants";
import type {
  OperationTaskStatus,
  OperationTaskType,
} from "@/features/rentals/types";
import {
  ReferenceText,
  ReservationStatusChip,
} from "@/features/reservations/components/reservation-cells";
import { TASK_TYPE_LABEL_KEYS as RESOLUTION_TYPE_LABEL_KEYS } from "@/features/reservations/constants";
import type {
  ReservationStatus,
  TaskType,
} from "@/features/reservations/types";
import { overviewLinks } from "@/features/overview/lib/links";
import type {
  OverviewOperationItem,
  TenantOverview,
} from "@/features/overview/types";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Minute clock for the deadline countdowns. */
export function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs]);
  return now;
}

/** "2 sa 15 dk kaldı" / "40 dk gecikti" for a deadline. */
function Countdown({
  deadline,
  now,
}: {
  deadline: string | null | undefined;
  now: number;
}) {
  const { t } = useLocale();
  if (!deadline) return <span className="text-muted-foreground">—</span>;
  const left = Math.round((new Date(deadline).getTime() - now) / 60_000);
  if (left < 0) {
    return (
      <Badge variant="danger" className="tabular-nums">
        {durationText(t, -left, "overview.time.late")}
      </Badge>
    );
  }
  return (
    <Badge
      variant={left < 60 ? "warning" : "secondary"}
      className="tabular-nums"
    >
      {durationText(t, left, "overview.time.left")}
    </Badge>
  );
}

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

function durationText(t: Translate, minutes: number, prefix: string) {
  const days = Math.floor(minutes / 1440);
  const hours = Math.floor((minutes % 1440) / 60);
  const mins = minutes % 60;
  if (days > 0) return t(`${prefix}_days`, { days, hours });
  if (hours > 0) return t(`${prefix}_hours`, { hours, minutes: mins });
  return t(`${prefix}_minutes`, { minutes: mins });
}

/** Compact attention card: capped rows + "Tümünü gör". */
function AttentionCard({
  title,
  total,
  href,
  emptyText,
  tone = "secondary",
  children,
  footer,
}: {
  title: string;
  total: number;
  href: string;
  emptyText: string;
  tone?: "secondary" | "warning" | "danger";
  children: ReactNode;
  footer?: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <EntitySectionCard
      title={title}
      className="flex min-w-0 flex-col"
      contentClassName="flex flex-1 flex-col gap-3 pt-3"
      actions={
        <Badge
          variant={total > 0 ? tone : "secondary"}
          className="tabular-nums"
        >
          {total}
        </Badge>
      }
    >
      {total === 0 ? (
        <p className="text-muted-foreground rounded-md border border-dashed px-3 py-6 text-center text-sm">
          {emptyText}
        </p>
      ) : (
        <ul className="divide-border -mx-1 divide-y">{children}</ul>
      )}
      {footer}
      <div className="mt-auto flex justify-end">
        <Button asChild variant="ghost" size="sm">
          <Link href={href}>
            {total > 0
              ? t("overview.see_all_count", { count: total })
              : t("overview.see_all")}
            <ArrowRight className="size-3.5" />
          </Link>
        </Button>
      </div>
    </EntitySectionCard>
  );
}

function Row({
  href,
  title,
  meta,
  aside,
}: {
  href: string;
  title: ReactNode;
  meta: ReactNode;
  aside?: ReactNode;
}) {
  return (
    <li>
      <Link
        href={href}
        className="hover:bg-muted/60 focus-visible:ring-ring flex items-start justify-between gap-3 rounded-md px-1 py-2.5 outline-none focus-visible:ring-2"
      >
        <span className="min-w-0 flex-1 space-y-1">
          <span className="flex flex-wrap items-center gap-1.5 text-sm font-medium">
            {title}
          </span>
          <span className="text-muted-foreground line-clamp-2 block text-xs break-words">
            {meta}
          </span>
        </span>
        {aside ? <span className="shrink-0 text-right">{aside}</span> : null}
      </Link>
    </li>
  );
}

function joinMeta(parts: (string | null | undefined)[]) {
  return parts.filter(Boolean).join(" · ");
}

type SectionProps = {
  data: TenantOverview;
  slug: string;
  branch: string | undefined;
  now: number;
};

/** Today's handovers and returns (DataTable; up to 100 rows). */
export function TodayOperationsSection({ data, slug, branch }: SectionProps) {
  const { t, locale } = useLocale();
  const router = useRouter();
  const today = data.today_operations;
  const columns = useMemo(
    () =>
      [
        createColumn<OverviewOperationItem>({
          accessorKey: "due_at",
          labelKey: "overview.today.columns.time",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {datetime(row.original.due_at, "HH:mm", locale)}
            </span>
          ),
        }),
        createColumn<OverviewOperationItem>({
          accessorKey: "task_type",
          labelKey: "overview.today.columns.type",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (["handover", "return"] as const).map((v) => ({
            value: v,
            labelKey: OPERATION_TYPE_LABEL_KEYS[v],
            label: t(OPERATION_TYPE_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <Badge
              variant={
                row.original.task_type === "handover" ? "default" : "outline"
              }
            >
              {t(
                OPERATION_TYPE_LABEL_KEYS[
                  row.original.task_type as OperationTaskType
                ],
              )}
            </Badge>
          ),
        }),
        createColumn<OverviewOperationItem>({
          id: "plate",
          accessorFn: (row) => row.plate,
          labelKey: "overview.today.columns.plate",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs font-medium">
              {row.original.plate || "—"}
            </span>
          ),
        }),
        createColumn<OverviewOperationItem>({
          id: "customer",
          accessorFn: (row) => row.customer_name,
          labelKey: "overview.today.columns.customer",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-col">
              <span className="font-medium">{row.original.customer_name}</span>
              <span className="text-muted-foreground text-xs">
                {row.original.rental.reference}
              </span>
            </span>
          ),
        }),
        createColumn<OverviewOperationItem>({
          id: "branch",
          accessorFn: (row) => row.branch.code,
          labelKey: "overview.today.columns.branch",
          enableSorting: false,
          cell: ({ row }) => row.original.branch.code,
        }),
        createColumn<OverviewOperationItem>({
          accessorKey: "status",
          labelKey: "overview.today.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (
            [
              "planned",
              "assigned",
              "en_route",
              "issue_reported",
              "completed",
            ] as const
          ).map((v) => ({
            value: v,
            labelKey: OPERATION_STATUS_LABEL_KEYS[v],
            label: t(OPERATION_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <OperationTaskStatusChip
              status={row.original.status as OperationTaskStatus}
            />
          ),
        }),
      ] as ColumnDef<OverviewOperationItem, unknown>[],
    [locale, t],
  );
  if (!today) return null;
  const counts = (label: string, c: typeof today.handovers) => (
    <span className="text-muted-foreground text-sm">
      {label}:{" "}
      <span className="text-foreground font-medium tabular-nums">
        {c.done}/{c.total}
      </span>
    </span>
  );
  return (
    <EntitySectionCard
      title={t("overview.today.title")}
      className="min-w-0"
      badge={today.total}
      actions={
        <Button asChild variant="ghost" size="sm">
          <Link href={overviewLinks.todayOperations(slug, branch, data.today)}>
            {t("overview.see_all")}
            <ArrowRight className="size-3.5" />
          </Link>
        </Button>
      }
    >
      <div className="mb-3 flex flex-wrap gap-x-5 gap-y-1">
        {counts(t("overview.today.handovers"), today.handovers)}
        {counts(t("overview.today.returns"), today.returns)}
      </div>
      <EntityTable
        columns={columns}
        data={today.items}
        getRowId={(row) => row.uuid}
        emptyTitle={t("overview.today.empty")}
        emptyDescription={t("overview.today.empty_description")}
        onRowClick={(row) =>
          router.push(routes.tenant.rentals.detail(slug, row.rental.uuid))
        }
        features={{
          persistKey: "tenant-overview-today-v1",
          rowSelection: false,
          globalFilter: true,
          columnOrdering: false,
          columnPinning: false,
          columnResizing: false,
          viewMode: false,
        }}
      />
    </EntitySectionCard>
  );
}

export function OverdueReturnsCard({ data, slug, branch, now }: SectionProps) {
  const { t, locale } = useLocale();
  const section = data.overdue_returns;
  if (!section) return null;
  return (
    <AttentionCard
      title={t("overview.overdue.title")}
      total={section.total}
      tone="danger"
      href={overviewLinks.overdueReturns(slug, branch)}
      emptyText={t("overview.overdue.empty")}
    >
      {section.items.map((it) => (
        <Row
          key={it.uuid}
          href={routes.tenant.rentals.detail(slug, it.uuid)}
          title={
            <>
              <span className="font-mono text-xs">{it.plate}</span>
              <ReferenceText reference={it.reference} />
            </>
          }
          meta={joinMeta([
            it.customer_name,
            it.return_branch.code,
            t("overview.overdue.planned", {
              at: datetime(it.planned_return_at, "d MMM HH:mm", locale),
            }),
          ])}
          aside={<Countdown deadline={it.planned_return_at} now={now} />}
        />
      ))}
    </AttentionCard>
  );
}

const WAITING_KEYS = {
  approvals: {
    title: "overview.approvals.title",
    empty: "overview.approvals.empty",
    overdue: "overview.approvals.overdue",
  },
  awaiting_payment: {
    title: "overview.awaiting_payment.title",
    empty: "overview.awaiting_payment.empty",
    overdue: "overview.awaiting_payment.overdue",
  },
} as const;

function WaitingCard({
  kind,
  data,
  slug,
  branch,
  now,
}: SectionProps & { kind: "approvals" | "awaiting_payment" }) {
  const { t, locale } = useLocale();
  const section = kind === "approvals" ? data.approvals : data.awaiting_payment;
  if (!section) return null;
  const href =
    kind === "approvals"
      ? overviewLinks.approvals(slug)
      : overviewLinks.awaitingPayment(slug, branch);
  return (
    <AttentionCard
      title={t(WAITING_KEYS[kind].title)}
      total={section.total}
      tone={section.overdue > 0 ? "danger" : "warning"}
      href={href}
      emptyText={t(WAITING_KEYS[kind].empty)}
      footer={
        section.overdue > 0 ? (
          <p className="text-destructive text-xs font-medium">
            {t(WAITING_KEYS[kind].overdue, { count: section.overdue })}
          </p>
        ) : null
      }
    >
      {section.items.map((it) => (
        <Row
          key={it.uuid}
          href={routes.tenant.reservations.detail(slug, it.uuid)}
          title={
            <>
              <ReferenceText reference={it.reference} />
              <ReservationStatusChip status={it.status as ReservationStatus} />
            </>
          }
          meta={joinMeta([
            it.customer_name,
            it.pickup_branch.code,
            datetime(it.starts_at, "d MMM HH:mm", locale),
            kind === "awaiting_payment"
              ? t("overview.awaiting_payment.due", {
                  amount: moneyText(it.amount_due, locale),
                })
              : moneyText(it.total_amount, locale),
          ])}
          aside={<Countdown deadline={it.deadline} now={now} />}
        />
      ))}
    </AttentionCard>
  );
}

export function ApprovalsCard(props: SectionProps) {
  return <WaitingCard kind="approvals" {...props} />;
}

export function AwaitingPaymentCard(props: SectionProps) {
  return <WaitingCard kind="awaiting_payment" {...props} />;
}

export function AvailabilityProblemsCard({ data, slug }: SectionProps) {
  const { t, locale } = useLocale();
  const section = data.availability_problems;
  if (!section) return null;
  const byType = Object.entries(section.by_type).filter(([, n]) => n > 0);
  return (
    <AttentionCard
      title={t("overview.availability.title")}
      total={section.total}
      tone="danger"
      href={overviewLinks.resolutionTasks(slug)}
      emptyText={t("overview.availability.empty")}
      footer={
        byType.length ? (
          <div className="flex flex-wrap gap-1.5">
            {byType.map(([type, n]) => (
              <Badge key={type} variant="outline" className="tabular-nums">
                {t(RESOLUTION_TYPE_LABEL_KEYS[type as TaskType] ?? type)}: {n}
              </Badge>
            ))}
          </div>
        ) : null
      }
    >
      {section.items.map((it) => (
        <Row
          key={it.uuid}
          href={routes.tenant.reservations.detail(slug, it.reservation.uuid)}
          title={
            <>
              <StatusChip
                label={t(
                  RESOLUTION_TYPE_LABEL_KEYS[it.task_type as TaskType] ??
                    it.task_type,
                )}
                tone="danger"
              />
              <ReferenceText reference={it.reservation.reference} />
            </>
          }
          meta={joinMeta([
            it.reason,
            it.pickup_branch.code,
            datetime(it.created_at, "d MMM HH:mm", locale),
          ])}
        />
      ))}
    </AttentionCard>
  );
}

export function IntegrationJobsCard({ data, slug }: SectionProps) {
  const { t, locale } = useLocale();
  const section = data.integration_jobs;
  if (!section) return null;
  const href = overviewLinks.integrationJobs(slug);
  return (
    <AttentionCard
      title={t("overview.jobs.title")}
      total={section.total}
      tone="danger"
      href={href}
      emptyText={t("overview.jobs.empty")}
      footer={
        <p className="text-muted-foreground text-xs">
          {t("overview.jobs.scope_note")}
        </p>
      }
    >
      {section.items.map((it) => (
        <Row
          key={it.uuid}
          href={
            it.kind.startsWith("kabis.")
              ? routes.tenant.kabis.root(slug)
              : it.kind.startsWith("einvoice.")
                ? `${routes.tenant.invoices.root(slug)}?needs_action=true`
                : href
          }
          title={
            <>
              <span>
                {INTEGRATION_JOB_KIND_LABEL_KEYS[it.kind]
                  ? t(INTEGRATION_JOB_KIND_LABEL_KEYS[it.kind])
                  : it.kind}
              </span>
              <StatusChip
                label={t(
                  INTEGRATION_JOB_STATUS_LABEL_KEYS[
                    it.status as IntegrationJobStatus
                  ],
                )}
                tone={it.status === "failed" ? "danger" : "warning"}
              />
            </>
          }
          meta={joinMeta([
            it.account?.name ??
              (INTEGRATION_PROVIDER_LABEL_KEYS[it.provider]
                ? t(INTEGRATION_PROVIDER_LABEL_KEYS[it.provider])
                : it.provider),
            t("overview.jobs.attempts", { count: it.attempts }),
            it.last_error || null,
            datetime(it.updated_at, "d MMM HH:mm", locale),
          ])}
        />
      ))}
    </AttentionCard>
  );
}

export function DocumentsCard({ data, slug, branch, now }: SectionProps) {
  const { t, locale } = useLocale();
  const section = data.documents;
  if (!section) return null;
  return (
    <AttentionCard
      title={t("overview.documents.title")}
      total={section.total}
      tone="warning"
      href={overviewLinks.documents(slug, branch)}
      emptyText={
        section.required
          ? t("overview.documents.empty", { hours: section.window_hours })
          : t("overview.documents.not_required")
      }
    >
      {section.items.map((it) => (
        <Row
          key={it.reservation.uuid}
          href={
            it.rental
              ? routes.tenant.rentals.detail(slug, it.rental.uuid)
              : routes.tenant.reservations.detail(slug, it.reservation.uuid)
          }
          title={
            <>
              <ReferenceText reference={it.reservation.reference} />
              <Badge
                variant={it.document ? "warning" : "outline"}
                className={cn(!it.document && "text-muted-foreground")}
              >
                {it.document
                  ? t("overview.documents.awaiting_approval")
                  : t("overview.documents.not_prepared")}
              </Badge>
            </>
          }
          meta={joinMeta([
            it.customer_name,
            it.pickup_branch.code,
            t("overview.documents.handover_at", {
              at: datetime(it.reservation.starts_at, "d MMM HH:mm", locale),
            }),
          ])}
          aside={<Countdown deadline={it.reservation.starts_at} now={now} />}
        />
      ))}
    </AttentionCard>
  );
}
