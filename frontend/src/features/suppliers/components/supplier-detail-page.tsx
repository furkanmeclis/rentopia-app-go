"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Archive,
  ArchiveRestore,
  Banknote,
  FilePlus2,
  Pencil,
  Truck,
} from "lucide-react";
import { usePathname, useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { Timeline } from "@/components/common/timeline";
import {
  EntityActions,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { StatementTable } from "@/features/finance/components/account-detail-page";
import { Amount } from "@/features/finance/components/finance-cells";
import { useAccount } from "@/features/finance/hooks/use-finance";
import { SupplierInvoicesTable } from "@/features/suppliers/components/invoices-table";
import { SupplierPaymentsTable } from "@/features/suppliers/components/payments-table";
import { SupplierDueChip } from "@/features/suppliers/components/supplier-cells";
import { SupplierDialog } from "@/features/suppliers/components/supplier-dialog";
import {
  SUPPLIER_STATUS_LABEL_KEYS,
  SUPPLIER_TYPE_LABEL_KEYS,
} from "@/features/suppliers/constants";
import {
  useSaveSupplier,
  useSupplier,
  useSupplierActivity,
} from "@/features/suppliers/hooks/use-suppliers";
import type { Supplier } from "@/features/suppliers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import {
  PlateBadge,
  VehicleStatusChip,
} from "@/features/vehicles/components/vehicle-cells";
import { useVehiclesList } from "@/features/vehicles/hooks/use-vehicles";
import type { Vehicle } from "@/features/vehicles/types";
import { date, datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const TABS = [
  "overview",
  "invoices",
  "payments",
  "statement",
  "vehicles",
  "activity",
] as const;

type Tab = (typeof TABS)[number];

const TAB_LABEL_KEYS: Record<Tab, string> = {
  overview: "suppliers.tabs.overview",
  invoices: "suppliers.tabs.invoices",
  payments: "suppliers.tabs.payments",
  statement: "suppliers.tabs.statement",
  vehicles: "suppliers.tabs.vehicles",
  activity: "suppliers.tabs.activity",
};

function resolveTab(value: string | undefined): Tab {
  return value && (TABS as readonly string[]).includes(value)
    ? (value as Tab)
    : "overview";
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="min-w-0 space-y-1">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{children || "—"}</dd>
    </div>
  );
}

function OverviewTab({ supplier }: { supplier: Supplier }) {
  const { t } = useLocale();
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <EntitySectionCard title={t("suppliers.sections.identity")}>
        <dl className="grid gap-4 sm:grid-cols-2">
          <Field label={t("suppliers.fields.title")}>{supplier.title}</Field>
          <Field label={t("suppliers.fields.supplier_type")}>
            {t(SUPPLIER_TYPE_LABEL_KEYS[supplier.supplier_type])}
          </Field>
          <Field
            label={
              supplier.tax_kind === "tckn"
                ? t("suppliers.fields.tckn")
                : t("suppliers.fields.tax_number")
            }
          >
            {supplier.tax_number ? (
              <span className="font-mono">{supplier.tax_number}</span>
            ) : null}
          </Field>
          <Field label={t("suppliers.fields.tax_office")}>
            {supplier.tax_office}
          </Field>
          <Field label={t("suppliers.fields.phone")}>{supplier.phone}</Field>
          <Field label={t("suppliers.fields.email")}>{supplier.email}</Field>
          <Field label={t("suppliers.fields.website")}>
            {supplier.website}
          </Field>
          <Field label={t("suppliers.fields.city")}>{supplier.city}</Field>
          <div className="sm:col-span-2">
            <Field label={t("suppliers.fields.address")}>
              {supplier.address}
            </Field>
          </div>
        </dl>
      </EntitySectionCard>
      <EntitySectionCard title={t("suppliers.sections.billing")}>
        <dl className="grid gap-4 sm:grid-cols-2">
          <Field label={t("suppliers.fields.payment_term_days")}>
            {t("suppliers.term_days", { count: supplier.payment_term_days })}
          </Field>
          <Field label={t("suppliers.fields.e_invoice")}>
            {supplier.e_invoice ? t("suppliers.yes") : t("suppliers.no")}
          </Field>
          <Field label={t("suppliers.fields.bank_name")}>
            {supplier.bank_name}
          </Field>
          <Field label={t("suppliers.fields.iban")}>
            {supplier.iban ? (
              <span className="font-mono text-xs">{supplier.iban}</span>
            ) : null}
          </Field>
          <Field label={t("suppliers.fields.billing_email")}>
            {supplier.billing_email}
          </Field>
          <div className="sm:col-span-2">
            <Field label={t("suppliers.fields.billing_address")}>
              {supplier.billing_address}
            </Field>
          </div>
        </dl>
      </EntitySectionCard>
      <EntitySectionCard
        title={t("suppliers.fields.contacts")}
        badge={supplier.contacts.length}
      >
        {supplier.contacts.length === 0 ? (
          <EmptyState
            className="py-6"
            title={t("suppliers.fields.contacts_empty")}
          />
        ) : (
          <ul className="divide-y">
            {supplier.contacts.map((c, i) => (
              <li key={`${c.name}-${i}`} className="space-y-0.5 py-2">
                <div className="text-sm font-medium">
                  {c.name}
                  {c.role ? (
                    <span className="text-muted-foreground font-normal">
                      {" "}
                      · {c.role}
                    </span>
                  ) : null}
                </div>
                <div className="text-muted-foreground text-xs">
                  {[c.phone, c.email].filter(Boolean).join(" · ") || "—"}
                </div>
              </li>
            ))}
          </ul>
        )}
      </EntitySectionCard>
      <EntitySectionCard title={t("suppliers.fields.note")}>
        <p className="text-muted-foreground text-sm whitespace-pre-wrap">
          {supplier.note || t("suppliers.no_note")}
        </p>
      </EntitySectionCard>
    </div>
  );
}

