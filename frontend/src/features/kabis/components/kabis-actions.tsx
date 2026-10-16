"use client";

import { FileText } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { FileUploader } from "@/components/upload/file-uploader";
import { missingFieldText } from "@/features/kabis/components/kabis-cells";
import {
  KABIS_ATTACHMENT_ACCEPT,
  KABIS_ATTACHMENT_MAX_BYTES,
  KABIS_RETRY_ACTION_KEYS,
} from "@/features/kabis/constants";
import {
  useResolveKabis,
  useRetryKabis,
} from "@/features/kabis/hooks/use-kabis";
import type { KabisResolution } from "@/features/kabis/types";
import { ApiError } from "@/lib/api";
import { fileSize } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type T = (key: string, params?: Record<string, string | number>) => string;

/** Text of a refused KABİS action: the code first, then the details. */
export function kabisProblemText(t: T, error: unknown): string {
  if (!(error instanceof ApiError)) return t("kabis.problems.generic");
  switch (error.code) {
    case "KABIS_DATA_MISSING":
      return t("kabis.problems.data_missing", {
        fields: error.details
          .map((d) => (d.field ? missingFieldText(t, d.field) : d.message))
          .join(", "),
      });
    case "KABIS_DEPENDENCY_PENDING":
      return t("kabis.problems.dependency_pending");
    case "KABIS_UNVERIFIED":
      return t("kabis.problems.unverified");
    case "KABIS_BUSY":
      return t("kabis.problems.busy");
    case "KABIS_NOT_RETRYABLE":
      return t("kabis.problems.not_retryable");
    case "KABIS_NOT_RESOLVABLE":
      return t("kabis.problems.not_resolvable");
  }
  if (error.details.length > 0) {
    return error.details
      .map((d) => (d.field ? `${d.field}: ${d.message}` : d.message))
      .join(" · ");
  }
  return error.message;
}

/**
 * "Yeniden dene": a status check first on the server; the toast says what
 * happened (re-validated, already reported, still in progress, sent again)
 * or why nothing was sent.
 */
export function useKabisRetryAction() {
  const { t } = useLocale();
  const retry = useRetryKabis();
  return {
    pending: retry.isPending,
    run: (uuid: string) =>
      retry.mutate(uuid, {
        onSuccess: (res) =>
          appToast.success(
            t(KABIS_RETRY_ACTION_KEYS[res.action] ?? "kabis.toast.retried"),
          ),
        onError: (error) => appToast.error(kabisProblemText(t, error)),
      }),
  };
}

const RESOLVE_FORM = "kabis-resolve-form";

type ResolveValues = { reason: string; reference: string };

/**
 * Mark a notification "reported manually" (filed through the official
 * manual channel, TEC-682 procedure) or "reconciled" (confirmed with the
 * authority): reason, the official reference and an optional receipt.
 */
export function KabisResolveDialog({
  target,
  onClose,
}: {
  target: { uuid: string; action: KabisResolution } | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const resolve = useResolveKabis();
  const [file, setFile] = useState<File | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const [uploaderKey, setUploaderKey] = useState(0);
  const manual = target?.action === "mark-manual";

  const close = () => {
    setFile(null);
    setProblem(null);
    setUploaderKey((k) => k + 1);
    onClose();
  };

  const schema = useMemo(
    () =>
      z.object({
        reason: z
          .string()
          .trim()
          .min(3, t("kabis.validation.reason"))
          .max(500, t("kabis.validation.reason")),
        reference: z
          .string()
          .trim()
          .min(1, t("kabis.validation.reference"))
          .max(120, t("kabis.validation.reference")),
      }),
    [t],
  );

  return (
    <AppDialog
      open={Boolean(target)}
      onOpenChange={(next) => {
        if (!next) close();
      }}
      title={
        manual
          ? t("kabis.resolve.manual_title")
          : t("kabis.resolve.reconcile_title")
      }
      description={
        manual
          ? t("kabis.resolve.manual_description")
          : t("kabis.resolve.reconcile_description")
      }
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={close}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={RESOLVE_FORM}
            disabled={resolve.isPending}
          >
            {manual
              ? t("kabis.actions.mark_manual")
              : t("kabis.actions.reconcile")}
          </Button>
        </>
      }
    >
      {target ? (
        <AppForm<ResolveValues>
          id={RESOLVE_FORM}
          schema={schema}
          defaultValues={{ reason: "", reference: "" }}
          onSubmit={async (values) => {
            setProblem(null);
            try {
              await resolve.mutateAsync({
                uuid: target.uuid,
                action: target.action,
                input: {
                  reason: values.reason.trim(),
                  reference: values.reference.trim(),
                  file,
                },
              });
              appToast.success(
                t(
                  manual
                    ? "kabis.toast.marked_manual"
                    : "kabis.toast.reconciled",
                ),
              );
              close();
            } catch (error) {
              setProblem(kabisProblemText(t, error));
            }
          }}
        >
          <AppInput
            name="reference"
            label={t("kabis.fields.reference")}
            description={t("kabis.fields.reference_hint")}
          />
          <AppTextarea
            name="reason"
            rows={3}
            label={t("kabis.fields.reason")}
            description={t("kabis.fields.reason_hint")}
          />
          <div className="space-y-2">
            <p className="text-sm font-medium">
              {t("kabis.fields.attachment")}
            </p>
            {file ? (
              <p className="text-muted-foreground flex items-center gap-1.5 text-xs">
                <FileText className="size-3.5" />
                {file.name} · {fileSize(file.size)}
              </p>
            ) : null}
            <FileUploader
              key={uploaderKey}
              multiple={false}
              accept={KABIS_ATTACHMENT_ACCEPT}
              maxSizeBytes={KABIS_ATTACHMENT_MAX_BYTES}
              onUpload={(files) => setFile(files[0] ?? null)}
            />
            <p className="text-muted-foreground text-xs">
              {t("kabis.fields.attachment_hint")}
            </p>
          </div>
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
