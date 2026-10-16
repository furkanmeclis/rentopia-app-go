"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Eye, Scissors, Undo2 } from "lucide-react";
import { useRouter, useSearchParams } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Card, CardContent } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBranchesList } from "@/features/branches/hooks/use-branches";
import {
  DepositMethodBadge,
  DepositStatusChip,
} from "@/features/deposits/components/deposit-cells";
import { DepositDetailDrawer } from "@/features/deposits/components/deposit-detail-drawer";
import {
  DEPOSIT_METHOD_LABEL_KEYS,
  DEPOSIT_METHODS,
  DEPOSIT_STATUS_LABEL_KEYS,
  DEPOSIT_STATUSES,
  DEPOSITS_RESOURCE,
  HELD_OVER_PRESETS,
} from "@/features/deposits/constants";
import {
  useDepositTotals,
  useDepositsList,
  useDepositsMeta,
  useRentalDeposits,
} from "@/features/deposits/hooks/use-deposits";
import type { SecurityDeposit } from "@/features/deposits/types";
import { Amount } from "@/features/finance/components/finance-cells";
import type { ListParams } from "@/features/finance/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const ANY = "any";

function values(value: unknown): string[] {
  if (Array.isArray(value)) return value.map(String).filter(Boolean);
  if (typeof value === "string" && value) return [value];
  return [];
}

function TotalCard({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <Card className="py-4">
      <CardContent className="space-y-1 px-4">
        <p className="text-muted-foreground text-xs">{label}</p>
        <div className="text-lg font-semibold">{children}</div>
      </CardContent>
    </Card>
  );
}

/**
 * "Depozitolar": every deposit / pre-authorization with status, method,
 * branch and date filters and "held over N days"; a row opens the deposit
 * (resolution, documents). `?rental=<uuid>` (the deposit task link) opens
 * that rental's open deposit with its resolution.
 */