function StatementTab({ supplier }: { supplier: Supplier }) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const { can } = usePermission();
  const account = useAccount(supplier.account?.uuid ?? "");
  if (!can(permissions.tenant.finance.accounts.read)) {
    return (
      <EntitySectionCard title={t("suppliers.tabs.statement")}>
        <EmptyState
          className="py-8"
          title={t("suppliers.statement.forbidden")}
        />
      </EntitySectionCard>
    );
  }
  if (!supplier.account) {
    return (
      <EntitySectionCard title={t("suppliers.tabs.statement")}>
        <EmptyState className="py-8" title={t("suppliers.statement.empty")} />
      </EntitySectionCard>
    );
  }
  if (account.isLoading || !account.data) {
    return <Loading label={t("common.loading")} />;
  }
  return <StatementTable account={account.data} liveOrg={organization} />;
}

function VehiclesTab({ supplier }: { supplier: Supplier }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.vehicles.read);
  const query = useVehiclesList(
    { limit: 100, offset: 0, sort: "plate", supplier: supplier.uuid },
    allowed,
  );
  const columns = useMemo(
    () =>
      [
        createColumn<Vehicle>({
          accessorKey: "plate",
          labelKey: "vehicles.columns.plate",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => <PlateBadge plate={row.original.plate} />,
        }),
        createColumn<Vehicle>({
          accessorKey: "label",
          labelKey: "vehicles.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="max-w-[16rem] truncate">
              {row.original.label} · {row.original.model_year}
            </span>
          ),
        }),
        createColumn<Vehicle>({
          id: "home_branch",
          accessorFn: (row) => row.home_branch.code,
          labelKey: "vehicles.columns.home_branch",
          enableSorting: false,
          cell: ({ row }) => row.original.home_branch.name,
        }),
        createColumn<Vehicle>({
          accessorKey: "status",
          labelKey: "vehicles.columns.status",
          enableSorting: false,
          cell: ({ row }) => <VehicleStatusChip status={row.original.status} />,
        }),
      ] as ColumnDef<Vehicle, unknown>[],
    [],
  );
  if (!allowed) {
    return (
      <EntitySectionCard title={t("suppliers.tabs.vehicles")}>
        <EmptyState
          className="py-8"
          title={t("suppliers.vehicles.forbidden")}
        />
      </EntitySectionCard>
    );
  }
  return (
    <EntitySectionCard
      title={t("suppliers.tabs.vehicles")}
      badge={query.data?.total}
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.fleet.vehicles.detail(slug, row.uuid))
        }
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("suppliers.load_failed")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("suppliers.vehicles.empty")}
        pageCount={1}
        features={{
          persistKey: "tenant-supplier-vehicles-v1",
          sorting: false,
          pagination: false,
        }}
      />
    </EntitySectionCard>
  );
}

