"use client";

import { FilePlus2 } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { EntitySectionCard, EntityTable } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useInvoiceColumns } from "@/features/invoices/components/invoice-columns";
import { CreateInvoiceDialog } from "@/features/invoices/components/invoice-dialogs";
import { useInvoicesList } from "@/features/invoices/hooks/use-invoices";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const LIMIT = 50;

/**
 * Invoices of a rental or a customer account with "Fatura kes" (a draft
 * of the live accruals no standing invoice covers yet).
 */
export function InvoicesCard({
  account,
  rental,
}: {
  account?: string;
  rental?: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const [open, setOpen] = useState(false);
  const canRead = can(permissions.tenant.invoices.read);
  const query = useInvoicesList(
    { limit: LIMIT, offset: 0, sort: "-issue_date", account, rental },
    canRead,
  );
  const columns = useInvoiceColumns({});
  if (!canRead) return null;
  const total = query.data?.total ?? 0;
  return (
    <EntitySectionCard
      title={t("invoices.card.title")}
      badge={total}
      actions={
        can(permissions.tenant.invoices.write) ? (
          <Button type="button" size="sm" onClick={() => setOpen(true)}>
            <FilePlus2 className="size-4" />
            {t("invoices.actions.create")}
          </Button>
        ) : undefined
      }
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("invoices.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("invoices.card.empty")}
        emptyDescription={t("invoices.card.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-invoices-card-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
          pagination: false,
        }}
      />
      {total > LIMIT ? (
        <p className="mt-3 text-sm">
          <Link
            href={routes.tenant.invoices.root(slug)}
            className="underline-offset-4 hover:underline"
          >
            {t("invoices.card.see_all", { count: total })}
          </Link>
        </p>
      ) : null}
      <CreateInvoiceDialog
        open={open}
        onClose={() => setOpen(false)}
        account={account}
        rental={rental}
      />
    </EntitySectionCard>
  );
}
