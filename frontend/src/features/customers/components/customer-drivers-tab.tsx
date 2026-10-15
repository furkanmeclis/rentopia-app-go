"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  BadgeCheck,
  ClipboardCheck,
  Pencil,
  Plus,
  Trash2,
  UserCheck,
} from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  SensitiveValue,
  useCustomerReveal,
} from "@/features/customers/components/customer-reveal";
import {
  ELIGIBILITY_REASON_LABEL_KEYS,
  IDENTITY_TYPE_LABEL_KEYS,
} from "@/features/customers/constants";
import {
  useCustomerDrivers,
  useDeleteCustomerDriver,
  useDriverEligibility,
  useSaveCustomerDriver,
} from "@/features/customers/hooks/use-customers";
import {
  countryName,
  countryOptions,
  isValidPhone,
} from "@/features/customers/lib/format";
import { identityError } from "@/features/customers/schemas/customer-form";
import type {
  Customer,
  CustomerDriver,
  CustomerDriverCreateRequest,
  DriverEligibility,
} from "@/features/customers/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const DRIVER_FORM_ID = "customer-driver-form";
const ELIGIBILITY_FORM_ID = "customer-driver-eligibility";

type DriverValues = {
  copy_from_customer: boolean;
  first_name: string;
  last_name: string;
  birth_date: string;
  phone: string;
  email: string;
  identity_type: "none" | "tckn" | "passport";
  identity_number: string;
  nationality: string;
  licence_number: string;
  licence_class: string;
  licence_issued_on: string;
  licence_expires_on: string;
  licence_country: string;
  note: string;
};

