"use client";

import { ExternalLink } from "lucide-react";
import Link from "next/link";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { BalanceStatusChip } from "@/features/finance/components/finance-cells";
import {
  LedgerEntriesTable,
  LedgerFigures,
} from "@/features/finance/components/ledger-cards";
import { useCustomerLedger } from "@/features/finance/hooks/use-finance";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Customer detail "Cari hesap" tab: balance and the latest entries. */
export function CustomerLedgerCard({ customerUuid }: { customerUuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.finance.accounts.read);
  const query = useCustomerLedger(customerUuid, allowed);
  const ledger = query.data;
  const account = ledger?.account ?? null;

  if (!allowed) {
    return (
      <EntitySectionCard title={t("finance.customer_card.title")}>
        <EmptyState className="py-10" title={t("finance.forbidden")} />
      </EntitySectionCard>
    );
  }

  return (
    <EntitySectionCard
      title={t("finance.customer_card.title")}
      actions={
        account ? (
          <Button asChild size="sm" variant="outline">
            <Link href={routes.tenant.finance.account(slug, account.uuid)}>
              <ExternalLink className="size-4" />
              {t("finance.customer_card.open_statement")}
            </Link>
          </Button>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("finance.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {ledger && !account ? (
        <EmptyState
          className="py-10"
          title={t("finance.customer_card.no_account")}
          description={t("finance.customer_card.no_account_description")}
        />
      ) : null}
      {ledger && account ? (
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
              {account.code}
            </code>
            <BalanceStatusChip status={account.balance.status} />
            <span className="text-muted-foreground">
              {t("finance.customer_card.terms", {
                days: account.payment_term_days,
              })}
            </span>
          </div>
          <LedgerFigures
            figures={[
              {
                label: t("finance.columns.balance"),
                value: account.balance.balance,
                currency: account.balance.currency,
                signed: true,
              },
              {
                label: t("finance.columns.overdue"),
                value: account.balance.overdue,
                currency: account.balance.currency,
              },
              {
                label: t("finance.columns.due_soon"),
                value: account.balance.due_soon,
                currency: account.balance.currency,
              },
              {
                label: t("finance.columns.deposit_held"),
                value: account.balance.deposit_held,
                currency: account.balance.currency,
              },
            ]}
          />
          <LedgerEntriesTable
            entries={ledger.entries}
            persistKey="tenant-customer-ledger-v1"
          />
        </div>
      ) : null}
    </EntitySectionCard>
  );
}
