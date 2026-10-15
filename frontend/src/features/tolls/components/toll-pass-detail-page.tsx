"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Ban, Link2, RotateCcw, Unlink } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { Timeline } from "@/components/common/timeline";
import {
  EntityDetail,
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
import { actorName } from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  ReviewReasonBadge,
  TollStatusChip,
  reviewReasonText,
  stationText,
  tollMoney,
} from "@/features/tolls/components/toll-cells";
import {
  AssignTollDialog,
  type ReasonAction,
  TollReasonDialog,
} from "@/features/tolls/components/toll-decision-dialogs";
import {
  CHANGED_FIELD_LABEL_KEYS,
  EVENT_LABEL_KEYS,
  MATCH_METHOD_LABEL_KEYS,
  SOURCE_LABEL_KEYS,
  TOLL_CHARGE_KIND_LABEL_KEYS,
  TOLL_STATUS_LABEL_KEYS,
} from "@/features/tolls/constants";
import { useTollPass } from "@/features/tolls/hooks/use-tolls";
import type {
  TollPassCharge,
  TollPassDetail,
  TollPassRevision,
  TollStatus,
} from "@/features/tolls/types";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function ChargesCard({ pass }: { pass: TollPassDetail }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const columns = useMemo(
    () =>
      [
        createColumn<TollPassCharge>({
          accessorKey: "kind",
          labelKey: "tolls.charges.columns.kind",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1.5">
              <span className="font-medium">
                {t(
                  TOLL_CHARGE_KIND_LABEL_KEYS[row.original.kind] ??
                    row.original.kind,
                )}
              </span>
              {row.original.reversal_of_uuid ? (
                <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
                  {t("tolls.charges.reversal")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<TollPassCharge>({
          id: "rental",
          accessorFn: (row) => row.rental.reference,
          labelKey: "tolls.charges.columns.rental",
          enableSorting: false,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.rentals.detail(
                slug,
                row.original.rental.uuid,
              )}
              className="font-mono text-xs font-medium underline-offset-4 hover:underline"
            >
              {row.original.rental.reference}
            </Link>
          ),
        }),
        createColumn<TollPassCharge>({
          accessorKey: "amount",
          labelKey: "tolls.charges.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span
              className={
                row.original.amount.startsWith("-")
                  ? "text-muted-foreground tabular-nums"
                  : "font-medium tabular-nums"
              }
            >
              {tollMoney(row.original.amount, row.original.currency, locale)}
            </span>
          ),
        }),
        createColumn<TollPassCharge>({
          accessorKey: "reason",
          labelKey: "tolls.charges.columns.reason",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="text-sm">{row.original.reason || "—"}</span>
          ),
        }),
        createColumn<TollPassCharge>({
          accessorKey: "created_at",
          labelKey: "tolls.charges.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
      ] as ColumnDef<TollPassCharge, unknown>[],
    [locale, slug, t],
  );
  return (
    <EntitySectionCard
      title={t("tolls.charges.title")}
      badge={pass.charges.length}
    >
      <EntityTable
        columns={columns}
        data={pass.charges}
        getRowId={(row) => row.uuid}
        emptyTitle={t("tolls.charges.empty")}
        emptyDescription={t("tolls.charges.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-toll-pass-charges-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
    </EntitySectionCard>
  );
}

function RevisionsCard({ pass }: { pass: TollPassDetail }) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<TollPassRevision>({
          accessorKey: "revision",
          labelKey: "tolls.revisions.columns.revision",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => `#${row.original.revision}`,
        }),
        createColumn<TollPassRevision>({
          id: "changed",
          accessorFn: (row) => row.changed_fields.join(","),
          labelKey: "tolls.revisions.columns.changed",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="flex flex-wrap gap-1">
              {row.original.changed_fields.map((f) => (
                <Badge
                  key={f}
                  variant="outline"
                  className="text-xs font-normal"
                >
                  {CHANGED_FIELD_LABEL_KEYS[f]
                    ? t(CHANGED_FIELD_LABEL_KEYS[f])
                    : f}
                </Badge>
              ))}
            </span>
          ),
        }),
        createColumn<TollPassRevision>({
          id: "amount",
          accessorFn: (row) => row.amount_after,
          labelKey: "tolls.revisions.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {tollMoney(row.original.amount_before, pass.currency, locale)} →{" "}
              <span className="font-medium">
                {tollMoney(row.original.amount_after, pass.currency, locale)}
              </span>
            </span>
          ),
        }),
        createColumn<TollPassRevision>({
          accessorKey: "source",
          labelKey: "tolls.revisions.columns.source",
          enableSorting: false,
          cell: ({ row }) => t(SOURCE_LABEL_KEYS[row.original.source]),
        }),
        createColumn<TollPassRevision>({
          accessorKey: "created_at",
          labelKey: "tolls.revisions.columns.created_at",
          enableSorting: false,
          cell: ({ row }) =>
            datetime(row.original.created_at, "d MMM yyyy HH:mm", locale),
        }),
      ] as ColumnDef<TollPassRevision, unknown>[],
    [locale, pass.currency, t],
  );
  return (
    <EntitySectionCard
      title={t("tolls.revisions.title")}
      badge={pass.revisions.length}
    >
      <EntityTable
        columns={columns}
        data={pass.revisions}
        getRowId={(row) => row.uuid}
        emptyTitle={t("tolls.revisions.empty")}
        emptyDescription={t("tolls.revisions.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-toll-pass-revisions-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
    </EntitySectionCard>
  );
}

function statusText(t: (key: string) => string, status: string) {
  const key = TOLL_STATUS_LABEL_KEYS[status as TollStatus];
  return key ? t(key) : status;
}

/**
 * One toll pass: the facts the provider (or the file) reported, the
 * vehicle / rental it was matched to, every charge it caused (reversals
 * included), its revisions and the timeline of decisions with reasons.
 */
export function TollPassDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const query = useTollPass(uuid);
  const [assignOpen, setAssignOpen] = useState(false);
  const [reason, setReason] = useState<ReasonAction | null>(null);
  const p = query.data;
  const canReview = can(permissions.tenant.tolls.review);

  return (
    <EntityPage
      title={t("tolls.detail.title")}
      permission={permissions.tenant.tolls.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tolls.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("tolls.title"), href: routes.tenant.tolls.root(slug) },
        { label: p ? p.external_id : t("tolls.detail.title") },
      ]}
      actions={
        p && canReview ? (
          <div className="flex flex-wrap gap-2">
            <Button type="button" size="sm" onClick={() => setAssignOpen(true)}>
              <Link2 className="size-4" />
              {p.rental
                ? t("tolls.actions.reassign")
                : t("tolls.actions.assign")}
            </Button>
            {p.rental ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setReason("unassign")}
              >
                <Unlink className="size-4" />
                {t("tolls.actions.unassign")}
              </Button>
            ) : null}
            {p.status !== "ignored" ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setReason("ignore")}
              >
                <Ban className="size-4" />
                {t("tolls.actions.ignore")}
              </Button>
            ) : null}
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setReason("rematch")}
            >
              <RotateCcw className="size-4" />
              {t("tolls.actions.rematch")}
            </Button>
          </div>
        ) : null
      }
    >
      {query.isLoading ? <Loading /> : null}
      {query.isError ? (
        <ErrorState
          title={t("tolls.detail.not_found")}
          description={t("tolls.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {p ? (
        <div className="space-y-6">
          <EntityHeader
            title={stationText(p)}
            subtitle={
              <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className="tabular-nums">
                  {datetime(p.passed_at, "d MMM yyyy HH:mm", locale)}
                </span>
                <span className="font-mono">{p.plate || p.tag_number}</span>
                <span className="font-semibold tabular-nums">
                  {tollMoney(p.amount, p.currency, locale)}
                </span>
              </span>
            }
            badges={
              <>
                <TollStatusChip status={p.status} />
                <ReviewReasonBadge reason={p.review_reason} />
                {p.manual ? (
                  <Badge variant="secondary">{t("tolls.manual_badge")}</Badge>
                ) : null}
                {p.revision > 1 ? (
                  <Badge variant="outline">
                    {t("tolls.revision_badge", { revision: p.revision })}
                  </Badge>
                ) : null}
              </>
            }
          />
          <EntitySectionCard title={t("tolls.detail.facts")}>
            <EntityDetail
              sections={[
                {
                  id: "facts",
                  fields: [
                    {
                      key: "passed_at",
                      label: t("tolls.columns.passed_at"),
                      value: datetime(
                        p.passed_at,
                        "d MMM yyyy HH:mm:ss",
                        locale,
                      ),
                    },
                    {
                      key: "station",
                      label: t("tolls.columns.station"),
                      value: stationText(p),
                    },
                    {
                      key: "plate",
                      label: t("tolls.columns.plate"),
                      value: p.plate || "—",
                    },
                    {
                      key: "tag",
                      label: t("tolls.columns.tag_number"),
                      value: p.tag_number || "—",
                    },
                    {
                      key: "amount",
                      label: t("tolls.columns.amount"),
                      value: tollMoney(p.amount, p.currency, locale),
                    },
                    {
                      key: "fee",
                      label: t("tolls.fields.fee"),
                      value: p.fee_amount
                        ? tollMoney(p.fee_amount, p.currency, locale)
                        : "—",
                    },
                    {
                      key: "external_id",
                      label: t("tolls.columns.external_id"),
                      value: <span className="font-mono">{p.external_id}</span>,
                    },
                    {
                      key: "provider",
                      label: t("tolls.fields.provider"),
                      value: `${p.provider} · ${t(SOURCE_LABEL_KEYS[p.source])}`,
                    },
                    {
                      key: "fetched",
                      label: t("tolls.fields.last_fetched_at"),
                      value: datetime(
                        p.last_fetched_at,
                        "d MMM yyyy HH:mm",
                        locale,
                      ),
                    },
                    {
                      key: "account",
                      label: t("tolls.fields.account"),
                      value: p.account?.display_name ?? "—",
                    },
                  ],
                },
                {
                  id: "match",
                  title: t("tolls.detail.match"),
                  fields: [
                    {
                      key: "status",
                      label: t("tolls.columns.status"),
                      value: statusText(t, p.status),
                    },
                    {
                      key: "reason",
                      label: t("tolls.columns.review_reason"),
                      value: reviewReasonText(t, p.review_reason) || "—",
                    },
                    {
                      key: "vehicle",
                      label: t("tolls.fields.vehicle"),
                      value: p.vehicle ? (
                        <Link
                          href={routes.tenant.fleet.vehicles.detail(
                            slug,
                            p.vehicle.uuid,
                          )}
                          className="font-mono underline-offset-4 hover:underline"
                        >
                          {p.vehicle.plate}
                        </Link>
                      ) : (
                        t("tolls.detail.no_vehicle")
                      ),
                    },
                    {
                      key: "rental",
                      label: t("tolls.columns.rental"),
                      value: p.rental ? (
                        <Link
                          href={routes.tenant.rentals.detail(
                            slug,
                            p.rental.uuid,
                          )}
                          className="font-mono underline-offset-4 hover:underline"
                        >
                          {p.rental.reference}
                        </Link>
                      ) : (
                        t("tolls.no_rental_yet")
                      ),
                    },
                    {
                      key: "method",
                      label: t("tolls.fields.match_method"),
                      value: p.match_method
                        ? t(MATCH_METHOD_LABEL_KEYS[p.match_method])
                        : "—",
                    },
                    {
                      key: "decided",
                      label: t("tolls.fields.decided"),
                      value: p.decided_at
                        ? `${actorName(p.decided_by)} · ${datetime(p.decided_at, "d MMM yyyy HH:mm", locale)}`
                        : "—",
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>
          <ChargesCard pass={p} />
          <RevisionsCard pass={p} />
          <EntitySectionCard title={t("tolls.detail.timeline")}>
            <Timeline
              items={p.events.map((e) => ({
                id: e.uuid,
                title: t(EVENT_LABEL_KEYS[e.action]),
                description: [
                  e.reason,
                  e.from_status && e.to_status && e.from_status !== e.to_status
                    ? `${statusText(t, e.from_status)} → ${statusText(t, e.to_status)}`
                    : "",
                ]
                  .filter(Boolean)
                  .join(" · "),
                meta: (
                  <span className="text-right">
                    {datetime(e.created_at, "d MMM HH:mm", locale)}
                    {e.actor ? (
                      <span className="block">{actorName(e.actor)}</span>
                    ) : null}
                  </span>
                ),
              }))}
            />
          </EntitySectionCard>
          <AssignTollDialog
            pass={assignOpen ? p : null}
            onClose={() => setAssignOpen(false)}
          />
          <TollReasonDialog
            target={reason ? { pass: p, action: reason } : null}
            onClose={() => setReason(null)}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