function DriverDialog({
  customer,
  open,
  driver,
  copyFromCustomer,
  onOpenChange,
}: {
  customer: Customer;
  open: boolean;
  driver: CustomerDriver | null;
  /** "Müşteri aynı zamanda sürücü": the person comes from the customer. */
  copyFromCustomer: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const save = useSaveCustomerDriver(customer.uuid);
  const countries = useMemo(() => countryOptions(locale), [locale]);
  const editing = Boolean(driver);
  const schema = useMemo(
    () =>
      z
        .object({
          copy_from_customer: z.boolean(),
          first_name: z.string().max(128),
          last_name: z.string().max(128),
          birth_date: z.string(),
          phone: z
            .string()
            .refine(isValidPhone, t("customers.validation.phone")),
          email: z.union([
            z.literal(""),
            z.email(t("customers.validation.email")),
          ]),
          identity_type: z.enum(["none", "tckn", "passport"]),
          identity_number: z.string().max(32),
          nationality: z.string(),
          licence_number: z.string().max(32),
          licence_class: z.string().max(32),
          licence_issued_on: z.string(),
          licence_expires_on: z.string(),
          licence_country: z.string(),
          note: z.string().max(2000),
        })
        .superRefine((v, ctx) => {
          const issue = (path: string, key: string) =>
            ctx.addIssue({ code: "custom", path: [path], message: t(key) });
          if (!v.copy_from_customer) {
            if (!v.first_name.trim())
              issue("first_name", "customers.validation.required");
            if (!v.last_name.trim())
              issue("last_name", "customers.validation.required");
          }
          if (v.identity_type !== "none" && v.identity_number.trim()) {
            const err = identityError(v.identity_type, v.identity_number);
            if (err) issue("identity_number", err);
          }
          const licence = v.licence_number.replace(/[^0-9A-Za-z]/g, "");
          if (
            v.licence_number.trim() &&
            (licence.length < 4 || licence.length > 32)
          ) {
            issue("licence_number", "customers.validation.licence");
          }
          if (
            v.licence_issued_on &&
            v.licence_expires_on &&
            v.licence_expires_on < v.licence_issued_on
          ) {
            issue("licence_expires_on", "customers.validation.licence_dates");
          }
          if (
            v.birth_date &&
            v.licence_issued_on &&
            v.licence_issued_on < v.birth_date
          ) {
            issue(
              "licence_issued_on",
              "customers.validation.licence_before_birth",
            );
          }
        }),
    [t],
  );
  const keep = editing ? t("customers.form.keep_hint") : undefined;
  const copying = !editing && copyFromCustomer;

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!save.isPending) onOpenChange(next);
      }}
      title={
        editing
          ? t("customers.drivers.edit_title")
          : copying
            ? t("customers.drivers.self_title")
            : t("customers.drivers.create_title")
      }
      description={t("customers.drivers.dialog_description")}
      className="sm:max-w-2xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={DRIVER_FORM_ID} disabled={save.isPending}>
            {t("customers.actions.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<DriverValues>
          key={driver?.uuid ?? (copying ? "self" : "new")}
          id={DRIVER_FORM_ID}
          className="grid max-h-[65vh] gap-4 overflow-y-auto px-1 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            copy_from_customer: copying,
            first_name: driver?.first_name ?? "",
            last_name: driver?.last_name ?? "",
            birth_date: driver?.birth_date ?? "",
            phone: "",
            email: "",
            identity_type: driver?.identity_type ?? (editing ? "none" : "tckn"),
            identity_number: "",
            nationality: driver?.nationality ?? (editing ? "" : "TR"),
            licence_number: "",
            licence_class: driver?.licence_class ?? "B",
            licence_issued_on: driver?.licence_issued_on ?? "",
            licence_expires_on: driver?.licence_expires_on ?? "",
            licence_country: driver?.licence_country ?? "TR",
            note: driver?.note ?? "",
          }}
          onSubmit={async (v) => {
            const body: Record<string, unknown> = {
              birth_date: v.birth_date || null,
              licence_class: v.licence_class.trim(),
              licence_issued_on: v.licence_issued_on || null,
              licence_expires_on: v.licence_expires_on || null,
              licence_country: v.licence_country || null,
              note: v.note.trim(),
            };
            if (v.copy_from_customer) {
              body.copy_from_customer = true;
            } else {
              body.first_name = v.first_name.trim();
              body.last_name = v.last_name.trim();
              body.nationality = v.nationality || null;
              if (v.identity_type === "none") {
                if (driver?.identity_type) body.identity_number = "";
              } else if (v.identity_number.trim()) {
                body.identity_type = v.identity_type;
                body.identity_number = v.identity_number.trim();
              }
              if (v.phone.trim() || !editing) body.phone = v.phone.trim();
              if (v.email.trim() || !editing) body.email = v.email.trim();
            }
            if (v.licence_number.trim()) {
              body.licence_number = v.licence_number.trim();
            }
            await save.mutateAsync({
              driverUuid: driver?.uuid,
              body: body as CustomerDriverCreateRequest,
            });
            onOpenChange(false);
          }}
        >
          {(form) => {
            const identityType = form.watch("identity_type");
            return (
              <>
                {copying ? (
                  <Alert className="sm:col-span-2">
                    <UserCheck className="size-4" />
                    <AlertDescription>
                      {t("customers.drivers.self_note", {
                        name: customer.display_name,
                      })}
                    </AlertDescription>
                  </Alert>
                ) : (
                  <>
                    <AppInput
                      name="first_name"
                      label={t("customers.fields.first_name")}
                    />
                    <AppInput
                      name="last_name"
                      label={t("customers.fields.last_name")}
                    />
                    <AppSelect
                      name="identity_type"
                      label={t("customers.fields.identity_type")}
                      options={[
                        {
                          value: "tckn",
                          label: t(IDENTITY_TYPE_LABEL_KEYS.tckn),
                        },
                        {
                          value: "passport",
                          label: t(IDENTITY_TYPE_LABEL_KEYS.passport),
                        },
                        {
                          value: "none",
                          label: t("customers.identity_types.none"),
                        },
                      ]}
                    />
                    {identityType !== "none" ? (
                      <AppInput
                        name="identity_number"
                        label={t("customers.fields.identity_number")}
                        autoComplete="off"
                        placeholder={
                          driver?.identity_type === identityType
                            ? driver.identity_masked
                            : undefined
                        }
                        description={
                          driver?.identity_type === identityType
                            ? keep
                            : undefined
                        }
                      />
                    ) : (
                      <div />
                    )}
                    <AppCombobox
                      name="nationality"
                      label={t("customers.fields.nationality")}
                      options={countries}
                      placeholder={t("customers.form.select_country")}
                      searchPlaceholder={t("customers.form.search_country")}
                      emptyText={t("customers.form.no_country")}
                    />
                    <AppInput
                      name="phone"
                      type="tel"
                      label={t("customers.fields.phone")}
                      placeholder={driver?.phone_masked || "0532 000 00 00"}
                      description={driver?.phone_masked ? keep : undefined}
                    />
                    <AppInput
                      name="email"
                      type="email"
                      label={t("customers.fields.email")}
                      placeholder={driver?.email_masked || undefined}
                      description={driver?.email_masked ? keep : undefined}
                    />
                  </>
                )}
                <AppDatePicker
                  name="birth_date"
                  label={t("customers.drivers.birth_date")}
                  description={t("customers.drivers.birth_date_hint")}
                />
                <AppInput
                  name="licence_number"
                  label={t("customers.drivers.licence_number")}
                  autoComplete="off"
                  placeholder={driver?.licence_masked || undefined}
                  description={driver?.licence_masked ? keep : undefined}
                />
                <AppInput
                  name="licence_class"
                  label={t("customers.drivers.licence_class")}
                  placeholder="B"
                />
                <AppDatePicker
                  name="licence_issued_on"
                  label={t("customers.drivers.licence_issued_on")}
                />
                <AppDatePicker
                  name="licence_expires_on"
                  label={t("customers.drivers.licence_expires_on")}
                />
                <AppCombobox
                  name="licence_country"
                  label={t("customers.drivers.licence_country")}
                  description={t("customers.drivers.licence_country_hint")}
                  options={countries}
                  placeholder={t("customers.form.select_country")}
                  searchPlaceholder={t("customers.form.search_country")}
                  emptyText={t("customers.form.no_country")}
                />
                <AppTextarea
                  name="note"
                  label={t("customers.fields.note")}
                  className="sm:col-span-2"
                />
              </>
            );
          }}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/** Checks a driver against product conditions (min age / licence years). */