export function DepositsPage() {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const router = useRouter();
  const search = useSearchParams();
  const rentalParam = search.get("rental");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [heldOver, setHeldOver] = useState<string>(ANY);
  const [selected, setSelected] = useState<string | null>(null);
  const [autoResolve, setAutoResolve] = useState(false);
  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, DEPOSITS_RESOURCE),
  });
  const branches = useBranchesList({ limit: 100, offset: 0, sort: "code" });
  const totals = useDepositTotals(
    heldOver !== ANY ? Number(heldOver) : undefined,
  );
  const rentalCard = useRentalDeposits(rentalParam, Boolean(rentalParam));
  const [handledRental, setHandledRental] = useState<string | null>(null);
  if (rentalParam && rentalCard.data && handledRental !== rentalParam) {
    setHandledRental(rentalParam);
    const open = rentalCard.data.open_deposit_uuid;
    const latest = rentalCard.data.items[0]?.uuid ?? null;
    if (open || latest) {
      setSelected(open ?? latest);
      setAutoResolve(Boolean(open));
    }
  }
  const params = useMemo<ListParams>(() => {
    const col = (id: string) =>
      values(listState.columnFilters.find((f) => f.id === id)?.value);
    const join = (v: string[]) => (v.length ? v.join(",") : undefined);
    return {
      ...listState.params,
      status: join(col("status")),
      method: join(col("method")),
      branch: join(col("branch")),
      rental: rentalParam ?? undefined,
      from: from || undefined,
      to: to || undefined,
      held_over_days: heldOver !== ANY ? heldOver : undefined,
    };
  }, [
    from,
    heldOver,
    listState.columnFilters,
    listState.params,
    rentalParam,
    to,
  ]);
  const query = useDepositsList(params);
  const meta = useDepositsMeta();

  const columns = useMemo(
    () =>
      [
        createColumn<SecurityDeposit>({
          accessorKey: "created_at",
          labelKey: "deposits.columns.created_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {datetime(row.original.created_at, "dd.MM.yyyy HH:mm", locale)}
            </span>
          ),
        }),
        createColumn<SecurityDeposit>({
          id: "rental",
          accessorFn: (row) => row.rental.code ?? "",
          labelKey: "deposits.columns.rental",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div className="space-y-0.5">
              <div className="font-mono text-xs font-medium">
                {row.original.rental.code}
              </div>
              <div className="text-muted-foreground max-w-[14rem] truncate text-xs">
                {row.original.customer.name || "—"}
              </div>
            </div>
          ),
        }),
        createColumn<SecurityDeposit>({
          id: "branch",
          accessorFn: (row) => row.branch.code ?? "",
          labelKey: "deposits.columns.branch",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: (branches.data?.items ?? []).map((b) => ({
            value: b.uuid,
            label: `${b.code} · ${b.name}`,
          })),
          cell: ({ row }) => row.original.branch.name,
        }),
        createColumn<SecurityDeposit>({
          accessorKey: "method",
          labelKey: "deposits.columns.method",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: DEPOSIT_METHODS.map((m) => ({
            value: m,
            labelKey: DEPOSIT_METHOD_LABEL_KEYS[m],
            label: t(DEPOSIT_METHOD_LABEL_KEYS[m]),
          })),
          cell: ({ row }) => (
            <DepositMethodBadge method={row.original.method} />
          ),
        }),
        createColumn<SecurityDeposit>({
          accessorKey: "amount",
          labelKey: "deposits.columns.amount",
          enableSorting: true,
          cell: ({ row }) => (
            <div className="space-y-0.5">
              <Amount
                value={row.original.amount}
                currency={row.original.currency}
                className="font-semibold"
              />
              {Number(row.original.captured_amount) > 0 ? (
                <div className="text-muted-foreground text-xs">
                  {t("deposits.columns.captured")}:{" "}
                  <Amount
                    value={row.original.captured_amount}
                    currency={row.original.currency}
                  />
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<SecurityDeposit>({
          accessorKey: "status",
          labelKey: "deposits.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: DEPOSIT_STATUSES.map((s) => ({
            value: s,
            labelKey: DEPOSIT_STATUS_LABEL_KEYS[s],
            label: t(DEPOSIT_STATUS_LABEL_KEYS[s]),
          })),
          cell: ({ row }) => <DepositStatusChip status={row.original.status} />,
        }),
        createColumn<SecurityDeposit>({
          accessorKey: "held_at",
          labelKey: "deposits.columns.held_at",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.held_at ? (
              <div className="space-y-0.5 whitespace-nowrap">
                <div className="tabular-nums">
                  {datetime(row.original.held_at, "dd.MM.yyyy", locale)}
                </div>
                {row.original.status === "held" ? (
                  <div className="text-muted-foreground text-xs">
                    {t("deposits.detail.held_days", {
                      days: row.original.held_days,
                    })}
                  </div>
                ) : null}
              </div>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<SecurityDeposit>({
          id: "cash_account",
          accessorFn: (row) => row.cash_account?.code ?? row.pos_reference,
          labelKey: "deposits.columns.cash_account",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.cash_account
              ? `${row.original.cash_account.code} · ${row.original.cash_account.name}`
              : row.original.pos_reference || "—",
        }),
        createColumn<SecurityDeposit>({
          id: "actions",
          labelKey: "deposits.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const d = row.original;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "open",
                    label: t("deposits.actions.open"),
                    icon: Eye,
                    onSelect: () => {
                      setAutoResolve(false);
                      setSelected(d.uuid);
                    },
                  },
                  {
                    id: "resolve",
                    label: t("deposits.actions.resolve"),
                    icon: d.capabilities.deduct ? Scissors : Undo2,
                    permission: permissions.tenant.finance.deposits.write,
                    disabled: !d.capabilities.release,
                    onSelect: () => {
                      setAutoResolve(true);
                      setSelected(d.uuid);
                    },
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<SecurityDeposit, unknown>[],
    [branches.data?.items, locale, t],
  );

  const engineQuery = {
    q: params.q,
    status: params.status as string | undefined,
    method: params.method as string | undefined,
    branch: params.branch as string | undefined,
    rental: params.rental as string | undefined,
    from: params.from as string | undefined,
    to: params.to as string | undefined,
    held_over_days: params.held_over_days as string | undefined,
    sort: params.sort,
  };
  const total = query.data?.total ?? 0;
  const hasFilters = Boolean(
    params.q ||
    params.status ||
    params.method ||
    params.branch ||
    params.rental ||
    params.from ||
    params.to ||
    params.held_over_days,
  );
  const refresh = () => {
    void query.refetch();
    void totals.refetch();
  };
  const sums = totals.data;

  return (
    <EntityPage
      title={t("deposits.title")}
      description={t("deposits.description")}
      permission={permissions.tenant.finance.deposits.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("deposits.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("deposits.title") },
      ]}
    >
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <TotalCard label={t("deposits.totals.held")}>
          <Amount value={sums?.held_amount} currency={sums?.currency} />
          <span className="text-muted-foreground ml-2 text-xs font-normal">
            {t("deposits.totals.count", { count: sums?.held_count ?? 0 })}
          </span>
        </TotalCard>
        <TotalCard label={t("deposits.totals.cash")}>
          <Amount
            value={
              sums
                ? (
                    Number(sums.by_method.cash ?? 0) +
                    Number(sums.by_method.bank_transfer ?? 0)
                  ).toFixed(2)
                : undefined
            }
            currency={sums?.currency}
          />
        </TotalCard>
        <TotalCard label={t("deposits.totals.preauth")}>
          <Amount
            value={
              sums
                ? (
                    Number(sums.by_method.pos_preauth ?? 0) +
                    Number(sums.by_method.online_preauth ?? 0)
                  ).toFixed(2)
                : undefined
            }
            currency={sums?.currency}
          />
        </TotalCard>
        <TotalCard
          label={t("deposits.totals.held_over", {
            days: sums?.held_over_days ?? 7,
          })}
        >
          {sums?.held_over ?? 0}
          {sums && sums.pending_count > 0 ? (
            <span className="text-muted-foreground ml-2 text-xs font-normal">
              {t("deposits.totals.pending", { count: sums.pending_count })}
            </span>
          ) : null}
        </TotalCard>
      </div>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) => {
          setAutoResolve(false);
          setSelected(row.uuid);
        }}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("deposits.load_failed")}
        onRetry={refresh}
        realtime={listState.realtimeFor(refresh)}
        emptyTitle={
          hasFilters ? t("deposits.empty_filtered") : t("deposits.empty")
        }
        emptyDescription={
          hasFilters ? undefined : t("deposits.empty_description")
        }
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 20)),
        )}
        state={listState.tableState}
        features={{
          persistKey: "tenant-finance-deposits-v1",
          rowSelection: false,
          viewMode: true,
        }}
        toolbarExtra={
          <>
            <div className="grid w-full grid-cols-2 gap-2 sm:w-auto">
              <DatePicker
                value={from}
                onChange={setFrom}
                placeholder={t("deposits.filters.from")}
              />
              <DatePicker
                value={to}
                onChange={setTo}
                placeholder={t("deposits.filters.to")}
              />
            </div>
            <Select value={heldOver} onValueChange={setHeldOver}>
              <SelectTrigger
                className="w-full sm:w-[12rem]"
                aria-label={t("deposits.filters.held_over")}
              >
                <SelectValue placeholder={t("deposits.filters.held_over")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ANY}>
                  {t("deposits.filters.held_over_any")}
                </SelectItem>
                {HELD_OVER_PRESETS.map((d) => (
                  <SelectItem key={d} value={String(d)}>
                    {t("deposits.filters.held_over_days", { days: d })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <ResourceIOToolbar
              resource={DEPOSITS_RESOURCE}
              query={engineQuery}
              capabilities={meta.data?.capabilities}
            />
            <EntityToolbar
              onRefresh={refresh}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
      <DepositDetailDrawer
        uuid={selected}
        autoResolve={autoResolve}
        onOpenChange={(next) => {
          if (!next) {
            setSelected(null);
            setAutoResolve(false);
            if (rentalParam) {
              router.replace(routes.tenant.finance.deposits(slug));
            }
          }
        }}
      />
    </EntityPage>
  );
}
