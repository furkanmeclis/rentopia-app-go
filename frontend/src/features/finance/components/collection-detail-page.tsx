"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ArrowUpFromLine, Ban, Split, Unlink } from "lucide-react";
import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityHeader,
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  AllocationStatusChip,
  CollectionStatusChip,
} from "@/features/finance/components/collection-columns";
import {
  AllocateDialog,
  RefundCollectionDialog,
  ReleaseAllocationDialog,
  VoidCollectionDialog,
} from "@/features/finance/components/collection-dialogs";
import { Amount } from "@/features/finance/components/finance-cells";
import { chargeKindLabel } from "@/features/finance/components/open-items-allocator";
import {
  ITEM_STATUS_LABEL_KEYS,
  ITEM_STATUS_TONES,
  METHOD_LABEL_KEYS,
  PURPOSE_LABEL_KEYS,
  REFUND_STATUS_LABEL_KEYS,
  REFUND_STATUS_TONES,
  SETTLEMENT_STATUS_LABEL_KEYS,
} from "@/features/finance/constants";
import { useCollection } from "@/features/finance/hooks/use-finance";
import { reasonText } from "@/features/finance/lib/format";
import type {
  EntryPurpose,
  FinanceCollection,
  FinanceCollectionAllocation,
  FinanceCollectionRefund,
} from "@/features/finance/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Dialog = "allocate" | "refund" | "void" | null;

/**
 * Collection detail: the money figures (gross / commission / net for online
 * payments), where it landed, the open items it pays (with release) and its
 * refunds (provider result for online ones); refund, void and allocate.
 */
export function CollectionDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const query = useCollection(uuid);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [release, setRelease] = useState<FinanceCollectionAllocation | null>(
    null,
  );
  const c = query.data;
  const canAllocate = can(permissions.tenant.finance.accounts.write);
  const canRefund = can(permissions.tenant.payment.refund);
  const canVoid = can(permissions.tenant.finance.ledger.adjust);
  const onOpen = (d: Dialog) => (open: boolean) => setDialog(open ? d : null);
  const online = c?.method === "online";

  return (
    <EntityPage
      title={c?.code ?? t("finance.collections.title")}
      description={c?.account.name ?? undefined}
      permission={permissions.tenant.finance.accounts.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("finance.collections.title"),
          href: routes.tenant.finance.collections(slug),
        },
        { label: c?.code ?? "…" },
      ]}
      actions={
        c ? (
          <EntityActions>
            {canAllocate && Number(c.unallocated_amount) > 0 ? (
              <Button
                type="button"
                size="sm"
                onClick={() => setDialog("allocate")}
              >
                <Split className="size-4" />
                {t("finance.actions.allocate")}
              </Button>
            ) : null}
            {canRefund && Number(c.refundable) > 0 ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setDialog("refund")}
              >
                <ArrowUpFromLine className="size-4" />
                {t("finance.actions.refund")}
              </Button>
            ) : null}
            {canVoid && c.voidable ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setDialog("void")}
              >
                <Ban className="size-4" />
                {t("finance.actions.void")}
              </Button>
            ) : null}
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("finance.collections.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {c ? (
        <div className="space-y-6">
          <EntityHeader
            title={c.code}
            subtitle={
              <span className="flex flex-wrap items-center gap-2">
                <Link
                  href={routes.tenant.finance.account(slug, c.account.uuid)}
                  className="hover:underline"
                >
                  {c.account.name || c.account.code}
                </Link>
                <span>·</span>
                <span>{t(METHOD_LABEL_KEYS[c.method])}</span>
                <span>·</span>
                <span className="tabular-nums">
                  {date(c.collection_date, "dd.MM.yyyy", locale)}
                </span>
              </span>
            }
            badges={
              <>
                <CollectionStatusChip c={c} />
                <AllocationStatusChip c={c} />
                {c.settlement_status !== "none" ? (
                  <Badge variant="outline">
                    {t(SETTLEMENT_STATUS_LABEL_KEYS[c.settlement_status])}
                  </Badge>
                ) : null}
              </>
            }
          >
            <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <Figure label={t("finance.columns.amount")}>
                <Amount
                  value={c.amount}
                  currency={c.currency}
                  className="text-lg font-semibold"
                />
              </Figure>
              <Figure label={t("finance.columns.allocated")}>
                <Amount value={c.allocated_amount} currency={c.currency} />
              </Figure>
              <Figure label={t("finance.columns.credit_left")}>
                <Amount value={c.unallocated_amount} currency={c.currency} />
              </Figure>
              <Figure label={t("finance.columns.refunded")}>
                <Amount value={c.refunded_amount} currency={c.currency} />
              </Figure>
              {online ? (
                <>
                  <Figure label={t("finance.columns.gross")}>
                    <Amount value={c.gross_amount} currency={c.currency} />
                  </Figure>
                  <Figure label={t("finance.columns.commission")}>
                    <Amount
                      value={c.commission_amount}
                      currency={c.currency}
                      className="text-destructive"
                    />
                  </Figure>
                  <Figure label={t("finance.columns.net")}>
                    <Amount value={c.net_amount} currency={c.currency} />
                  </Figure>
                </>
              ) : null}
            </dl>
          </EntityHeader>

          <EntitySectionCard title={t("finance.collection.details")}>
            <CollectionFacts c={c} />
          </EntitySectionCard>

          <AllocationsCard
            c={c}
            onRelease={
              canAllocate && c.status !== "void" ? setRelease : undefined
            }
          />
          <RefundsCard c={c} />

          <AllocateDialog
            collection={c}
            open={dialog === "allocate"}
            onOpenChange={onOpen("allocate")}
          />
          <RefundCollectionDialog
            collection={c}
            open={dialog === "refund"}
            onOpenChange={onOpen("refund")}
          />
          <VoidCollectionDialog
            collection={c}
            open={dialog === "void"}
            onOpenChange={onOpen("void")}
          />
          <ReleaseAllocationDialog
            collection={c}
            allocation={release}
            onOpenChange={(open) => {
              if (!open) setRelease(null);
            }}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}

