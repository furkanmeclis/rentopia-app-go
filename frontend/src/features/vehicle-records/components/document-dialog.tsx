"use client";

import { FileText } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { FileUploader } from "@/components/upload/file-uploader";
import {
  DOCUMENT_FILE_ACCEPT,
  DOCUMENT_TYPES,
  DOCUMENT_TYPE_LABEL_KEYS,
  MAX_DOCUMENT_FILE_BYTES,
} from "@/features/vehicle-records/constants";
import { useSaveVehicleDocument } from "@/features/vehicle-records/hooks/use-vehicle-records";
import type {
  VehicleDocument,
  VehicleDocumentType,
} from "@/features/vehicle-records/types";
import { fileSize } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "vehicle-document-form";

/**
 * Create / edit a vehicle document with an optional scan (PDF, JPEG, PNG,
 * WebP; at most 10 MB; the API checks the type from the bytes). Editing
 * with a new file replaces the stored one.
 */
export function DocumentDialog({
  vehicleUuid,
  open,
  document,
  onOpenChange,
}: {
  vehicleUuid: string;
  open: boolean;
  /** The document to edit; null creates a new one. */
  document: VehicleDocument | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const save = useSaveVehicleDocument(vehicleUuid);
  const [file, setFile] = useState<File | null>(null);
  const [uploaderKey, setUploaderKey] = useState(0);

  const schema = useMemo(
    () =>
      z
        .object({
          doc_type: z.string().min(1, t("vehicle_records.validation.required")),
          document_number: z.string().max(128),
          issuer: z.string().max(255),
          valid_from: z.string(),
          valid_until: z.string(),
          note: z.string().max(2000),
        })
        .refine(
          (v) =>
            !v.valid_from || !v.valid_until || v.valid_until >= v.valid_from,
          {
            path: ["valid_until"],
            message: t("vehicle_records.validation.until_before_from"),
          },
        ),
    [t],
  );
  type Values = z.infer<typeof schema>;

  const close = (next: boolean) => {
    if (save.isPending) return;
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
      title={
        document
          ? t("vehicle_records.documents.edit_title")
          : t("vehicle_records.documents.create_title")
      }
      description={t("vehicle_records.documents.dialog_description")}
      className="sm:max-w-2xl"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={save.isPending}>
            {save.isPending
              ? t("vehicle_records.saving")
              : t("vehicle_records.save")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<Values>
          key={document?.uuid ?? "new"}
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{
            doc_type: document?.doc_type ?? "muayene",
            document_number: document?.document_number ?? "",
            issuer: document?.issuer ?? "",
            valid_from: document?.valid_from ?? "",
            valid_until: document?.valid_until ?? "",
            note: document?.note ?? "",
          }}
          onSubmit={async (values) => {
            await save.mutateAsync({
              documentUuid: document?.uuid,
              body: {
                doc_type: values.doc_type as VehicleDocumentType,
                document_number: values.document_number.trim(),
                issuer: values.issuer.trim(),
                valid_from: values.valid_from || null,
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
            label={t("vehicle_records.fields.doc_type")}
            options={DOCUMENT_TYPES.map((value) => ({
              value,
              label: t(DOCUMENT_TYPE_LABEL_KEYS[value]),
            }))}
          />
          <AppInput
            name="document_number"
            label={t("vehicle_records.fields.document_number")}
          />
          <AppInput
            name="issuer"
            label={t("vehicle_records.fields.issuer")}
            className="sm:col-span-2"
          />
          <AppDatePicker
            name="valid_from"
            label={t("vehicle_records.fields.valid_from")}
          />
          <AppDatePicker
            name="valid_until"
            label={t("vehicle_records.fields.valid_until")}
            description={t("vehicle_records.fields.valid_until_hint")}
          />
          <AppTextarea
            name="note"
            label={t("vehicle_records.fields.note")}
            className="sm:col-span-2"
          />
          <div className="space-y-2 sm:col-span-2">
            <p className="text-sm font-medium">
              {document?.file
                ? t("vehicle_records.documents.replace_file")
                : t("vehicle_records.documents.file")}
            </p>
            {document?.file ? (
              <p className="text-muted-foreground flex items-center gap-1.5 text-xs">
                <FileText className="size-3.5" />
                {document.file.name} · {fileSize(document.file.size_bytes)}
              </p>
            ) : null}
            <FileUploader
              key={uploaderKey}
              multiple={false}
              accept={DOCUMENT_FILE_ACCEPT}
              maxSizeBytes={MAX_DOCUMENT_FILE_BYTES}
              onUpload={(files) => setFile(files[0] ?? null)}
            />
            <p className="text-muted-foreground text-xs">
              {t("vehicle_records.documents.file_hint")}
            </p>
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