function ActivityTab({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(20);
  const query = useSupplierActivity(uuid, { limit, offset: 0 });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;
  const describe = (action: string) => {
    const key = `activity.actions.${action}`;
    const label = t(key);
    return label === key ? action : label;
  };
  return (
    <EntitySectionCard
      title={t("suppliers.tabs.activity")}
      badge={query.data ? total : undefined}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.data && items.length === 0 ? (
        <EmptyState className="py-8" title={t("suppliers.activity_empty")} />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry.action),
              description: entry.actor
                ? `${entry.actor.name} ${entry.actor.surname}`.trim() ||
                  entry.actor.email
                : t("suppliers.system"),
              meta: (
                <time
                  dateTime={entry.created_at}
                  title={datetime(entry.created_at, undefined, locale)}
                >
                  {relativeDatetime(entry.created_at, locale)}
                </time>
              ),
            }))}
          />
          {items.length < total ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={query.isFetching}
              onClick={() => setLimit((l) => l + 20)}
            >
              {t("suppliers.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/**
 * Supplier detail: what we owe and when (header figures), then tabs —
 * overview, invoices, payments, account statement, vehicles, activity.
 */
export function SupplierDetailPage({
  uuid,
  initialTab,
}: {
  uuid: string;
  initialTab?: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const query = useSupplier(uuid);
  const save = useSaveSupplier();
  const [tab, setTab] = useState<Tab>(() => resolveTab(initialTab));
  const [editOpen, setEditOpen] = useState(false);
  const [invoiceOpen, setInvoiceOpen] = useState(false);
  const [paymentOpen, setPaymentOpen] = useState(false);
  const s = query.data;
  const openTab = (next: string) => {
    const resolved = resolveTab(next);
    setTab(resolved);
    router.replace(`${pathname}?tab=${resolved}`, { scroll: false });
  };
  const canWrite = can(permissions.tenant.suppliers.write);
  const canInvoice = can(permissions.tenant.finance.expenses.write);
  const canPay = can(permissions.tenant.finance.cash.write);

  return (
    <EntityPage
      title={s?.title ?? t("suppliers.detail_title")}
      description={s?.code}
      permission={permissions.tenant.suppliers.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("suppliers.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("suppliers.title"),
          href: routes.tenant.finance.suppliers(slug),
        },
        { label: s?.code ?? "…" },
      ]}
      actions={
        s ? (
          <EntityActions>
            {canInvoice && s.status === "active" ? (
              <Button
                type="button"
                size="sm"
                onClick={() => {
                  openTab("invoices");
                  setInvoiceOpen(true);
                }}
              >
                <FilePlus2 className="size-4" />
                {t("suppliers.actions.add_invoice")}
              </Button>
            ) : null}
            {canPay ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  openTab("payments");
                  setPaymentOpen(true);
                }}
              >
                <Banknote className="size-4" />
                {t("suppliers.actions.pay")}
              </Button>
            ) : null}
            {canWrite ? (
              <>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setEditOpen(true)}
                >
                  <Pencil className="size-4" />
                  {t("suppliers.actions.edit")}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  disabled={save.isPending}
                  onClick={() =>
                    save.mutate({
                      uuid: s.uuid,
                      body: {
                        status: s.status === "active" ? "archived" : "active",
                      },
                    })
                  }
                >
                  {s.status === "active" ? (
                    <Archive className="size-4" />
                  ) : (
                    <ArchiveRestore className="size-4" />
                  )}
                  {s.status === "active"
                    ? t("suppliers.actions.archive")
                    : t("suppliers.actions.activate")}
                </Button>
              </>
            ) : null}
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("suppliers.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {s ? (
        <div className="space-y-6">
          <EntityHeader
            leading={
              <div className="bg-muted/40 flex size-14 shrink-0 items-center justify-center rounded-lg border">
                <Truck className="text-muted-foreground size-6" />
              </div>
            }
            title={s.title}
            subtitle={
              <span className="flex flex-wrap items-center gap-2">
                <code className="bg-muted rounded px-1.5 py-0.5 text-xs font-semibold">
                  {s.code}
                </code>
                {s.tax_number ? (
                  <span className="font-mono text-xs">{s.tax_number}</span>
                ) : null}
                <span>
                  {t("suppliers.term_days", { count: s.payment_term_days })}
                </span>
              </span>
            }
            badges={
              <>
                <Badge variant="outline">
                  {t(SUPPLIER_TYPE_LABEL_KEYS[s.supplier_type])}
                </Badge>
                <SupplierDueChip state={s.due_state} />
                {s.status === "archived" ? (
                  <StatusChip
                    label={t(SUPPLIER_STATUS_LABEL_KEYS.archived)}
                    tone="default"
                  />
                ) : null}
              </>
            }
          >
            <dl className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-5">
              <Field label={t("suppliers.columns.payable")}>
                <Amount value={s.payable} className="text-lg font-semibold" />
              </Field>
              <Field label={t("suppliers.columns.overdue_amount")}>
                <Amount
                  value={s.overdue_amount}
                  className={
                    Number(s.overdue_amount) > 0
                      ? "text-destructive"
                      : undefined
                  }
                />
              </Field>
              <Field label={t("suppliers.columns.due_soon_amount")}>
                <Amount value={s.due_soon_amount} />
              </Field>
              <Field label={t("suppliers.columns.next_due_date")}>
                {s.next_due_date
                  ? date(s.next_due_date, "dd.MM.yyyy", locale)
                  : null}
              </Field>
              <Field label={t("suppliers.columns.open_invoices")}>
                {String(s.open_invoices)}
              </Field>
            </dl>
          </EntityHeader>

          <Tabs value={tab} onValueChange={openTab} className="space-y-4">
            <div className="-mx-1 overflow-x-auto px-1 pb-1">
              <TabsList className="h-auto w-max">
                {TABS.map((value) => (
                  <TabsTrigger key={value} value={value}>
                    {t(TAB_LABEL_KEYS[value])}
                  </TabsTrigger>
                ))}
              </TabsList>
            </div>
            <TabsContent value="overview">
              <OverviewTab supplier={s} />
            </TabsContent>
            <TabsContent value="invoices">
              {can(permissions.tenant.finance.expenses.read) ? (
                <SupplierInvoicesTable
                  supplierUuid={s.uuid}
                  persistKey="tenant-supplier-detail-invoices-v1"
                  initialSort="-due_date"
                  createOpen={invoiceOpen}
                  onCreateClose={() => setInvoiceOpen(false)}
                />
              ) : (
                <EmptyState title={t("suppliers.invoices.forbidden")} />
              )}
            </TabsContent>
            <TabsContent value="payments">
              {can(permissions.tenant.finance.cash.read) ? (
                <SupplierPaymentsTable
                  supplierUuid={s.uuid}
                  persistKey="tenant-supplier-detail-payments-v1"
                  createOpen={paymentOpen}
                  onCreateClose={() => setPaymentOpen(false)}
                />
              ) : (
                <EmptyState title={t("suppliers.payments.forbidden")} />
              )}
            </TabsContent>
            <TabsContent value="statement">
              <StatementTab supplier={s} />
            </TabsContent>
            <TabsContent value="vehicles">
              <VehiclesTab supplier={s} />
            </TabsContent>
            <TabsContent value="activity">
              <ActivityTab uuid={s.uuid} />
            </TabsContent>
          </Tabs>
          <SupplierDialog
            supplier={s}
            open={editOpen}
            onOpenChange={setEditOpen}
          />
        </div>
      ) : null}
    </EntityPage>
  );
}
