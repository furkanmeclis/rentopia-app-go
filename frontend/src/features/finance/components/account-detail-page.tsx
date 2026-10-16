"use client";

import {
  ArrowDownToLine,
  ArrowUpFromLine,
  Handshake,
  MoreHorizontal,
  Pencil,
  Scale,
  Send,
} from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityActions,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { AppInput, AppTextarea } from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  AccountCollectionsCard,
  OpenItemsCard,
} from "@/features/finance/components/account-collections";
import { CollectDialog } from "@/features/finance/components/collect-dialog";
import { useEntryColumns } from "@/features/finance/components/entry-columns";
import {
  Amount,
  BalanceStatusChip,
} from "@/features/finance/components/finance-cells";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  AdjustmentDialog,
  OffsetDialog,
  RefundDialog,
  ReverseEntryDialog,
  SendStatementDialog,
} from "@/features/finance/components/ledger-dialogs";
import { ENTRIES_RESOURCE } from "@/features/finance/constants";
import {
  useAccount,
  useFinanceMeta,
  usePatchAccount,
  useStatement,
} from "@/features/finance/hooks/use-finance";
import { errorText } from "@/features/finance/lib/format";
import type { FinanceAccount, LedgerEntry } from "@/features/finance/types";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Dialog =
  "collect" | "refund" | "adjust" | "offset" | "statement" | "terms" | null;

/**
 * Account detail: balance figures, the statement (entries DataTable with the
 * running balance, PDF / XLSX export through the io engine) and the
 * account actions (collection, refund, adjustment, deposit offset,
 * statement + pay link by SMS).
 */
