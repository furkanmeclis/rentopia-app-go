"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Download, Lock, Paperclip, Plus, Trash2 } from "lucide-react";
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
import {
  AppDatePicker,
  AppForm,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { FileUploader } from "@/components/upload/file-uploader";
import { permissions } from "@/config/permissions";
import {
  DOCUMENT_FILE_ACCEPT,
  DOCUMENT_TYPES,
  DOCUMENT_TYPE_LABEL_KEYS,
  MAX_DOCUMENT_FILE_BYTES,
} from "@/features/customers/constants";
import {
  useCreateCustomerDocument,
  useCustomerDocuments,
  useCustomerDrivers,
  useDeleteCustomerDocument,
  useDownloadCustomerDocument,
  useUploadCustomerDocumentFile,
} from "@/features/customers/hooks/use-customers";
import type {
  CustomerDocument,
  CustomerDocumentType,
} from "@/features/customers/types";
import { fileSize } from "@/lib/utils";
import { date, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "customer-document-form";
const NO_DRIVER = "none";

type Values = {
  doc_type: CustomerDocumentType;
  driver_uuid: string;
  valid_until: string;
  note: string;
};

function DocumentDialog({
  customerUuid,
  open,
  onOpenChange,
}: {
  customerUuid: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const create = useCreateCustomerDocument(customerUuid);
  const drivers = useCustomerDrivers(customerUuid);
  const [file, setFile] = useState<File | null>(null);
  const [uploaderKey, setUploaderKey] = useState(0);
  const schema = useMemo(
    () =>
      z.object({
        doc_type: z.enum([
          "identity",
          "passport",
          "driving_licence",
          "tax_certificate",
          "signature_circular",
          "contract",
          "other",
        ]),
        driver_uuid: z.string(),
        valid_until: z.string(),
        note: z.string().max(2000),
      }),
    [],
  );
  const close = (next: boolean) => {
    if (create.isPending) return;
    if (!next) {
      setFile(null);
      setUploaderKey((k) => k + 1);
    }
    onOpenChange(next);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("customers.documents.create_title")}
      description={t("customers.documents.dialog_description")}
      className="sm:max-w-2xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={create.isPending}>
            {t("customers.actions.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<Values>
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            doc_type: "identity",
            driver_uuid: NO_DRIVER,
            valid_until: "",
            note: "",
          }}
          onSubmit={async (values) => {
            await create.mutateAsync({
              body: {
                doc_type: values.doc_type,
                driver_uuid:
                  values.driver_uuid === NO_DRIVER ? null : values.driver_uuid,
                valid_until: values.valid_until || null,
                note: values.note.trim(),
              },
              file,
            });
            close(false);
          }}
        >
          <AppSelect
            name="doc_type"
            label={t("customers.documents.doc_type")}
            options={DOCUMENT_TYPES.map((value) => ({
              value,
              label: t(DOCUMENT_TYPE_LABEL_KEYS[value]),
            }))}
          />
          <AppSelect
            name="driver_uuid"
            label={t("customers.documents.driver")}
            options={[
              { value: NO_DRIVER, label: t("customers.documents.no_driver") },
              ...(drivers.data?.items ?? []).map((driver) => ({
                value: driver.uuid,
                label: driver.full_name,
              })),
            ]}
          />
          <AppDatePicker
            name="valid_until"
            label={t("customers.documents.valid_until")}
          />
          <div />
          <AppTextarea
            name="note"
            label={t("customers.fields.note")}
            className="sm:col-span-2"
          />
          <div className="space-y-2 sm:col-span-2">
            <p className="text-sm font-medium">
              {t("customers.documents.file")}
            </p>
            <FileUploader
              key={uploaderKey}
              multiple={false}
              accept={DOCUMENT_FILE_ACCEPT}
              maxSizeBytes={MAX_DOCUMENT_FILE_BYTES}
              onUpload={(files) => setFile(files[0] ?? null)}
            />
            <p className="text-muted-foreground text-xs">
              {t("customers.documents.file_hint")}
            </p>
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

function UploadDialog({
  customerUuid,
  document,
  onOpenChange,
}: {
  customerUuid: string;
  document: CustomerDocument | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const upload = useUploadCustomerDocumentFile(customerUuid);
  const [file, setFile] = useState<File | null>(null);
  return (
    <AppDialog
      open={Boolean(document)}
      onOpenChange={(open) => {
        if (upload.isPending) return;
        if (!open) setFile(null);
        onOpenChange(open);
      }}
      title={
        document?.has_file
          ? t("customers.documents.replace_file")
          : t("customers.documents.upload_file")
      }
      description={t("customers.documents.file_hint")}
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
          <Button
            type="button"
            disabled={!file || upload.isPending}
            onClick={async () => {
              if (!document || !file) return;
              await upload.mutateAsync({ docUuid: document.uuid, file });
              setFile(null);
              onOpenChange(false);
            }}
          >
            {t("customers.documents.upload")}
          </Button>
        </>
      }
    >
      {document ? (
        <FileUploader
          multiple={false}
          accept={DOCUMENT_FILE_ACCEPT}
          maxSizeBytes={MAX_DOCUMENT_FILE_BYTES}
          onUpload={(files) => setFile(files[0] ?? null)}
        />
      ) : null}
    </AppDialog>
  );
}

/**
 * "Belgeler" tab: identity / licence scans and other documents in private
 * storage. Downloads are streamed by the API and need the document
 * download and reveal permissions; each download is recorded.
 */
export function CustomerDocumentsTab({
  customerUuid,
  readOnly,
}: {
  customerUuid: string;
  readOnly: boolean;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const query = useCustomerDocuments(customerUuid);
  const remove = useDeleteCustomerDocument(customerUuid);
  const download = useDownloadCustomerDocument(customerUuid);
  const [createOpen, setCreateOpen] = useState(false);
  const [uploadTarget, setUploadTarget] = useState<CustomerDocument | null>(
    null,
  );
  const [deleteTarget, setDeleteTarget] = useState<CustomerDocument | null>(
    null,
  );
  const write = permissions.tenant.customers.write;
  const canDownload =
    can(permissions.tenant.document.download) &&
    can(permissions.tenant.customers.reveal);

  const columns = useMemo(
    () =>
      [
        createColumn<CustomerDocument>({
          id: "doc_type",
          accessorFn: (row) => row.doc_type,
          labelKey: "customers.documents.doc_type",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: DOCUMENT_TYPES.map((value) => ({
            value,
            labelKey: DOCUMENT_TYPE_LABEL_KEYS[value],
            label: t(DOCUMENT_TYPE_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <span className="font-medium">
              {t(DOCUMENT_TYPE_LABEL_KEYS[row.original.doc_type])}
            </span>
          ),
        }),
        createColumn<CustomerDocument>({
          id: "driver",
          accessorFn: (row) => row.driver?.full_name ?? "",
          labelKey: "customers.documents.driver",
          enableSorting: true,
          cell: ({ row }) => row.original.driver?.full_name || "—",
        }),
        createColumn<CustomerDocument>({
          id: "valid_until",
          accessorFn: (row) => row.valid_until ?? "",
          labelKey: "customers.documents.valid_until",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.valid_until
              ? date(row.original.valid_until, undefined, locale)
              : "—",
        }),
        createColumn<CustomerDocument>({
          id: "file",
          accessorFn: (row) => row.file_name,
          labelKey: "customers.documents.file",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.has_file ? (
              <span className="flex items-center gap-1.5 text-sm">
                <Lock className="text-muted-foreground size-3.5" />
                <span className="max-w-48 truncate">
                  {row.original.file_name}
                </span>
                <span className="text-muted-foreground text-xs">
                  {fileSize(row.original.size_bytes)}
                </span>
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<CustomerDocument>({
          accessorKey: "created_at",
          labelKey: "customers.columns.created_at",
          enableSorting: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<CustomerDocument>({
          accessorKey: "note",
          labelKey: "customers.fields.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<CustomerDocument>({
          id: "actions",
          labelKey: "customers.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => {
            const doc = row.original;
            return (
              <EntityRowActions
                actions={[
                  ...(doc.has_file && canDownload
                    ? [
                        {
                          id: "download",
                          label: t("customers.documents.download"),
                          icon: Download,
                          disabled: download.isPending,
                          onSelect: () =>
                            download.mutate({
                              docUuid: doc.uuid,
                              fallbackName: doc.file_name || "document",
                            }),
                        },
                      ]
                    : []),
                  ...(readOnly
                    ? []
                    : [
                        {
                          id: "upload",
                          label: doc.has_file
                            ? t("customers.documents.replace_file")
                            : t("customers.documents.upload_file"),
                          icon: Paperclip,
                          permission: write,
                          onSelect: () => setUploadTarget(doc),
                        },
                        {
                          id: "delete",
                          label: t("customers.actions.delete"),
                          icon: Trash2,
                          permission: write,
                          variant: "destructive" as const,
                          onSelect: () => setDeleteTarget(doc),
                        },
                      ]),
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<CustomerDocument, unknown>[],
    [canDownload, download, locale, readOnly, t, write],
  );

  const items = query.data?.items ?? [];

  return (
    <EntitySectionCard
      title={t("customers.documents.title")}
      badge={query.data ? items.length : undefined}
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {canDownload
          ? t("customers.documents.help")
          : t("customers.documents.help_no_download")}
      </p>
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("customers.documents.empty_title")}
        emptyDescription={t("customers.documents.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-customer-documents-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          readOnly ? null : (
            <PermissionGuard permission={write}>
              <Button
                type="button"
                size="sm"
                onClick={() => setCreateOpen(true)}
              >
                <Plus className="size-4" />
                {t("customers.documents.add")}
              </Button>
            </PermissionGuard>
          )
        }
      />
      <DocumentDialog
        customerUuid={customerUuid}
        open={createOpen}
        onOpenChange={setCreateOpen}
      />
      <UploadDialog
        customerUuid={customerUuid}
        document={uploadTarget}
        onOpenChange={(open) => {
          if (!open) setUploadTarget(null);
        }}
      />
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title={t("customers.documents.delete_title")}
        description={
          deleteTarget
            ? t(DOCUMENT_TYPE_LABEL_KEYS[deleteTarget.doc_type])
            : undefined
        }
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