function Figure({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="truncate">{children}</dd>
    </div>
  );
}

function CollectionFacts({ c }: { c: FinanceCollection }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const rows: { label: string; value: ReactNode }[] = [
    {
      label: t("finance.fields.method"),
      value: t(METHOD_LABEL_KEYS[c.method]),
    },
    {
      label: t("finance.fields.reference_no"),
      value: c.reference_no || "—",
    },
    { label: t("finance.fields.payer_name"), value: c.payer_name || "—" },
    {
      label: t("finance.fields.purpose"),
      value: t(PURPOSE_LABEL_KEYS[c.purpose as EntryPurpose]),
    },
    {
      label: t("finance.fields.branch"),
      value: c.branch?.name ?? t("finance.head_office"),
    },
    {
      label: t("finance.fields.cash_account"),
      value: c.cash_account ? (
        <Link
          href={routes.tenant.finance.cashAccount(slug, c.cash_account.uuid)}
          className="hover:underline"
        >
          {c.cash_account.code} · {c.cash_account.name}
        </Link>
      ) : online(c) ? (
        t("finance.collection.settlement_pending")
      ) : (
        "—"
      ),
    },
    {
      label: t("finance.fields.rental"),
      value: c.rental ? (
        <Link
          href={routes.tenant.rentals.detail(slug, c.rental.uuid)}
          className="hover:underline"
        >
          {c.rental.code}
        </Link>
      ) : (
        (c.reservation?.code ?? "—")
      ),
    },
    {
      label: t("finance.fields.recorded_by"),
      value: c.created_by
        ? `${c.created_by.name} ${c.created_by.surname}`.trim() ||
          c.created_by.email
        : t("finance.collection.system"),
    },
    {
      label: t("finance.fields.recorded_at"),
      value: datetime(c.created_at, undefined, locale),
    },
  ];
  if (c.settled_at) {
    rows.push({
      label: t("finance.fields.settled_at"),
      value: datetime(c.settled_at, undefined, locale),
    });
  }
  if (c.voided_at) {
    rows.push({
      label: t("finance.fields.void_reason"),
      value: c.void_reason,
    });
  }
  return (
    <div className="space-y-4">
      <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {rows.map((r) => (
          <div key={r.label} className="min-w-0 space-y-1">
            <dt className="text-muted-foreground text-xs">{r.label}</dt>
            <dd className="text-sm break-words">{r.value}</dd>
          </div>
        ))}
      </dl>
      {c.note ? (
        <p className="text-muted-foreground text-sm">{c.note}</p>
      ) : null}
    </div>
  );
}

function online(c: FinanceCollection): boolean {
  return c.method === "online";
}

