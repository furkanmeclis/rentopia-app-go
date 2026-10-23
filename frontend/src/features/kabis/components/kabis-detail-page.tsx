"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { CheckCheck, Download, FileCheck2, RotateCcw } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  KabisResolveDialog,
  useKabisRetryAction,
} from "@/features/kabis/components/kabis-actions";
import {
  KabisOfficialBadge,
  KabisStatusChip,
  holdText,
  kabisEventText,
  kabisStatusText,
  missingFieldText,
} from "@/features/kabis/components/kabis-cells";
import {
  KABIS_CAUSE_LABEL_KEYS,
  KABIS_ROLE_LABEL_KEYS,
  KABIS_STATUS_HINT_KEYS,
} from "@/features/kabis/constants";
import {
  useDownloadKabisAttachment,
  useKabisNotification,
} from "@/features/kabis/hooks/use-kabis";
import type {
  KabisNotificationDetail,
  KabisResolution,
} from "@/features/kabis/types";
import { IntegrationAttemptsTable } from "@/features/payments/components/integration-jobs";
import { actorName } from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { fileSize } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useFeature } from "@/providers/feature-provider";
import { useLocale } from "@/providers/locale-provider";

type Person = {
  id: string;
  role: string;
  name: string;
  identity: string;
  licence: string;
  nationality: string;
};

const DATE = "d MMM yyyy HH:mm";

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function obj(v: unknown): Record<string, unknown> {
  return v && typeof v === "object" ? (v as Record<string, unknown>) : {};
}

/** Renter and drivers from the masked summary (hints only). */
function peopleOf(summary: Record<string, unknown>): Person[] {
  const one = (p: Record<string, unknown>, id: string): Person => ({
    id,
    role: str(p.role),
    name:
      str(p.company_title) ||
      [str(p.first_name), str(p.last_name)].filter(Boolean).join(" "),
    identity: str(p.identity_hint),
    licence: str(p.licence_hint),
    nationality: str(p.nationality),
  });
  const out: Person[] = [];
  if (summary.renter) out.push(one(obj(summary.renter), "renter"));
  const drivers = Array.isArray(summary.drivers) ? summary.drivers : [];
  drivers.forEach((d, i) => out.push(one(obj(d), `driver-${i}`)));
  return out;
}

function PeopleCard({ n }: { n: KabisNotificationDetail }) {
  const { t } = useLocale();
  const people = useMemo(() => peopleOf(n.summary), [n.summary]);
  const columns = useMemo(
    () =>
      [
        createColumn<Person>({
          accessorKey: "role",
          labelKey: "kabis.people.columns.role",
          enableSorting: false,
          cell: ({ row }) =>
            t(KABIS_ROLE_LABEL_KEYS[row.original.role] ?? row.original.role),
        }),
        createColumn<Person>({
          accessorKey: "name",
          labelKey: "kabis.people.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name || "—"}</span>
          ),
        }),
        createColumn<Person>({
          accessorKey: "identity",
          labelKey: "kabis.people.columns.identity",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.identity || "—"}
            </span>
          ),
        }),
        createColumn<Person>({
          accessorKey: "licence",
          labelKey: "kabis.people.columns.licence",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-mono text-xs">
              {row.original.role === "renter"
                ? ""
                : row.original.licence || "—"}
            </span>
          ),
        }),
        createColumn<Person>({
          accessorKey: "nationality",
          labelKey: "kabis.people.columns.nationality",
          enableSorting: false,
          cell: ({ row }) => row.original.nationality || "—",
        }),
      ] as ColumnDef<Person, unknown>[],
    [t],
  );
  return (
    <EntitySectionCard title={t("kabis.detail.people")} badge={people.length}>
      <p className="text-muted-foreground mb-3 text-xs">
        {t("kabis.detail.people_hint")}
      </p>
      <EntityTable
        columns={columns}
        data={people}
        getRowId={(row) => row.id}
        emptyTitle={t("kabis.people.empty")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-kabis-people-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
        }}
      />
    </EntitySectionCard>
  );
}

/**
 * One KABİS notification: its state (official or a local record), why it
 * waits, what is missing, the masked facts it reports, the staff
 * resolution and every provider call.
 */
