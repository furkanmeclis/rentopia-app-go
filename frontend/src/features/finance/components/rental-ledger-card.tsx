"use client";

import { ArrowDownToLine, ExternalLink } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { CollectDialog } from "@/features/finance/components/collect-dialog";
import {
  LedgerEntriesTable,
  LedgerFigures,
} from "@/features/finance/components/ledger-cards";
import { useRentalLedger } from "@/features/finance/hooks/use-finance";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Rental detail "Cari" tab: what this rental owes and what was paid. */
export function RentalLedgerCard({ rentalUuid }: { rentalUuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.finance.accounts.read);
  const canCollect = can(permissions.tenant.payment.collect);
  const [collect, setCollect] = useState(false);
  const query = useRentalLedger(rentalUuid, allowed);
  const ledger = query.data;
  if (!allowed) return null;

  return (
    <EntitySectionCard
      title={t("finance.rental_card.title")}
      badge={ledger?.entries.length}
      actions={
        <div className="flex flex-wrap gap-2">
          {canCollect && ledger ? (
            <Button type="button" size="sm" onClick={() => setCollect(true)}>
              <ArrowDownToLine className="size-4" />
              {t("finance.actions.collect")}
            </Button>
          ) : null}
          {ledger?.account ? (
            <Button asChild size="sm" variant="outline">
              <Link
                href={routes.tenant.finance.account(slug, ledger.account.uuid)}
              >
                <ExternalLink className="size-4" />
                {t("finance.rental_card.open_account")}
              </Link>
            </Button>
          ) : null}
        </div>
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
      {ledger ? (
        ledger.entries.length === 0 ? (
          <EmptyState
            className="py-10"
            title={t("finance.rental_card.empty")}
            description={t("finance.rental_card.empty_description")}
          />
        ) : (
          <div className="space-y-4">
            {ledger.account ? (
              <p className="text-muted-foreground text-sm">
                {t("finance.rental_card.account", {
                  name: ledger.account.name ?? "",
                  code: ledger.account.code ?? "",
                })}
              </p>
            ) : null}
            <LedgerFigures
              figures={[
                {
                  label: t("finance.columns.debit"),
                  value: ledger.debit,
                  currency: ledger.currency,
                },
                {
                  label: t("finance.columns.credit"),
                  value: ledger.credit,
                  currency: ledger.currency,
                },
                {
                  label: t("finance.columns.balance"),
                  value: ledger.balance,
                  currency: ledger.currency,
                  signed: true,
                },
                {
                  label: t("finance.columns.deposit_held"),
                  value: ledger.deposit_held,
                  currency: ledger.currency,
                },
              ]}
            />
            <LedgerEntriesTable
              entries={ledger.entries}
              persistKey="tenant-rental-ledger-v1"
            />
          </div>
        )
      ) : null}
      {ledger ? (
        <CollectDialog
          open={collect}
          onOpenChange={setCollect}
          accountUuid={ledger.account?.uuid}
          rentalUuid={rentalUuid}
          subtitle={[ledger.reference, ledger.account?.name]
            .filter(Boolean)
            .join(" · ")}
          payerName={ledger.account?.name ?? undefined}
        />
      ) : null}
    </EntitySectionCard>
  );
}
