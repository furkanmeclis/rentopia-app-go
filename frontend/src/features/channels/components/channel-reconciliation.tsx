"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { format, startOfMonth } from "date-fns";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { createColumn, DataTable } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import {
  amountText,
  errorCodeText,
} from "@/features/channels/components/channel-cells";
import {
  COLLECTED_BY_LABEL_KEYS,
  MATCH_LABEL_KEYS,
  MATCH_STATUSES,
  MATCH_TONES,
} from "@/features/channels/constants";
import {
  useChannelAccounts,
  useChannelReconciliation,
} from "@/features/channels/hooks/use-channels";
import type { ChannelReconciliationLine } from "@/features/channels/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function Total({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-card space-y-1 rounded-lg border p-3">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p className="text-base font-semibold tabular-nums">{value}</p>
    </div>
  );
}

/**
 * Channel vs Rentopia reconciliation of a period: the channel's statement
 * against the channel bookings and their reservations, with totals of what
 * each side owes.
 */
export function ChannelReconciliation() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const accounts = useChannelAccounts();
  const [account, setAccount] = useState("");
  const [from, setFrom] = useState(
    format(startOfMonth(new Date()), "yyyy-MM-dd"),
  );
  const [to, setTo] = useState(format(new Date(), "yyyy-MM-dd"));
  const selected = account || accounts.data?.[0]?.uuid || "";
  const report = useChannelReconciliation(selected, from, to);
  const r = report.data;
  const currency = r?.currency ?? "TRY";

  const columns = useMemo(
    () =>
      [
        createColumn<ChannelReconciliationLine>({
          accessorKey: "external_ref",
          labelKey: "channels.reconciliation.columns.external_ref",
          enableSorting: true,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs font-medium">
              {row.original.external_ref}
            </span>
          ),
        }),
        createColumn<ChannelReconciliationLine>({
          accessorKey: "match",
          labelKey: "channels.reconciliation.columns.match",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: MATCH_STATUSES.map((v) => ({
            value: v,
            labelKey: MATCH_LABEL_KEYS[v],
            label: t(MATCH_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <span className="flex flex-col items-start gap-1">
              <StatusChip
                label={t(MATCH_LABEL_KEYS[row.original.match])}
                tone={MATCH_TONES[row.original.match]}
              />
              {row.original.error_code ? (
                <span className="text-muted-foreground text-[11px]">
                  {errorCodeText(t, row.original.error_code)}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<ChannelReconciliationLine>({
          accessorKey: "starts_at",
          labelKey: "channels.reconciliation.columns.starts_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.starts_at
              ? datetime(row.original.starts_at, "d MMM yyyy HH:mm", locale)
              : "—",
        }),
        createColumn<ChannelReconciliationLine>({
          accessorKey: "channel_status",
          labelKey: "channels.reconciliation.columns.channel_status",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.channel_status === "cancelled"
              ? t("channels.reconciliation.cancelled")
              : t("channels.reconciliation.confirmed"),
        }),
        createColumn<ChannelReconciliationLine>({
          id: "channel_total",
          accessorFn: (row) => Number(row.channel_total ?? 0),
          labelKey: "channels.reconciliation.columns.channel_total",
          enableSorting: true,
          cell: ({ row }) =>
            amountText(row.original.channel_total, locale, currency),
        }),
        createColumn<ChannelReconciliationLine>({
          id: "channel_commission",
          accessorFn: (row) => Number(row.channel_commission ?? 0),
          labelKey: "channels.reconciliation.columns.channel_commission",
          enableSorting: false,
          cell: ({ row }) =>
            amountText(row.original.channel_commission, locale, currency),
        }),
        createColumn<ChannelReconciliationLine>({
          id: "reservation",
          accessorFn: (row) => row.reservation?.reference ?? "",
          labelKey: "channels.reconciliation.columns.reservation",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.reservation ? (
              <Link
                href={routes.tenant.reservations.detail(
                  slug,
                  row.original.reservation.uuid,
                )}
                className="font-mono text-xs underline-offset-4 hover:underline"
              >
                {row.original.reservation.reference}
              </Link>
            ) : (
              "—"
            ),
        }),
        createColumn<ChannelReconciliationLine>({
          id: "local_total",
          accessorFn: (row) => Number(row.local_total ?? 0),
          labelKey: "channels.reconciliation.columns.local_total",
          enableSorting: false,
          cell: ({ row }) =>
            amountText(row.original.local_total, locale, currency),
        }),
        createColumn<ChannelReconciliationLine>({
          id: "difference",
          accessorFn: (row) => Number(row.difference ?? 0),
          labelKey: "channels.reconciliation.columns.difference",
          enableSorting: true,
          cell: ({ row }) =>
            amountText(row.original.difference, locale, currency),
        }),
        createColumn<ChannelReconciliationLine>({
          accessorKey: "collected_by",
          labelKey: "channels.reconciliation.columns.collected_by",
          enableSorting: false,
          cell: ({ row }) =>
            COLLECTED_BY_LABEL_KEYS[row.original.collected_by]
              ? t(COLLECTED_BY_LABEL_KEYS[row.original.collected_by])
              : "—",
        }),
      ] as ColumnDef<ChannelReconciliationLine, unknown>[],
    [currency, locale, slug, t],
  );

  if (accounts.isSuccess && (accounts.data?.length ?? 0) === 0) {
    return (
      <Alert>
        <AlertDescription>
          {t("channels.accounts.empty_description")}
        </AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <Select value={selected} onValueChange={setAccount}>
          <SelectTrigger className="w-full sm:w-64">
            <SelectValue placeholder={t("channels.reconciliation.account")} />
          </SelectTrigger>
          <SelectContent>
            {(accounts.data ?? []).map((a) => (
              <SelectItem key={a.uuid} value={a.uuid}>
                {a.display_name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <DatePicker
          value={from}
          onChange={(v) => setFrom(v ?? "")}
          placeholder={t("channels.reconciliation.from")}
          className="w-full sm:w-40"
        />
        <DatePicker
          value={to}
          onChange={(v) => setTo(v ?? "")}
          placeholder={t("channels.reconciliation.to")}
          className="w-full sm:w-40"
        />
      </div>
      {report.isLoading ? (
        <Skeleton className="h-48 w-full" />
      ) : report.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("channels.load_failed")}
          onRetry={() => void report.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : r ? (
        <>
          <p className="text-muted-foreground text-xs">
            {r.source === "statement"
              ? t("channels.reconciliation.source_statement")
              : t("channels.reconciliation.source_local")}
          </p>
          {r.warning ? (
            <Alert>
              <AlertDescription>
                {t("channels.reconciliation.warning")}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <Total
              label={t("channels.reconciliation.totals.channel_gross")}
              value={amountText(r.totals.channel_gross, locale, currency)}
            />
            <Total
              label={t("channels.reconciliation.totals.channel_commission")}
              value={amountText(r.totals.channel_commission, locale, currency)}
            />
            <Total
              label={t("channels.reconciliation.totals.collected_by_channel")}
              value={amountText(
                r.totals.collected_by_channel,
                locale,
                currency,
              )}
            />
            <Total
              label={t("channels.reconciliation.totals.collected_by_us")}
              value={amountText(r.totals.collected_by_us, locale, currency)}
            />
          </div>
          <div className="flex flex-wrap gap-2">
            {MATCH_STATUSES.map((m) => (
              <StatusChip
                key={m}
                label={`${t(MATCH_LABEL_KEYS[m])}: ${r.counts[m] ?? 0}`}
                tone={(r.counts[m] ?? 0) > 0 ? MATCH_TONES[m] : "default"}
              />
            ))}
          </div>
          <DataTable
            columns={columns}
            data={r.lines}
            getRowId={(row) => row.external_ref}
            emptyTitle={t("channels.reconciliation.empty_title")}
            emptyDescription={t("channels.reconciliation.empty_description")}
            initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
            pageSizeOptions={[20, 50, 100]}
            features={{
              sorting: true,
              multiSort: false,
              globalFilter: true,
              columnFilters: true,
              columnVisibility: false,
              columnOrdering: false,
              columnPinning: false,
              columnResizing: false,
              rowSelection: false,
              pagination: r.lines.length > 20,
              density: false,
              viewMode: false,
              facetedFilters: true,
            }}
          />
        </>
      ) : null}
    </div>
  );
}
