"use client";

import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityCreateButton, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { SupplierInvoicesTable } from "@/features/suppliers/components/invoices-table";
import { PayablesCard } from "@/features/suppliers/components/payables-card";
import { SupplierPaymentsTable } from "@/features/suppliers/components/payments-table";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const DUE_STATES = ["overdue", "due_soon"];

/** "Gider faturaları": every supplier invoice. */
export function SupplierInvoicesPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [createOpen, setCreateOpen] = useState(false);
  return (
    <EntityPage
      title={t("suppliers.invoices.title")}
      description={t("suppliers.invoices.description")}
      permission={permissions.tenant.finance.expenses.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("suppliers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("suppliers.invoices.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setCreateOpen(true)}
          label={t("suppliers.invoices.create")}
          permission={permissions.tenant.finance.expenses.write}
        />
      }
    >
      <SupplierInvoicesTable
        persistKey="tenant-supplier-invoices-v1"
        createOpen={createOpen}
        onCreateClose={() => setCreateOpen(false)}
      />
    </EntityPage>
  );
}

/** "Tedarikçi ödemeleri": every supplier payment. */
export function SupplierPaymentsPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [createOpen, setCreateOpen] = useState(false);
  return (
    <EntityPage
      title={t("suppliers.payments.title")}
      description={t("suppliers.payments.description")}
      permission={permissions.tenant.finance.cash.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("suppliers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("suppliers.payments.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => setCreateOpen(true)}
          label={t("suppliers.payments.create")}
          permission={permissions.tenant.finance.cash.write}
        />
      }
    >
      <SupplierPaymentsTable
        persistKey="tenant-supplier-payments-v1"
        createOpen={createOpen}
        onCreateClose={() => setCreateOpen(false)}
      />
    </EntityPage>
  );
}

/**
 * "Vadesi yaklaşan ödemeler": the due figures and the overdue / due-soon
 * supplier invoices (oldest due date first), each payable from its row.
 */
export function PayablesPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <EntityPage
      title={t("suppliers.payables.title")}
      description={t("suppliers.payables.description")}
      permission={permissions.tenant.finance.expenses.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("suppliers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("suppliers.payables.title") },
      ]}
    >
      <PayablesCard showLink={false} />
      <SupplierInvoicesTable
        persistKey="tenant-payables-v1"
        initialDueStates={DUE_STATES}
        initialSort="due_date"
        emptyTitle={t("suppliers.payables.empty")}
      />
    </EntityPage>
  );
}