export function KabisDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const query = useKabisNotification(uuid);
  const retry = useKabisRetryAction();
  const kabis = useFeature("kabis");
  const download = useDownloadKabisAttachment();
  const [resolve, setResolve] = useState<KabisResolution | null>(null);
  const n = query.data;
  const place = obj(n?.summary.place);
  const vehicle = obj(n?.summary.vehicle);

  return (
    <EntityPage
      title={t("kabis.detail.title")}
      permission={permissions.tenant.kabis.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("kabis.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("kabis.title"), href: routes.tenant.kabis.root(slug) },
        {
          label: n
            ? `${n.rental.reference} · ${kabisEventText(t, n.event, n.sequence)}`
            : t("kabis.detail.title"),
        },
      ]}
      actions={
        n ? (
          <div className="flex flex-wrap gap-2">
            {kabis && n.actions.retry ? (
              <Button
                type="button"
                size="sm"
                disabled={retry.pending}
                onClick={() => retry.run(n.uuid)}
              >
                <RotateCcw className="size-4" />
                {n.actions.revalidates
                  ? t("kabis.actions.revalidate")
                  : t("kabis.actions.retry")}
              </Button>
            ) : null}
            {n.actions.mark_manual ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setResolve("mark-manual")}
              >
                <FileCheck2 className="size-4" />
                {t("kabis.actions.mark_manual")}
              </Button>
            ) : null}
            {n.actions.reconcile ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setResolve("reconcile")}
              >
                <CheckCheck className="size-4" />
                {t("kabis.actions.reconcile")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {query.isLoading ? <Loading /> : null}
      {query.isError ? (
        <ErrorState
          title={t("kabis.detail.not_found")}
          description={t("kabis.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {n ? (
        <div className="space-y-6">
          <EntityHeader
            title={`${kabisEventText(t, n.event, n.sequence)} · ${n.rental.reference}`}
            subtitle={
              <span className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <span className="tabular-nums">
                  {datetime(n.occurred_at, DATE, locale)}
                </span>
                <span className="font-mono">{n.plate || "—"}</span>
                <span>{n.branch.name}</span>
              </span>
            }
            badges={
              <>
                <KabisStatusChip status={n.status} />
                <KabisOfficialBadge status={n.status} />
              </>
            }
          />
          <Alert
            variant={
              n.status === "blocked" ||
              n.status === "failed" ||
              n.status === "needs_verification"
                ? "destructive"
                : "default"
            }
          >
            <AlertTitle>{kabisStatusText(t, n.status)}</AlertTitle>
            <AlertDescription>
              <p>{t(KABIS_STATUS_HINT_KEYS[n.status])}</p>
              {n.status === "waiting_dependency" && n.hold_reason ? (
                <p>{holdText(t, n.hold_reason)}</p>
              ) : null}
              {n.missing_fields.length ? (
                <ul className="mt-1 list-disc ps-5">
                  {n.missing_fields.map((f) => (
                    <li key={f}>{missingFieldText(t, f)}</li>
                  ))}
                </ul>
              ) : null}
            </AlertDescription>
          </Alert>
          <EntitySectionCard title={t("kabis.detail.state")}>
            <EntityDetail
              sections={[
                {
                  id: "state",
                  fields: [
                    {
                      key: "attempts",
                      label: t("kabis.columns.attempts"),
                      value: `${n.attempts} / ${n.max_attempts}`,
                    },
                    {
                      key: "next",
                      label: t("kabis.fields.next_attempt_at"),
                      value: n.next_attempt_at
                        ? datetime(n.next_attempt_at, DATE, locale)
                        : "—",
                    },
                    {
                      key: "verified",
                      label: t("kabis.columns.last_verified_at"),
                      value: n.last_verified_at
                        ? datetime(n.last_verified_at, DATE, locale)
                        : t("kabis.fields.never_verified"),
                    },
                    {
                      key: "reference",
                      label: t("kabis.columns.external_reference"),
                      value: (
                        <span className="font-mono">
                          {n.external_reference || "—"}
                        </span>
                      ),
                    },
                    {
                      key: "adapter",
                      label: t("kabis.fields.adapter"),
                      value: n.account?.name ?? n.provider,
                    },
                    {
                      key: "cause",
                      label: t("kabis.fields.cause"),
                      value: t(KABIS_CAUSE_LABEL_KEYS[n.cause] ?? n.cause),
                    },
                    {
                      key: "rental",
                      label: t("kabis.columns.rental"),
                      value: (
                        <Link
                          href={routes.tenant.rentals.detail(
                            slug,
                            n.rental.uuid,
                          )}
                          className="font-mono underline-offset-4 hover:underline"
                        >
                          {n.rental.reference}
                        </Link>
                      ),
                    },
                    ...(n.corrects_uuid
                      ? [
                          {
                            key: "corrects",
                            label: t("kabis.fields.corrects"),
                            value: (
                              <Link
                                href={routes.tenant.kabis.detail(
                                  slug,
                                  n.corrects_uuid,
                                )}
                                className="underline-offset-4 hover:underline"
                              >
                                {t("kabis.fields.open_corrected")}
                              </Link>
                            ),
                          },
                        ]
                      : []),
                    {
                      key: "error",
                      label: t("kabis.columns.last_error"),
                      // Held states explain themselves above (localized).
                      value:
                        n.last_error &&
                        n.status !== "blocked" &&
                        n.status !== "waiting_dependency" ? (
                          <span className="text-destructive break-words">
                            {n.last_error}
                          </span>
                        ) : (
                          "—"
                        ),
                    },
                  ],
                },
                {
                  id: "facts",
                  title: t("kabis.detail.facts"),
                  fields: [
                    {
                      key: "occurred",
                      label: t("kabis.columns.occurred_at"),
                      value: datetime(
                        n.occurred_at,
                        "d MMM yyyy HH:mm:ss",
                        locale,
                      ),
                    },
                    {
                      key: "place",
                      label: t("kabis.fields.place"),
                      value:
                        [
                          str(place.name),
                          str(place.point),
                          str(place.district),
                          str(place.city),
                        ]
                          .filter(Boolean)
                          .join(" · ") || "—",
                    },
                    {
                      key: "address",
                      label: t("kabis.fields.address"),
                      value: str(place.address) || "—",
                    },
                    {
                      key: "plate",
                      label: t("kabis.columns.plate"),
                      value: (
                        <span className="font-mono">
                          {str(vehicle.plate) || "—"}
                        </span>
                      ),
                    },
                    {
                      key: "vehicle",
                      label: t("kabis.fields.vehicle"),
                      value: str(vehicle.brand_model) || "—",
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>
          <PeopleCard n={n} />
          {n.resolution ? (
            <EntitySectionCard title={t("kabis.detail.resolution")}>
              <EntityDetail
                sections={[
                  {
                    id: "resolution",
                    fields: [
                      {
                        key: "kind",
                        label: t("kabis.fields.resolution"),
                        value: kabisStatusText(t, n.status),
                      },
                      {
                        key: "reference",
                        label: t("kabis.fields.reference"),
                        value: (
                          <span className="font-mono">
                            {n.resolution_reference}
                          </span>
                        ),
                      },
                      {
                        key: "reason",
                        label: t("kabis.fields.reason"),
                        value: n.resolution_reason,
                      },
                      {
                        key: "by",
                        label: t("kabis.fields.resolved_by"),
                        value: `${n.resolved_by ? actorName(n.resolved_by) : "—"} · ${
                          n.resolved_at
                            ? datetime(n.resolved_at, DATE, locale)
                            : ""
                        }`,
                      },
                      {
                        key: "attachment",
                        label: t("kabis.fields.attachment"),
                        value: n.attachment ? (
                          <Button
                            type="button"
                            variant="outline"
                            size="sm"
                            disabled={download.isPending}
                            onClick={() =>
                              download.mutate({
                                uuid: n.uuid,
                                fallbackName: n.attachment?.name ?? "kabis",
                              })
                            }
                          >
                            <Download className="size-4" />
                            {n.attachment.name} · {fileSize(n.attachment.size)}
                          </Button>
                        ) : (
                          "—"
                        ),
                      },
                    ],
                  },
                ]}
              />
            </EntitySectionCard>
          ) : null}
          <EntitySectionCard
            title={t("kabis.detail.attempts")}
            badge={n.attempt_log.length}
          >
            <p className="text-muted-foreground mb-3 text-xs">
              {t("kabis.detail.attempts_hint")}
            </p>
            <IntegrationAttemptsTable
              attempts={n.attempt_log}
              persistKey="tenant-kabis-attempts-v1"
            />
          </EntitySectionCard>
          {n.status === "needs_verification" ? (
            <Badge variant="outline" className="font-normal">
              {t("kabis.detail.no_blind_retry")}
            </Badge>
          ) : null}
          <KabisResolveDialog
            target={resolve ? { uuid: n.uuid, action: resolve } : null}
            onClose={() => setResolve(null)}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
