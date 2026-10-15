"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Star, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { PermissionGuard } from "@/components/common/permission-guard";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { AppForm, AppInput, AppSwitch, AppTextarea } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  SensitiveValue,
  useCustomerReveal,
} from "@/features/customers/components/customer-reveal";
import {
  useCustomerContacts,
  useDeleteCustomerContact,
  useSaveCustomerContact,
} from "@/features/customers/hooks/use-customers";
import { isValidPhone } from "@/features/customers/lib/format";
import type { CustomerContact } from "@/features/customers/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "customer-contact-form";

type Values = {
  full_name: string;
  title: string;
  phone: string;
  email: string;
  is_primary: boolean;
  note: string;
};

function ContactDialog({
  customerUuid,
  open,
  contact,
  onOpenChange,
}: {
  customerUuid: string;
  open: boolean;
  contact: CustomerContact | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const save = useSaveCustomerContact(customerUuid);
  const schema = useMemo(
    () =>
      z.object({
        full_name: z
          .string()
          .trim()
          .min(1, t("customers.validation.required"))
          .max(255),
        title: z.string().max(128),
        phone: z.string().refine(isValidPhone, t("customers.validation.phone")),
        email: z.union([
          z.literal(""),
          z.email(t("customers.validation.email")),
        ]),
        is_primary: z.boolean(),
        note: z.string().max(2000),
      }),
    [t],
  );
  const keep = contact ? t("customers.form.keep_hint") : undefined;

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!save.isPending) onOpenChange(next);
      }}
      title={
        contact
          ? t("customers.contacts.edit_title")
          : t("customers.contacts.create_title")
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={save.isPending}>
            {t("customers.actions.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<Values>
          key={contact?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            full_name: contact?.full_name ?? "",
            title: contact?.title ?? "",
            phone: "",
            email: "",
            is_primary: contact?.is_primary ?? false,
            note: contact?.note ?? "",
          }}
          onSubmit={async (values) => {
            await save.mutateAsync({
              contactUuid: contact?.uuid,
              body: {
                full_name: values.full_name.trim(),
                title: values.title.trim(),
                is_primary: values.is_primary,
                note: values.note.trim(),
                ...(values.phone.trim() || !contact
                  ? { phone: values.phone.trim() }
                  : {}),
                ...(values.email.trim() || !contact
                  ? { email: values.email.trim() }
                  : {}),
              },
            });
            onOpenChange(false);
          }}
        >
          <AppInput
            name="full_name"
            label={t("customers.contacts.full_name")}
          />
          <AppInput name="title" label={t("customers.contacts.title_field")} />
          <AppInput
            name="phone"
            type="tel"
            label={t("customers.fields.phone")}
            placeholder={contact?.phone_masked || "0532 000 00 00"}
            description={contact?.phone_masked ? keep : undefined}
          />
          <AppInput
            name="email"
            type="email"
            label={t("customers.fields.email")}
            placeholder={contact?.email_masked || undefined}
            description={contact?.email_masked ? keep : undefined}
          />
          <AppSwitch
            name="is_primary"
            label={t("customers.contacts.is_primary")}
            className="sm:col-span-2"
          />
          <AppTextarea
            name="note"
            label={t("customers.fields.note")}
            className="sm:col-span-2"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/** Contacts of a corporate customer (DataTable with dialog CRUD). */
export function CustomerContactsCard({
  customerUuid,
  readOnly,
}: {
  customerUuid: string;
  readOnly: boolean;
}) {
  const { t } = useLocale();
  const query = useCustomerContacts(customerUuid);
  const remove = useDeleteCustomerContact(customerUuid);
  const { revealed } = useCustomerReveal();
  const [dialog, setDialog] = useState<{
    open: boolean;
    contact: CustomerContact | null;
  }>({ open: false, contact: null });
  const [deleteTarget, setDeleteTarget] = useState<CustomerContact | null>(
    null,
  );
  const write = permissions.tenant.customers.write;
  const plain = useMemo(() => {
    const map = new Map<string, { phone: string; email: string }>();
    for (const c of revealed?.contacts ?? []) map.set(c.uuid, c);
    return map;
  }, [revealed]);

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerContact>({
          accessorKey: "full_name",
          labelKey: "customers.contacts.full_name",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="flex items-center gap-2 font-medium">
              {row.original.full_name}
              {row.original.is_primary ? (
                <Badge variant="outline" className="gap-1">
                  <Star className="size-3" aria-hidden />
                  {t("customers.contacts.primary")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<CustomerContact>({
          accessorKey: "title",
          labelKey: "customers.contacts.title_field",
          enableSorting: true,
          cell: ({ row }) => row.original.title || "—",
        }),
        createColumn<CustomerContact>({
          accessorKey: "phone_masked",
          labelKey: "customers.fields.phone",
          enableSorting: false,
          cell: ({ row }) => (
            <SensitiveValue
              masked={row.original.phone_masked}
              plain={plain.get(row.original.uuid)?.phone}
            />
          ),
        }),
        createColumn<CustomerContact>({
          accessorKey: "email_masked",
          labelKey: "customers.fields.email",
          enableSorting: false,
          cell: ({ row }) => (
            <SensitiveValue
              masked={row.original.email_masked}
              plain={plain.get(row.original.uuid)?.email}
            />
          ),
        }),
        createColumn<CustomerContact>({
          accessorKey: "note",
          labelKey: "customers.fields.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<CustomerContact>({
          id: "actions",
          labelKey: "customers.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) =>
            readOnly ? null : (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("customers.actions.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () =>
                      setDialog({ open: true, contact: row.original }),
                  },
                  {
                    id: "delete",
                    label: t("customers.actions.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive",
                    onSelect: () => setDeleteTarget(row.original),
                  },
                ]}
              />
            ),
        }),
      ] as ColumnDef<CustomerContact, unknown>[],
    [plain, readOnly, t, write],
  );

  const items = query.data?.items ?? [];

  return (
    <EntitySectionCard
      title={t("customers.contacts.title")}
      badge={query.data ? items.length : undefined}
    >
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("customers.contacts.empty_title")}
        emptyDescription={t("customers.contacts.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-customer-contacts-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          readOnly ? null : (
            <PermissionGuard permission={write}>
              <Button
                type="button"
                size="sm"
                onClick={() => setDialog({ open: true, contact: null })}
              >
                <Plus className="size-4" />
                {t("customers.contacts.add")}
              </Button>
            </PermissionGuard>
          )
        }
      />
      <ContactDialog
        customerUuid={customerUuid}
        open={dialog.open}
        contact={dialog.contact}
        onOpenChange={(open) => setDialog((current) => ({ ...current, open }))}
      />
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title={t("customers.contacts.delete_title")}
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
