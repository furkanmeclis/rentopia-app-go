"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useReconcileIntegrationJob } from "@/features/payments/hooks/use-payments";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "integration-job-reconcile-form";

type Values = { note: string; external_ref: string };

/**
 * "Mutabık işaretle" (TEC-962): the work was confirmed outside the runner
 * (provider panel, delivery log), e.g. sent after the backup a restore came
 * from. The job is never sent again; nothing is sent now. A note is
 * required, the provider's reference is optional.
 */
export function ReconcileJobDialog({
  jobUuid,
  onClose,
}: {
  jobUuid: string | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const reconcile = useReconcileIntegrationJob();
  const [problem, setProblem] = useState<string | null>(null);
  const schema = useMemo(
    () =>
      z.object({
        note: z
          .string()
          .trim()
          .min(3, t("payments.integrations.jobs.reconcile.note_invalid"))
          .max(500, t("payments.integrations.jobs.reconcile.note_invalid")),
        external_ref: z
          .string()
          .trim()
          .max(200, t("payments.integrations.jobs.reconcile.ref_invalid")),
      }),
    [t],
  );
  const close = () => {
    setProblem(null);
    onClose();
  };
  return (
    <AppDialog
      open={Boolean(jobUuid)}
      onOpenChange={(next) => {
        if (!next && !reconcile.isPending) close();
      }}
      title={t("payments.integrations.jobs.reconcile.title")}
      description={t("payments.integrations.jobs.reconcile.description")}
      className="sm:max-w-md"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={close}
            disabled={reconcile.isPending}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={reconcile.isPending}>
            {t("payments.integrations.jobs.reconcile.confirm")}
          </Button>
        </>
      }
    >
      {jobUuid ? (
        <div className="space-y-4">
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AppForm<Values>
            id={FORM_ID}
            className="space-y-4"
            schema={schema}
            defaultValues={{ note: "", external_ref: "" }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await reconcile.mutateAsync({
                  uuid: jobUuid,
                  note: values.note.trim(),
                  external_ref: values.external_ref.trim() || undefined,
                });
                close();
              } catch (error) {
                setProblem(
                  error instanceof ApiError
                    ? error.message
                    : t("payments.integrations.jobs.reconcile.failed"),
                );
              }
            }}
          >
            <AppTextarea
              name="note"
              label={t("payments.integrations.jobs.reconcile.note")}
              placeholder={t(
                "payments.integrations.jobs.reconcile.note_placeholder",
              )}
              rows={3}
            />
            <AppInput
              name="external_ref"
              label={t("payments.integrations.jobs.reconcile.external_ref")}
              description={t(
                "payments.integrations.jobs.reconcile.external_ref_hint",
              )}
            />
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}