function AllocationsCard({
  c,
  onRelease,
}: {
  c: FinanceCollection;
  onRelease?: (a: FinanceCollectionAllocation) => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const rows = useMemo(() => c.allocations ?? [], [c.allocations]);
  const columns = useMemo<
    ColumnDef<FinanceCollectionAllocation, unknown>[]
  >(() => {
    const cols: ColumnDef<FinanceCollectionAllocation, unknown>[] = [
      createColumn<FinanceCollectionAllocation>({
        id: "item",
        accessorFn: (row) => row.description,
        labelKey: "finance.columns.item",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => {
          const a = row.original;
          return (
            <div className="max-w-[18rem] space-y-0.5">
              <div className="flex flex-wrap items-center gap-1">
                <Badge variant="outline">
                  {t(PURPOSE_LABEL_KEYS[a.purpose as EntryPurpose])}
                </Badge>
                {a.charge_kind ? (
                  <span className="text-muted-foreground text-xs">
                    {chargeKindLabel(t, a.charge_kind)}
                  </span>
                ) : null}
              </div>
              {a.rental ? (
                <Link
                  href={routes.tenant.rentals.detail(slug, a.rental.uuid)}
                  className="block truncate text-sm hover:underline"
                  onClick={(e) => e.stopPropagation()}
                >
                  {a.rental.code}
                </Link>
              ) : (
                <div className="truncate text-sm">{a.description}</div>
              )}
            </div>
          );
        },
      }),
      createColumn<FinanceCollectionAllocation>({
        accessorKey: "due_date",
        labelKey: "finance.columns.due_date",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="whitespace-nowrap tabular-nums">
            {date(row.original.due_date, "dd.MM.yyyy", locale)}
          </span>
        ),
      }),
      createColumn<FinanceCollectionAllocation>({
        accessorKey: "item_amount",
        labelKey: "finance.columns.item_amount",
        enableSorting: false,
        cell: ({ row }) => (
          <Amount
            value={row.original.item_amount}
            currency={c.currency}
            muted
          />
        ),
      }),
      createColumn<FinanceCollectionAllocation>({
        accessorKey: "amount",
        labelKey: "finance.columns.paid_by_collection",
        enableSorting: false,
        cell: ({ row }) => (
          <Amount
            value={row.original.amount}
            currency={c.currency}
            className="font-medium"
          />
        ),
      }),
      createColumn<FinanceCollectionAllocation>({
        accessorKey: "item_status",
        labelKey: "finance.columns.item_status",
        enableSorting: false,
        cell: ({ row }) => (
          <StatusChip
            label={t(ITEM_STATUS_LABEL_KEYS[row.original.item_status])}
            tone={ITEM_STATUS_TONES[row.original.item_status]}
          />
        ),
      }),
    ];
    if (onRelease) {
      cols.push(
        createColumn<FinanceCollectionAllocation>({
          id: "actions",
          labelKey: "finance.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "release",
                  label: t("finance.actions.release"),
                  icon: Unlink,
                  permission: permissions.tenant.finance.accounts.write,
                  onSelect: () => onRelease(row.original),
                },
              ]}
            />
          ),
        }),
      );
    }
    return cols;
  }, [c.currency, locale, onRelease, slug, t]);
  return (
    <EntitySectionCard
      title={t("finance.collection.allocations")}
      badge={rows.length}
    >
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.entry_uuid}
        emptyTitle={t("finance.collection.no_allocations")}
        emptyDescription={
          Number(c.unallocated_amount) > 0
            ? t("finance.collection.no_allocations_description")
            : undefined
        }
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-finance-collection-allocations-v1",
          rowSelection: false,
        }}
      />
    </EntitySectionCard>
  );
}

function RefundsCard({ c }: { c: FinanceCollection }) {
  const { t, locale } = useLocale();
  const rows = useMemo(() => c.refunds ?? [], [c.refunds]);
  const columns = useMemo<ColumnDef<FinanceCollectionRefund, unknown>[]>(
    () => [
      createColumn<FinanceCollectionRefund>({
        accessorKey: "created_at",
        labelKey: "finance.columns.date",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap tabular-nums">
            {datetime(row.original.created_at, undefined, locale)}
          </span>
        ),
      }),
      createColumn<FinanceCollectionRefund>({
        accessorKey: "amount",
        labelKey: "finance.columns.amount",
        enableSorting: false,
        cell: ({ row }) => (
          <Amount
            value={row.original.amount}
            currency={row.original.currency}
            className="font-medium"
          />
        ),
      }),
      createColumn<FinanceCollectionRefund>({
        accessorKey: "status",
        labelKey: "finance.columns.status",
        enableSorting: false,
        cell: ({ row }) => (
          <StatusChip
            label={t(
              REFUND_STATUS_LABEL_KEYS[row.original.status] ??
                "finance.refund_status.pending",
            )}
            tone={REFUND_STATUS_TONES[row.original.status] ?? "default"}
          />
        ),
      }),
      createColumn<FinanceCollectionRefund>({
        accessorKey: "reason",
        labelKey: "finance.columns.reason",
        enableSorting: false,
        cell: ({ row }) => (
          <div className="max-w-[20rem] space-y-0.5">
            <div className="truncate">
              {reasonText(t, row.original.reason) || "—"}
            </div>
            {row.original.failure_reason ? (
              <div className="text-destructive truncate text-xs">
                {row.original.failure_reason}
              </div>
            ) : null}
          </div>
        ),
      }),
      createColumn<FinanceCollectionRefund>({
        id: "cash_account",
        accessorFn: (row) => row.cash_account?.code ?? "",
        labelKey: "finance.columns.cash_account",
        enableSorting: false,
        cell: ({ row }) => row.original.cash_account?.name ?? "—",
      }),
    ],
    [locale, t],
  );
  return (
    <EntitySectionCard
      title={t("finance.collection.refunds")}
      badge={rows.length}
    >
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        emptyTitle={t("finance.collection.no_refunds")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-finance-collection-refunds-v1",
          rowSelection: false,
        }}
      />
    </EntitySectionCard>
  );
}
