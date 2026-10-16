"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ShieldCheck } from "lucide-react";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  DepositMethodBadge,
  DepositStatusChip,
} from "@/features/deposits/components/deposit-cells";
import { DepositDetailDrawer } from "@/features/deposits/components/deposit-detail-drawer";
import { TakeDepositDialog } from "@/features/deposits/components/take-deposit-dialog";
import { useRentalDeposits } from "@/features/deposits/hooks/use-deposits";
import type { SecurityDeposit } from "@/features/deposits/types";
import { Amount } from "@/features/finance/components/finance-cells";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Deposit card of a rental: "Depozito al" while none is open, the open
 * deposit with its actions, and every deposit of the rental (DataTable).
 */
export function RentalDepositCard({ rentalUuid }: { rentalUuid: string }) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.finance.deposits.read);
  const query = useRentalDeposits(rentalUuid, allowed);
  const [takeOpen, setTakeOpen] = useState(false);
  const [selected, setSelected] = useState<string | null>(null);
  const [resolveNow, setResolveNow] = useState(false);
  const card = query.data;
  const columns = useMemo(
    () =>
      [
        createColumn<SecurityDeposit>({
          accessorKey: "created_at",
          labelKey: "deposits.columns.created_at",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap tabular-nums">
              {datetime(row.original.created_at, "dd.MM.yyyy HH:mm", locale)}
            </span>
          ),
        }),
        createColumn<SecurityDeposit>({
          accessorKey: "method",
          labelKey: "deposits.columns.method",
          enableSorting: false,
          cell: ({ row }) => (
            <DepositMethodBadge method={row.original.method} />
          ),
        }),
        createColumn<SecurityDeposit>({
          accessorKey: "amount",
          labelKey: "deposits.columns.amount",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Amount
              value={row.original.amount}
              currency={row.original.currency}
              className="font-semibold"
            />
          ),
        }),
        createColumn<SecurityDeposit>({
          accessorKey: "captured_amount",
          labelKey: "deposits.columns.captured",
          enableSorting: false,
          cell: ({ row }) => (
            <Amount
              value={row.original.captured_amount}
              currency={row.original.currency}
              muted={Number(row.original.captured_amount) === 0}
            />
          ),
        }),
        createColumn<SecurityDeposit>({
          accessorKey: "status",
          labelKey: "deposits.columns.status",
          enableSorting: false,
          cell: ({ row }) => <DepositStatusChip status={row.original.status} />,
        }),
      ] as ColumnDef<SecurityDeposit, unknown>[],
    [locale],
  );
  if (!allowed) return null;
  const open = card?.items.find((d) => d.uuid === card.open_deposit_uuid);

  return (
    <EntitySectionCard
      title={t("deposits.card.title")}
      badge={card?.items.length}
      actions={
        card?.can_take ? (
          <Button size="sm" onClick={() => setTakeOpen(true)}>
            <ShieldCheck className="size-4" />
            {t("deposits.actions.take")}
          </Button>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("deposits.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {card ? (
        <div className="space-y-4">
          {open ? (
            <div className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <DepositStatusChip status={open.status} />
                  <DepositMethodBadge method={open.method} />
                </div>
                <div className="text-sm">
                  <Amount
                    value={open.open_amount}
                    currency={open.currency}
                    className="font-semibold"
                  />{" "}
                  <span className="text-muted-foreground">
                    {open.status === "pending"
                      ? t("deposits.card.pending_hint")
                      : t("deposits.card.held_hint")}
                  </span>
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setResolveNow(false);
                    setSelected(open.uuid);
                  }}
                >
                  {t("deposits.actions.open")}
                </Button>
                {open.capabilities.release ? (
                  <Button
                    size="sm"
                    onClick={() => {
                      setResolveNow(true);
                      setSelected(open.uuid);
                    }}
                  >
                    {t("deposits.actions.resolve")}
                  </Button>
                ) : null}
              </div>
            </div>
          ) : null}
          {card.deposit_task_uuid && open ? (
            <Alert>
              <AlertDescription>
                {t("deposits.card.task_open")}
              </AlertDescription>
            </Alert>
          ) : null}
          {card.items.length === 0 ? (
            <EmptyState
              className="py-8"
              title={t("deposits.card.empty")}
              description={
                Number(card.default_amount) > 0
                  ? t("deposits.card.empty_description", {
                      amount: Number(card.default_amount).toLocaleString(
                        locale === "tr" ? "tr-TR" : "en-GB",
                        { minimumFractionDigits: 2 },
                      ),
                      currency: card.currency,
                    })
                  : t("deposits.card.empty_no_default")
              }
            />
          ) : (
            <EntityTable
              columns={columns}
              data={card.items}
              getRowId={(row) => row.uuid}
              onRowClick={(row) => {
                setResolveNow(false);
                setSelected(row.uuid);
              }}
              emptyTitle={t("deposits.card.empty")}
              manual={{ filtering: false, sorting: false, pagination: false }}
              features={{
                persistKey: "tenant-rental-deposits-v1",
                rowSelection: false,
              }}
            />
          )}
          <TakeDepositDialog
            rentalUuid={rentalUuid}
            card={card}
            open={takeOpen}
            onOpenChange={setTakeOpen}
          />
        </div>
      ) : null}
      <DepositDetailDrawer
        uuid={selected}
        autoResolve={resolveNow}
        onOpenChange={(next) => {
          if (!next) {
            setSelected(null);
            setResolveNow(false);
          }
        }}
      />
    </EntitySectionCard>
  );
}
