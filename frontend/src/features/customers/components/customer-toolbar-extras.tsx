"use client";

import { ShieldAlert } from "lucide-react";
import { useCallback, useMemo, useState, type ReactNode } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { permissions } from "@/config/permissions";
import type { BulkActionDef } from "@/features/bulk-engine";
import { waitAndDownload } from "@/features/io/components/export-menu";
import { exportsService } from "@/features/io/services/exports.service";
import { EXPORT_PATHS, type ExportFormat } from "@/features/io/types";
import { useApiScope } from "@/features/tenant/hooks/use-api-scope";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const TAG_FORM_ID = "customer-bulk-tag";
const EXPORT_FORM_ID = "customer-sensitive-export";

type TagRequest = {
  action: BulkActionDef;
  resolve: (params: Record<string, string> | null) => void;
};

/**
 * Collects the `tag` param of the customers bulk actions (add / remove tag).
 * `resolveParams` plugs into `BulkActionMenu`.
 */
export function useCustomerBulkParams(): {
  resolveParams: (
    action: BulkActionDef,
  ) => Promise<Record<string, string> | null | undefined>;
  dialog: ReactNode;
} {
  const { t } = useLocale();
  const [request, setRequest] = useState<TagRequest | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        tag: z
          .string()
          .trim()
          .min(1, t("customers.validation.required"))
          .max(40, t("customers.validation.tag_length")),
      }),
    [t],
  );

  const resolveParams = useCallback(
    (action: BulkActionDef) =>
      new Promise<Record<string, string> | null>((resolve) =>
        setRequest({ action, resolve }),
      ),
    [],
  );
  const finish = (params: Record<string, string> | null) => {
    request?.resolve(params);
    setRequest(null);
  };

  const dialog = (
    <AppDialog
      open={Boolean(request)}
      onOpenChange={(open) => {
        if (!open) finish(null);
      }}
      title={request ? t(request.action.label_key) : ""}
      description={t("customers.bulk.tag_hint")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => finish(null)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={TAG_FORM_ID}>
            {t("customers.bulk.continue")}
          </Button>
        </>
      }
    >
      {request ? (
        <AppForm<{ tag: string }>
          key={request.action.id}
          id={TAG_FORM_ID}
          className="space-y-4"
          schema={schema}
          defaultValues={{ tag: "" }}
          onSubmit={async (values) => finish({ tag: values.tag.trim() })}
        >
          <AppInput name="tag" label={t("customers.bulk.tag")} />
        </AppForm>
      ) : null}
    </AppDialog>
  );

  return { resolveParams, dialog };
}

const FORMATS: ExportFormat[] = ["xlsx", "csv", "pdf", "json"];

const FORMAT_LABEL_KEYS: Record<ExportFormat, string> = {
  xlsx: "exports.formats.xlsx",
  csv: "exports.formats.csv",
  pdf: "exports.formats.pdf",
  json: "exports.formats.json",
};

/**
 * "Hassas verilerle dışa aktar": an export with plaintext identity / phone /
 * e-mail values. Shown to members with export and reveal; the API records
 * the request as an activity event. Regular exports stay masked.
 */
export function SensitiveExportButton({
  query,
}: {
  query: Record<string, string | undefined>;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const scope = useApiScope();
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const schema = useMemo(
    () => z.object({ format: z.enum(["xlsx", "csv", "pdf", "json"]) }),
    [],
  );

  if (
    !can(permissions.tenant.customers.export) ||
    !can(permissions.tenant.customers.reveal)
  ) {
    return null;
  }

  const run = async (format: ExportFormat) => {
    setPending(true);
    try {
      const cleaned: Record<string, string> = { include_sensitive: "true" };
      for (const [key, value] of Object.entries(query)) {
        if (value) cleaned[key] = value;
      }
      const job = await exportsService.request(
        EXPORT_PATHS["tenant.customers"],
        { format, query: cleaned, locale },
        scope,
      );
      setOpen(false);
      appToast.info(t("exports.toast.preparing"));
      const ok = await waitAndDownload(job, scope);
      if (ok) appToast.success(t("exports.toast.ready"));
      else appToast.error(t("exports.toast.download_failed"));
    } catch {
      appToast.error(t("exports.toast.failed"));
    } finally {
      setPending(false);
    }
  };

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="icon"
            className="size-8"
            disabled={pending}
            aria-label={t("customers.sensitive_export.button")}
            onClick={() => setOpen(true)}
          >
            <ShieldAlert className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="bottom">
          {t("customers.sensitive_export.button")}
        </TooltipContent>
      </Tooltip>
      <AppDialog
        open={open}
        onOpenChange={(next) => {
          if (!pending) setOpen(next);
        }}
        title={t("customers.sensitive_export.title")}
        description={t("customers.sensitive_export.description")}
        className="sm:max-w-md"
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setOpen(false)}
              disabled={pending}
            >
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              form={EXPORT_FORM_ID}
              variant="destructive"
              disabled={pending}
            >
              {t("customers.sensitive_export.confirm")}
            </Button>
          </>
        }
      >
        {open ? (
          <AppForm<{ format: ExportFormat }>
            id={EXPORT_FORM_ID}
            className="space-y-4"
            schema={schema}
            defaultValues={{ format: "xlsx" }}
            onSubmit={async (values) => run(values.format)}
          >
            <Alert variant="destructive">
              <ShieldAlert className="size-4" />
              <AlertDescription>
                {t("customers.sensitive_export.warning")}
              </AlertDescription>
            </Alert>
            <AppSelect
              name="format"
              label={t("customers.sensitive_export.format")}
              options={FORMATS.map((format) => ({
                value: format,
                label: t(FORMAT_LABEL_KEYS[format]),
              }))}
            />
          </AppForm>
        ) : null}
      </AppDialog>
    </>
  );
}