export function AccountDetailPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const { can } = usePermission();
  const query = useAccount(uuid);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [reverse, setReverse] = useState<LedgerEntry | null>(null);
  const a = query.data;

  const write = can(permissions.tenant.finance.accounts.write);
  const canCollect = can(permissions.tenant.payment.collect);
  const canRefund = write && can(permissions.tenant.payment.refund);
  const canAdjust = write && can(permissions.tenant.finance.ledger.adjust);
  const onOpen = (d: Dialog) => (open: boolean) => setDialog(open ? d : null);

  return (
    <EntityPage
      title={a?.name || t("finance.accounts.title")}
      description={a ? `${a.code}` : undefined}
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
          label: t("finance.accounts.title"),
          href: routes.tenant.finance.accounts(slug),
        },
        { label: a?.code ?? "…" },
      ]}
      actions={
        a && (write || canCollect) ? (
          <EntityActions>
            {canCollect && a.party_kind === "customer" ? (
              <Button
                type="button"
                size="sm"
                onClick={() => setDialog("collect")}
              >
                <ArrowDownToLine className="size-4" />
                {t("finance.actions.collect")}
              </Button>
            ) : null}
            {write && a.party_kind === "customer" ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setDialog("statement")}
              >
                <Send className="size-4" />
                {t("finance.actions.send_statement")}
              </Button>
            ) : null}
            {write ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    aria-label={t("finance.actions.more")}
                  >
                    <MoreHorizontal className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  {canRefund ? (
                    <DropdownMenuItem onSelect={() => setDialog("refund")}>
                      <ArrowUpFromLine className="size-4" />
                      {t("finance.actions.refund")}
                    </DropdownMenuItem>
                  ) : null}
                  {canAdjust ? (
                    <DropdownMenuItem onSelect={() => setDialog("adjust")}>
                      <Scale className="size-4" />
                      {t("finance.actions.adjust")}
                    </DropdownMenuItem>
                  ) : null}
                  {canAdjust && Number(a.balance.deposit_held) > 0 ? (
                    <DropdownMenuItem onSelect={() => setDialog("offset")}>
                      <Handshake className="size-4" />
                      {t("finance.actions.offset")}
                    </DropdownMenuItem>
                  ) : null}
                  <DropdownMenuItem onSelect={() => setDialog("terms")}>
                    <Pencil className="size-4" />
                    {t("finance.actions.edit_terms")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("finance.accounts.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {a ? (
        <div className="space-y-6">
          <EntityHeader
            title={a.name || t("finance.accounts.anonymous")}
            subtitle={
              <span className="flex flex-wrap items-center gap-2">
                <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
                  {a.code}
                </code>
                {a.branch ? <span>{a.branch.name}</span> : null}
                <span>
                  {t("finance.accounts.term_days", {
                    count: a.payment_term_days,
                  })}
                </span>
              </span>
            }
            badges={
              <>
                <BalanceStatusChip status={a.balance.status} />
                {a.status === "archived" ? (
                  <Badge variant="secondary">
                    {t("finance.account_status.archived")}
                  </Badge>
                ) : null}
              </>
            }
          >
            <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
              <Figure label={t("finance.columns.balance")}>
                <Amount
                  value={a.balance.balance}
                  currency={a.currency}
                  signed
                  className="text-lg font-semibold"
                />
              </Figure>
              <Figure label={t("finance.columns.debit")}>
                <Amount value={a.balance.debit} currency={a.currency} />
              </Figure>
              <Figure label={t("finance.columns.credit")}>
                <Amount value={a.balance.credit} currency={a.currency} />
              </Figure>
              <Figure label={t("finance.columns.overdue")}>
                <Amount
                  value={a.balance.overdue}
                  currency={a.currency}
                  className={
                    Number(a.balance.overdue) > 0
                      ? "text-destructive"
                      : undefined
                  }
                />
              </Figure>
              <Figure label={t("finance.columns.due_soon")}>
                <Amount value={a.balance.due_soon} currency={a.currency} />
              </Figure>
              <Figure label={t("finance.columns.deposit_held")}>
                <Amount value={a.balance.deposit_held} currency={a.currency} />
              </Figure>
            </dl>
            {a.note ? (
              <p className="text-muted-foreground text-sm">{a.note}</p>
            ) : null}
          </EntityHeader>

          {a.party_kind === "customer" ? (
            <>
              <OpenItemsCard accountUuid={a.uuid} />
              <AccountCollectionsCard accountUuid={a.uuid} />
            </>
          ) : null}

          <StatementTable
            account={a}
            onReverse={canAdjust ? setReverse : undefined}
            liveOrg={organization}
          />

          <CollectDialog
            accountUuid={a.uuid}
            subtitle={`${a.code} · ${a.name}`}
            payerName={a.name}
            open={dialog === "collect"}
            onOpenChange={onOpen("collect")}
          />
          <RefundDialog
            account={a}
            open={dialog === "refund"}
            onOpenChange={onOpen("refund")}
          />
          <AdjustmentDialog
            account={a}
            open={dialog === "adjust"}
            onOpenChange={onOpen("adjust")}
          />
          <OffsetDialog
            account={a}
            open={dialog === "offset"}
            onOpenChange={onOpen("offset")}
          />
          <SendStatementDialog
            account={a}
            open={dialog === "statement"}
            onOpenChange={onOpen("statement")}
            canPayLink={can(permissions.tenant.payments.createLink)}
          />
          <TermsDialog
            account={a}
            open={dialog === "terms"}
            onOpenChange={onOpen("terms")}
          />
          <ReverseEntryDialog
            entry={reverse}
            onOpenChange={(open) => {
              if (!open) setReverse(null);
            }}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}

function Figure({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="truncate">{children}</dd>
    </div>
  );
}

/** The statement of one account (also the supplier detail's tab). */
export function StatementTable({
  account,
  onReverse,
  liveOrg,
}: {
  account: FinanceAccount;
  onReverse?: (entry: LedgerEntry) => void;
  liveOrg: { uuid: string } | null | undefined;
}) {
  const { t } = useLocale();
  const listState = useServerListState({
    initialSort: "-entry_date",
    initialPageSize: 30,
    realtime: tenantLiveList(liveOrg, ENTRIES_RESOURCE),
  });
  const kinds = useMemo(() => {
    const v = listState.columnFilters.find((f) => f.id === "kind")?.value;
    return Array.isArray(v) ? v.map(String) : [];
  }, [listState.columnFilters]);
  const params = {
    ...listState.params,
    sort: "-entry_date",
    kind: kinds.length ? kinds.join(",") : undefined,
  };
  const query = useStatement(account.uuid, params);
  const meta = useFinanceMeta("entries");
  const columns = useEntryColumns({ onReverse, kindFilter: true });
  const total = query.data?.total ?? 0;
  return (
    <EntitySectionCard title={t("finance.statement.title")} badge={total}>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("finance.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("finance.statement.empty")}
        pageCount={Math.max(
          1,
          Math.ceil(total / (listState.pagination.pageSize || 30)),
        )}
        state={listState.tableState}
        features={{ persistKey: "tenant-finance-statement-v1", sorting: false }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={ENTRIES_RESOURCE}
              query={{ account: account.uuid, kind: params.kind }}
              capabilities={meta.data?.capabilities}
            />
            <EntityToolbar
              onRefresh={() => void query.refetch()}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
    </EntitySectionCard>
  );
}

function TermsDialog({
  account,
  open,
  onOpenChange,
}: {
  account: FinanceAccount;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const patch = usePatchAccount(account.uuid);
  return (
    <FinanceFormDialog<{ payment_term_days: string; note: string }>
      open={open}
      onOpenChange={(next) => {
        if (!next) setProblem(null);
        onOpenChange(next);
      }}
      formId="finance-terms-form"
      title={t("finance.terms.title")}
      description={t("finance.terms.description")}
      submitLabel={t("form.save")}
      pending={patch.isPending}
      problem={problem}
      schema={z.object({
        payment_term_days: z
          .string()
          .trim()
          .regex(/^\d{1,3}$/, t("finance.validation.term"))
          .refine((v) => Number(v) <= 365, t("finance.validation.term")),
        note: z.string().max(2000),
      })}
      defaultValues={{
        payment_term_days: String(account.payment_term_days),
        note: account.note,
      }}
      onSubmit={async (v) => {
        try {
          await patch.mutateAsync({
            payment_term_days: Number(v.payment_term_days),
            note: v.note,
          });
          onOpenChange(false);
        } catch (error) {
          setProblem(errorText(error, t("finance.errors.save_failed")));
        }
      }}
    >
      <AppInput
        name="payment_term_days"
        label={t("finance.fields.payment_term_days")}
        inputMode="numeric"
      />
      <AppTextarea name="note" label={t("finance.fields.note")} />
    </FinanceFormDialog>
  );
}