function EligibilityDialog({
  customerUuid,
  driver,
  onOpenChange,
}: {
  customerUuid: string;
  driver: CustomerDriver | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const check = useDriverEligibility();
  const [result, setResult] = useState<DriverEligibility | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        min_driver_age: z
          .string()
          .regex(/^\d{1,3}$/, t("customers.validation.number")),
        min_license_years: z
          .string()
          .regex(/^\d{1,3}$/, t("customers.validation.number")),
      }),
    [t],
  );
  const close = (open: boolean) => {
    if (!open) setResult(null);
    onOpenChange(open);
  };

  return (
    <AppDialog
      open={Boolean(driver)}
      onOpenChange={close}
      title={t("customers.eligibility.title")}
      description={
        driver
          ? t("customers.eligibility.description", { name: driver.full_name })
          : undefined
      }
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={ELIGIBILITY_FORM_ID}
            disabled={check.isPending}
          >
            {t("customers.eligibility.check")}
          </Button>
        </>
      }
    >
      {driver ? (
        <div className="space-y-4">
          <AppForm<{ min_driver_age: string; min_license_years: string }>
            id={ELIGIBILITY_FORM_ID}
            className="grid gap-4 sm:grid-cols-2"
            schema={schema}
            defaultValues={{ min_driver_age: "21", min_license_years: "1" }}
            onSubmit={async (values) => {
              const out = await check.mutateAsync({
                uuid: customerUuid,
                driverUuid: driver.uuid,
                minDriverAge: Number(values.min_driver_age),
                minLicenseYears: Number(values.min_license_years),
              });
              setResult(out);
            }}
          >
            <AppInput
              name="min_driver_age"
              type="number"
              min={0}
              label={t("customers.eligibility.min_driver_age")}
            />
            <AppInput
              name="min_license_years"
              type="number"
              min={0}
              label={t("customers.eligibility.min_license_years")}
            />
          </AppForm>
          {result ? (
            <Alert variant={result.eligible ? "default" : "destructive"}>
              <BadgeCheck className="size-4" />
              <AlertDescription className="space-y-1">
                <p className="font-medium">
                  {result.eligible
                    ? t("customers.eligibility.eligible")
                    : t("customers.eligibility.not_eligible")}
                </p>
                {result.reasons.map((reason) => (
                  <p key={reason}>
                    {t(ELIGIBILITY_REASON_LABEL_KEYS[reason] ?? reason)}
                  </p>
                ))}
              </AlertDescription>
            </Alert>
          ) : null}
        </div>
      ) : null}
    </AppDialog>
  );
}

