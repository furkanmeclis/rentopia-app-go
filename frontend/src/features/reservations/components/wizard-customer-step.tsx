"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { UserPlus } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import { EntityDrawer, EntityTable } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { CustomerTypeChip } from "@/features/customers/components/customer-cells";
import { CustomerForm } from "@/features/customers/components/customer-form";
import {
  useCreateCustomer,
  useCustomer,
  useCustomerDrivers,
} from "@/features/customers/hooks/use-customers";
import {
  customerFormDefaults,
  customerFormToRequest,
} from "@/features/customers/schemas/customer-form";
import { customersService } from "@/features/customers/services/customers.service";
import type { CustomerDriver } from "@/features/customers/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

export type WizardDrivers = {
  /** Selected driver uuids (row selection). */
  selected: RowSelectionState;
  /** Primary driver uuid (one of the selected). */
  primary: string;
};

/** Drivers of the customer: pick who drives and who is the primary driver. */
export function CustomerDriversPicker({
  customerUuid,
  value,
  onChange,
}: {
  customerUuid: string;
  value: WizardDrivers;
  onChange: (next: WizardDrivers) => void;
}) {
  const { t } = useLocale();
  const query = useCustomerDrivers(customerUuid);
  const drivers = useMemo(() => query.data?.items ?? [], [query.data]);
  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<CustomerDriver>(),
        createColumn<CustomerDriver>({
          accessorKey: "full_name",
          labelKey: "reservations.drivers.columns.name",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-1.5 font-medium">
              {row.original.full_name}
              {row.original.is_customer_self ? (
                <Badge variant="outline" className="px-1 py-0 text-[10px]">
                  {t("reservations.drivers.self")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<CustomerDriver>({
          accessorKey: "licence_class",
          labelKey: "reservations.drivers.columns.licence",
          enableSorting: false,
          cell: ({ row }) =>
            `${row.original.licence_class || "—"} · ${row.original.licence_masked || "—"}`,
        }),
        createColumn<CustomerDriver>({
          id: "age",
          accessorFn: (row) => row.age ?? 0,
          labelKey: "reservations.drivers.columns.age",
          enableSorting: false,
          cell: ({ row }) => row.original.age ?? "—",
        }),
        createColumn<CustomerDriver>({
          id: "licence_years",
          accessorFn: (row) => row.licence_years ?? 0,
          labelKey: "reservations.drivers.columns.licence_years",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.licence_expired ? (
              <StatusChip
                label={t("reservations.drivers.licence_expired")}
                tone="danger"
              />
            ) : (
              (row.original.licence_years ?? "—")
            ),
        }),
      ] as ColumnDef<CustomerDriver, unknown>[],
    [t],
  );
  const selectedIds = Object.keys(value.selected).filter(
    (id) => value.selected[id],
  );
  const selectedDrivers = drivers.filter((d) => selectedIds.includes(d.uuid));

  return (
    <div className="space-y-3">
      <EntityTable
        columns={columns}
        data={drivers}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("reservations.drivers.empty")}
        emptyDescription={t("reservations.drivers.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        state={{
          rowSelection: value.selected,
          onRowSelectionChange: (updater) => {
            const next =
              typeof updater === "function" ? updater(value.selected) : updater;
            const ids = Object.keys(next).filter((id) => next[id]);
            onChange({
              selected: next,
              primary: ids.includes(value.primary)
                ? value.primary
                : (ids[0] ?? ""),
            });
          },
        }}
        features={{
          persistKey: "tenant-reservation-wizard-drivers-v1",
          rowSelection: true,
          globalFilter: false,
          sorting: false,
        }}
      />
      {selectedDrivers.length > 1 ? (
        <div className="grid max-w-sm gap-1.5">
          <Label htmlFor="primary-driver">
            {t("reservations.drivers.primary")}
          </Label>
          <Select
            value={value.primary}
            onValueChange={(primary) => onChange({ ...value, primary })}
          >
            <SelectTrigger id="primary-driver">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {selectedDrivers.map((d) => (
                <SelectItem key={d.uuid} value={d.uuid}>
                  {d.full_name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}
    </div>
  );
}

/**
 * Customer of the reservation: search by name / phone / identity (masked
 * results), or create one in a drawer without leaving the wizard; then the
 * drivers who will drive.
 */
export function WizardCustomerStep({
  customerUuid,
  onCustomerChange,
  drivers,
  onDriversChange,
}: {
  customerUuid: string;
  onCustomerChange: (uuid: string) => void;
  drivers: WizardDrivers;
  onDriversChange: (next: WizardDrivers) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [createOpen, setCreateOpen] = useState(false);
  const create = useCreateCustomer();
  const customer = useCustomer(customerUuid || null);
  const loadOptions = useCallback(
    async (query: string): Promise<ComboboxOption[]> => {
      try {
        const page = await customersService.list(slug, {
          limit: 20,
          offset: 0,
          q: query || undefined,
          anonymized: "false",
        });
        return page.items.map((c) => ({
          value: c.uuid,
          label: c.display_name,
          description: [c.phone_masked, c.identity_masked]
            .filter(Boolean)
            .join(" · "),
        }));
      } catch {
        return [];
      }
    },
    [slug],
  );
  const initialOptions = useMemo<ComboboxOption[]>(
    () =>
      customer.data
        ? [{ value: customer.data.uuid, label: customer.data.display_name }]
        : [],
    [customer.data],
  );

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end gap-3">
        <div className="grid min-w-72 flex-1 gap-1.5">
          <Label htmlFor="wizard-customer">
            {t("reservations.fields.customer")}
          </Label>
          <AsyncCombobox
            id="wizard-customer"
            value={customerUuid}
            onValueChange={(uuid) => onCustomerChange(uuid)}
            loadOptions={loadOptions}
            initialOptions={initialOptions}
            placeholder={t("reservations.wizard.customer_placeholder")}
            searchPlaceholder={t("reservations.wizard.customer_search")}
            emptyText={t("reservations.wizard.customer_empty")}
            clearable
          />
        </div>
        <PermissionGuard permission={permissions.tenant.customers.write}>
          <Button
            type="button"
            variant="outline"
            onClick={() => setCreateOpen(true)}
          >
            <UserPlus className="size-4" />
            {t("reservations.wizard.new_customer")}
          </Button>
        </PermissionGuard>
      </div>

      {customer.data ? (
        <div className="bg-muted/40 flex flex-wrap items-center gap-3 rounded-lg border p-3 text-sm">
          <span className="font-medium">{customer.data.display_name}</span>
          <CustomerTypeChip type={customer.data.customer_type} />
          <span className="text-muted-foreground font-mono text-xs">
            {customer.data.phone_masked || "—"}
          </span>
          <span className="text-muted-foreground font-mono text-xs">
            {customer.data.email_masked || "—"}
          </span>
        </div>
      ) : null}

      {customerUuid ? (
        <div className="space-y-2">
          <div>
            <h3 className="text-sm font-semibold">
              {t("reservations.drivers.title")}
            </h3>
            <p className="text-muted-foreground text-sm">
              {t("reservations.drivers.help")}
            </p>
          </div>
          <CustomerDriversPicker
            key={customerUuid}
            customerUuid={customerUuid}
            value={drivers}
            onChange={onDriversChange}
          />
        </div>
      ) : null}

      <EntityDrawer
        open={createOpen}
        onOpenChange={setCreateOpen}
        title={t("reservations.wizard.new_customer")}
        description={t("reservations.wizard.new_customer_description")}
        size="xl"
      >
        {createOpen ? (
          <CustomerForm
            mode="create"
            defaultValues={customerFormDefaults()}
            isSubmitting={create.isPending}
            onCancel={() => setCreateOpen(false)}
            onSubmit={async (values) => {
              const created = await create.mutateAsync(
                customerFormToRequest(values, "create"),
              );
              onCustomerChange(created.uuid);
              setCreateOpen(false);
            }}
          />
        ) : null}
      </EntityDrawer>
    </div>
  );
}
