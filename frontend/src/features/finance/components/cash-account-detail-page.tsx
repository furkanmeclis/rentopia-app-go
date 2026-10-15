"use client";

import { ArrowLeftRight, Pencil, Plus } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  CashAccountDialog,
  MovementDialog,
  ReverseMovementDialog,
  TransferDialog,
} from "@/features/finance/components/cash-dialogs";
import { Amount } from "@/features/finance/components/finance-cells";
import { useMovementColumns } from "@/features/finance/components/movement-columns";
import {
  ACCOUNT_STATUS_LABEL_KEYS,
  CASH_KIND_LABEL_KEYS,
  MOVEMENTS_RESOURCE,
} from "@/features/finance/constants";
import {
  useCashAccount,
  useCategories,
  useFinanceMeta,
  useMovementsList,
} from "@/features/finance/hooks/use-finance";
import type { CashMovement } from "@/features/finance/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Dialog = "edit" | "movement" | "transfer" | null;

/** A till / bank account: balance, its movements (running balance). */
export function CashAccountDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const query = useCashAccount(uuid);
  const a = query.data;
  const [dialog, setDialog] = useState<Dialog>(null);
  const [reverse, setReverse] = useState<CashMovement | null>(null);
  const write = can(permissions.tenant.finance.cash.write);
  const listState = useServerListState({
    initialSort: "-movement_date",
    initialPageSize: 25,
    realtime: tenantLiveList(organization, MOVEMENTS_RESOURCE),
  });
  const kinds = useMemo(() => {
    const v = listState.columnFilters.find((f) => f.id === "kind")?.value;
    return Array.isArray(v) ? v.map(String) : [];
  }, [listState.columnFilters]);
  const params = {
    ...listState.params,
    account: uuid,
    kind: kinds.length ? kinds.join(",") : undefined,
  };
  const movements = useMovementsList(params, Boolean(a));
  const meta = useFinanceMeta("movements");
  const categories = useCategories(false);
  const columns = useMovementColumns({
    showAccount: false,
    showBalance: true,
    categories: categories.data?.items,
    onReverse: write ? setReverse : undefined,
  });
  const total = movements.data?.total ?? 0;
  const onOpen = (d: Dialog) => (open: boolean) => setDialog(open ? d : null);

  return (
    <EntityPage
      title={a?.name ?? t("finance.cash.title")}
      permission={permissions.tenant.finance.cash.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("finance.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("finance.cash.title"),
          href: routes.tenant.finance.cashAccounts(slug),
        },
        { label: a?.code ?? "…" },
      ]}
      actions={
        a && write ? (
          <EntityActions>
            {a.status === "active" ? (
              <>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => setDialog("movement")}
                >
                  <Plus className="size-4" />
                  {t("finance.actions.record_movement")}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setDialog("transfer")}
                >
                  <ArrowLeftRight className="size-4" />
                  {t("finance.actions.transfer")}
                </Button>
              </>
            ) : null}
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setDialog("edit")}
            >
              <Pencil className="size-4" />
              {t("finance.actions.edit")}
            </Button>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("finance.cash.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {a ? (
        <div className="space-y-6">
          <EntityHeader
            title={a.name}
            subtitle={
              <span className="flex flex-wrap items-center gap-2">
                <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
                  {a.code}
                </code>
                <span>{t(CASH_KIND_LABEL_KEYS[a.kind])}</span>
                <span>{a.branch?.name ?? t("finance.head_office")}</span>
                {a.iban ? (
                  <span className="font-mono text-xs">{a.iban}</span>
                ) : null}
              </span>
            }
            badges={
              <StatusChip
                label={t(ACCOUNT_STATUS_LABEL_KEYS[a.status])}
                tone={a.status === "active" ? "success" : "default"}
              />
            }
          >
            <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <div className="space-y-1">
                <dt className="text-muted-foreground text-xs">
                  {t("finance.columns.balance")}
                </dt>
                <dd>
                  <Amount
                    value={a.balance}
                    currency={a.currency}
                    className="text-lg font-semibold"
                  />
                </dd>
              </div>
              <div className="space-y-1">
                <dt className="text-muted-foreground text-xs">
                  {t("finance.columns.opening_balance")}
                </dt>
                <dd>
                  <Amount value={a.opening_balance} currency={a.currency} />
                  <span className="text-muted-foreground ml-1 text-xs">
                    {date(a.opening_date, "dd.MM.yyyy", locale)}
                  </span>
                </dd>
              </div>
              <div className="space-y-1">
                <dt className="text-muted-foreground text-xs">
                  {t("finance.columns.total_in")}
                </dt>
                <dd>
                  <Amount value={a.total_in} currency={a.currency} />
                </dd>
              </div>
              <div className="space-y-1">
                <dt className="text-muted-foreground text-xs">
                  {t("finance.columns.total_out")}
                </dt>
                <dd>
                  <Amount value={a.total_out} currency={a.currency} />
                </dd>
              </div>
            </dl>
          </EntityHeader>
          <EntitySectionCard title={t("finance.movements.title")} badge={total}>
            <EntityTable
              columns={columns}
              data={movements.data?.items ?? []}
              getRowId={(row) => row.uuid}
              isLoading={movements.isLoading}
              isError={movements.isError}
              errorDescription={t("finance.load_failed")}
              onRetry={() => void movements.refetch()}
              realtime={listState.realtimeFor(() => movements.refetch())}
              emptyTitle={t("finance.movements.empty")}
              pageCount={Math.max(
                1,
                Math.ceil(total / (listState.pagination.pageSize || 25)),
              )}
              state={listState.tableState}
              features={{
                persistKey: "tenant-finance-cash-account-movements-v1",
              }}
              toolbarExtra={
                <>
                  <ResourceIOToolbar
                    resource={MOVEMENTS_RESOURCE}
                    query={{ account: uuid, kind: params.kind }}
                    capabilities={
                      meta.data?.capabilities
                        ? { ...meta.data.capabilities, import: false }
                        : undefined
                    }
                  />
                  <EntityToolbar
                    onRefresh={() => void movements.refetch()}
                    refreshDisabled={movements.isFetching}
                  />
                </>
              }
            />
          </EntitySectionCard>
          <CashAccountDialog
            account={a}
            open={dialog === "edit"}
            onOpenChange={onOpen("edit")}
          />
          <MovementDialog
            account={a}
            open={dialog === "movement"}
            onOpenChange={onOpen("movement")}
          />
          <TransferDialog
            from={a}
            open={dialog === "transfer"}
            onOpenChange={onOpen("transfer")}
          />
          <ReverseMovementDialog
            movement={reverse}
            onOpenChange={(open) => {
              if (!open) setReverse(null);
            }}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