/**
 * "Sürücüler" tab: drivers of the customer (DataTable), "customer is also
 * driver" for individual customers, the rental condition check and dialog
 * CRUD. Identity / licence / phone show the mask unless revealed.
 */
export function CustomerDriversTab({ customer }: { customer: Customer }) {
  const { t, locale } = useLocale();
  const query = useCustomerDrivers(customer.uuid);
  const remove = useDeleteCustomerDriver(customer.uuid);
  const { revealed } = useCustomerReveal();
  const [dialog, setDialog] = useState<{
    open: boolean;
    driver: CustomerDriver | null;
    self: boolean;
  }>({ open: false, driver: null, self: false });
  const [deleteTarget, setDeleteTarget] = useState<CustomerDriver | null>(null);
  const [eligibilityTarget, setEligibilityTarget] =
    useState<CustomerDriver | null>(null);
  const write = permissions.tenant.customers.write;
  const readOnly = customer.anonymized;
  const items = useMemo(() => query.data?.items ?? [], [query.data]);
  const hasSelf = items.some((d) => d.is_customer_self);
  const plain = useMemo(() => {
    const map = new Map<
      string,
      { identity: string; licence: string; phone: string }
    >();
    for (const d of revealed?.drivers ?? []) {
      map.set(d.uuid, {
        identity: d.identity_number,
        licence: d.licence_number,
        phone: d.phone,
      });
    }
    return map;
  }, [revealed]);

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerDriver>({
          accessorKey: "full_name",
          labelKey: "customers.drivers.full_name",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-2 font-medium">
              {row.original.full_name || "—"}
              {row.original.is_customer_self ? (
                <Badge variant="outline">{t("customers.drivers.self")}</Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<CustomerDriver>({
          id: "age",
          accessorFn: (row) => row.age ?? -1,
          labelKey: "customers.drivers.age",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.age != null
              ? t("customers.drivers.years", { count: row.original.age })
              : "—",
        }),
        createColumn<CustomerDriver>({
          accessorKey: "identity_masked",
          labelKey: "customers.fields.identity_number",
          enableSorting: false,
          cell: ({ row }) => (
            <SensitiveValue
              masked={row.original.identity_masked}
              plain={plain.get(row.original.uuid)?.identity}
            />
          ),
        }),
        createColumn<CustomerDriver>({
          accessorKey: "licence_masked",
          labelKey: "customers.drivers.licence_number",
          enableSorting: false,
          cell: ({ row }) => (
            <SensitiveValue
              masked={row.original.licence_masked}
              plain={plain.get(row.original.uuid)?.licence}
            />
          ),
        }),
        createColumn<CustomerDriver>({
          accessorKey: "licence_class",
          labelKey: "customers.drivers.licence_class",
          enableSorting: true,
          cell: ({ row }) => row.original.licence_class || "—",
        }),
        createColumn<CustomerDriver>({
          id: "licence_years",
          accessorFn: (row) => row.licence_years ?? -1,
          labelKey: "customers.drivers.licence_years",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-2">
              {row.original.licence_years != null
                ? t("customers.drivers.years", {
                    count: row.original.licence_years,
                  })
                : "—"}
              {row.original.foreign_licence ? (
                <Badge variant="warning">
                  {t("customers.drivers.foreign", {
                    country: countryName(row.original.licence_country, locale),
                  })}
                </Badge>
              ) : null}
              {row.original.licence_expired ? (
                <StatusChip
                  label={t("customers.drivers.expired")}
                  tone="danger"
                />
              ) : null}
            </span>
          ),
        }),
        createColumn<CustomerDriver>({
          id: "licence_expires_on",
          accessorFn: (row) => row.licence_expires_on ?? "",
          labelKey: "customers.drivers.licence_expires_on",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.licence_expires_on
              ? date(row.original.licence_expires_on, undefined, locale)
              : "—",
        }),
        createColumn<CustomerDriver>({
          accessorKey: "phone_masked",
          labelKey: "customers.fields.phone",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <SensitiveValue
              masked={row.original.phone_masked}
              plain={plain.get(row.original.uuid)?.phone}
            />
          ),
        }),
        createColumn<CustomerDriver>({
          id: "actions",
          labelKey: "customers.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "eligibility",
                  label: t("customers.eligibility.action"),
                  icon: ClipboardCheck,
                  onSelect: () => setEligibilityTarget(row.original),
                },
                ...(readOnly
                  ? []
                  : [
                      {
                        id: "edit",
                        label: t("customers.actions.edit"),
                        icon: Pencil,
                        permission: write,
                        onSelect: () =>
                          setDialog({
                            open: true,
                            driver: row.original,
                            self: false,
                          }),
                      },
                      {
                        id: "delete",
                        label: t("customers.actions.delete"),
                        icon: Trash2,
                        permission: write,
                        variant: "destructive" as const,
                        onSelect: () => setDeleteTarget(row.original),
                      },
                    ]),
              ]}
            />
          ),
        }),
      ] as ColumnDef<CustomerDriver, unknown>[],
    [locale, plain, readOnly, t, write],
  );

  return (
    <EntitySectionCard
      title={t("customers.drivers.title")}
      badge={query.data ? items.length : undefined}
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("customers.drivers.help")}
      </p>
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("customers.drivers.empty_title")}
        emptyDescription={t("customers.drivers.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-customer-drivers-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          readOnly ? null : (
            <PermissionGuard permission={write}>
              {customer.customer_type === "individual" && !hasSelf ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() =>
                    setDialog({ open: true, driver: null, self: true })
                  }
                >
                  <UserCheck className="size-4" />
                  {t("customers.drivers.add_self")}
                </Button>
              ) : null}
              <Button
                type="button"
                size="sm"
                onClick={() =>
                  setDialog({ open: true, driver: null, self: false })
                }
              >
                <Plus className="size-4" />
                {t("customers.drivers.add")}
              </Button>
            </PermissionGuard>
          )
        }
      />
      <DriverDialog
        customer={customer}
        open={dialog.open}
        driver={dialog.driver}
        copyFromCustomer={dialog.self}
        onOpenChange={(open) => setDialog((current) => ({ ...current, open }))}
      />
      <EligibilityDialog
        customerUuid={customer.uuid}
        driver={eligibilityTarget}
        onOpenChange={(open) => {
          if (!open) setEligibilityTarget(null);
        }}
      />
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title={t("customers.drivers.delete_title")}
        description={deleteTarget?.full_name}
        confirmLabel={t("customers.actions.delete")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </EntitySectionCard>
  );
}
