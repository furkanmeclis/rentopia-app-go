"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Download, Paperclip, Pencil, Plus, Trash2 } from "lucide-react";
import { useMemo, useRef, useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { ResourceIOToolbar } from "@/features/io";
import { DocumentDialog } from "@/features/vehicle-records/components/document-dialog";
import {
  DaysLeft,
  DocumentStatusChip,
  ExpiryDate,
} from "@/features/vehicle-records/components/record-cells";
import {
  DOCUMENTS_RESOURCE,
  DOCUMENT_FILE_ACCEPT,
  DOCUMENT_STATUSES,
  DOCUMENT_STATUS_LABEL_KEYS,
  DOCUMENT_TYPES,
  DOCUMENT_TYPE_LABEL_KEYS,
  MAX_DOCUMENT_FILE_BYTES,
} from "@/features/vehicle-records/constants";
import {
  useDeleteVehicleDocument,
  useDownloadVehicleDocument,
  useUploadVehicleDocumentFile,
  useVehicleDocuments,
} from "@/features/vehicle-records/hooks/use-vehicle-records";
import type { VehicleDocument } from "@/features/vehicle-records/types";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * "Belgeler" tab of a vehicle: every document with its history, the
 * current one per type flagged, validity chips, upload / replace / download
 * of the scan (streamed by the API) and an export of the vehicle's
 * documents.
 */
export function VehicleDocumentsTab({ vehicleUuid }: { vehicleUuid: string }) {
  const { t, locale } = useLocale();
  const query = useVehicleDocuments(vehicleUuid);
  const remove = useDeleteVehicleDocument(vehicleUuid);
  const download = useDownloadVehicleDocument(vehicleUuid);
  const upload = useUploadVehicleDocumentFile(vehicleUuid);
  const [dialog, setDialog] = useState<{
    open: boolean;
    document: VehicleDocument | null;
  }>({ open: false, document: null });
  const [deleteTarget, setDeleteTarget] = useState<VehicleDocument | null>(
    null,
  );
  const fileInput = useRef<HTMLInputElement>(null);
  const uploadTarget = useRef<VehicleDocument | null>(null);
  const write = permissions.tenant.vehicles.write;

  const columns = useMemo(
    () =>
      [
        createColumn<VehicleDocument>({
          id: "doc_type",
          accessorFn: (row) => row.doc_type,
          labelKey: "vehicle_records.columns.doc_type",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: DOCUMENT_TYPES.map((value) => ({
            value,
            labelKey: DOCUMENT_TYPE_LABEL_KEYS[value],
            label: t(DOCUMENT_TYPE_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">
                {t(DOCUMENT_TYPE_LABEL_KEYS[row.original.doc_type])}
              </span>
              {row.original.is_current ? (
                <Badge variant="outline">{t("vehicle_records.current")}</Badge>
              ) : null}
            </div>
          ),
        }),
        createColumn<VehicleDocument>({
          accessorKey: "document_number",
          labelKey: "vehicle_records.columns.document_number",
          enableSorting: true,
          cell: ({ row }) => row.original.document_number || "—",
        }),
        createColumn<VehicleDocument>({
          accessorKey: "issuer",
          labelKey: "vehicle_records.columns.issuer",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => row.original.issuer || "—",
        }),
        createColumn<VehicleDocument>({
          id: "valid_from",
          accessorFn: (row) => row.valid_from ?? "",
          labelKey: "vehicle_records.columns.valid_from",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.valid_from
              ? date(row.original.valid_from, undefined, locale)
              : "—",
        }),
        createColumn<VehicleDocument>({
          id: "valid_until",
          accessorFn: (row) => row.valid_until ?? "",
          labelKey: "vehicle_records.columns.valid_until",
          enableSorting: true,
          cell: ({ row }) => (
            <div className="flex flex-col">
              <ExpiryDate value={row.original.valid_until} />
              {row.original.is_current ? (
                <DaysLeft days={row.original.days_left} />
              ) : null}
            </div>
          ),
        }),
        createColumn<VehicleDocument>({
          id: "status",
          accessorFn: (row) => row.status,
          labelKey: "vehicle_records.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: DOCUMENT_STATUSES.map((value) => ({
            value,
            labelKey: DOCUMENT_STATUS_LABEL_KEYS[value],
            label: t(DOCUMENT_STATUS_LABEL_KEYS[value]),
          })),
          cell: ({ row }) => (
            <DocumentStatusChip status={row.original.status} />
          ),
        }),
        createColumn<VehicleDocument>({
          id: "file",
          accessorFn: (row) => row.file?.name ?? "",
          labelKey: "vehicle_records.columns.file",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.file ? (
              <span className="flex items-center gap-1.5 text-sm">
                <Paperclip className="text-muted-foreground size-3.5" />
                <span className="max-w-48 truncate">
                  {row.original.file.name}
                </span>
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<VehicleDocument>({
          accessorKey: "note",
          labelKey: "vehicle_records.columns.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<VehicleDocument>({
          id: "actions",
          labelKey: "vehicle_records.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => {
            const doc = row.original;
            return (
              <EntityRowActions
                actions={[
                  ...(doc.file
                    ? [
                        {
                          id: "download",
                          label: t("vehicle_records.documents.download"),
                          icon: Download,
                          permission: permissions.tenant.document.download,
                          disabled: download.isPending,
                          onSelect: () =>
                            download.mutate({
                              documentUuid: doc.uuid,
                              fallbackName: doc.file?.name ?? "document",
                            }),
                        },
                      ]
                    : []),
                  {
                    id: "upload",
                    label: doc.file
                      ? t("vehicle_records.documents.replace_file")
                      : t("vehicle_records.documents.upload_file"),
                    icon: Paperclip,
                    permission: write,
                    disabled: upload.isPending,
                    onSelect: () => {
                      uploadTarget.current = doc;
                      fileInput.current?.click();
                    },
                  },
                  {
                    id: "edit",
                    label: t("vehicle_records.edit"),
                    icon: Pencil,
                    permission: write,
                    onSelect: () => setDialog({ open: true, document: doc }),
                  },
                  {
                    id: "delete",
                    label: t("vehicle_records.delete"),
                    icon: Trash2,
                    permission: write,
                    variant: "destructive",
                    onSelect: () => setDeleteTarget(doc),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<VehicleDocument, unknown>[],
    [download, locale, t, upload.isPending, write],
  );

  const items = query.data?.items ?? [];

  return (
    <EntitySectionCard
      title={t("vehicle_records.documents.title")}
      badge={query.data ? items.length : undefined}
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("vehicle_records.documents.help")}
      </p>
      <EntityTable
        columns={columns}
        data={items}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("vehicle_records.documents.empty_title")}
        emptyDescription={t("vehicle_records.documents.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-vehicle-documents-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <>
            <ResourceIOToolbar
              resource={DOCUMENTS_RESOURCE}
              query={{ vehicle: vehicleUuid }}
              capabilities={{ export: true }}
            />
            <PermissionGuard permission={write}>
              <Button
                type="button"
                size="sm"
                onClick={() => setDialog({ open: true, document: null })}
              >
                <Plus className="size-4" />
                {t("vehicle_records.documents.add")}
              </Button>
            </PermissionGuard>
          </>
        }
      />

      {/* Row action "upload / replace file": the shared hidden picker. */}
      <input
        ref={fileInput}
        type="file"
        className="hidden"
        accept={DOCUMENT_FILE_ACCEPT}
        onChange={(event) => {
          const file = event.target.files?.[0];
          const target = uploadTarget.current;
          event.target.value = "";
          if (!file || !target) return;
          if (file.size > MAX_DOCUMENT_FILE_BYTES) {
            appToast.error(t("vehicle_records.documents.file_too_large"));
            return;
          }
          upload.mutate({ documentUuid: target.uuid, file });
        }}
      />

      <DocumentDialog
        vehicleUuid={vehicleUuid}
        open={dialog.open}
        document={dialog.document}
        onOpenChange={(open) => setDialog((current) => ({ ...current, open }))}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title={t("vehicle_records.documents.delete_title")}
        description={
          deleteTarget
            ? t("vehicle_records.documents.delete_description", {
                type: t(DOCUMENT_TYPE_LABEL_KEYS[deleteTarget.doc_type]),
              })
            : undefined
        }
        confirmLabel={t("vehicle_records.delete")}
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
