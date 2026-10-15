"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppSwitch } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tollProblemText } from "@/features/tolls/components/toll-decision-dialogs";
import {
  useTollSettings,
  useUpdateTollSettings,
} from "@/features/tolls/hooks/use-tolls";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "toll-settings-form";

type Values = {
  processing_fee: string;
  auto_fetch: boolean;
  lookback_days: string;
};

/**
 * Processing fee (its own charge line next to each toll), automatic fetch
 * and look-back window. The provider account itself is connected in the
 * integrations page.
 */
export function TollSettingsDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const settings = useTollSettings(open);
  const update = useUpdateTollSettings();
  const [problem, setProblem] = useState<string | null>(null);
  const canWrite = can(permissions.tenant.settings.write);
  const schema = useMemo(
    () =>
      z.object({
        processing_fee: z
          .string()
          .trim()
          .regex(/^\d{1,6}([.,]\d{1,2})?$/, t("tolls.validation.fee")),
        auto_fetch: z.boolean(),
        lookback_days: z
          .string()
          .trim()
          .regex(/^\d{1,2}$/, t("tolls.validation.lookback"))
          .refine((v) => Number(v) >= 1 && Number(v) <= 60, {
            message: t("tolls.validation.lookback"),
          }),
      }),
    [t],
  );
  const data = settings.data;
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !update.isPending) onClose();
      }}
      title={t("tolls.settings.title")}
      description={t("tolls.settings.description")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {canWrite ? t("form.cancel") : t("common.close")}
          </Button>
          {canWrite ? (
            <Button type="submit" form={FORM_ID} disabled={update.isPending}>
              {t("form.save")}
            </Button>
          ) : null}
        </>
      }
    >
      {open && data ? (
        <div className="space-y-4">
          <Alert>
            <AlertDescription>
              {data.provider_connected
                ? t("tolls.settings.provider_connected")
                : t("tolls.settings.provider_missing")}{" "}
              {can(permissions.tenant.integrations.manage) ? (
                <Link
                  href={routes.tenant.settings.integrations(slug)}
                  className="font-medium underline underline-offset-4"
                >
                  {t("tolls.settings.open_integrations")}
                </Link>
              ) : null}
            </AlertDescription>
          </Alert>
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AppForm<Values>
            id={FORM_ID}
            className="grid gap-4"
            schema={schema}
            defaultValues={{
              processing_fee: data.processing_fee,
              auto_fetch: data.auto_fetch,
              lookback_days: String(data.lookback_days),
            }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await update.mutateAsync({
                  processing_fee: values.processing_fee
                    .trim()
                    .replace(",", "."),
                  auto_fetch: values.auto_fetch,
                  lookback_days: Number(values.lookback_days),
                });
                onClose();
              } catch (error) {
                setProblem(tollProblemText(t, error));
              }
            }}
          >
            <AppInput
              name="processing_fee"
              inputMode="decimal"
              label={t("tolls.settings.processing_fee")}
              description={t("tolls.settings.processing_fee_hint")}
              disabled={!canWrite}
            />
            <AppSwitch
              name="auto_fetch"
              label={t("tolls.settings.auto_fetch")}
              description={t("tolls.settings.auto_fetch_hint")}
              disabled={!canWrite}
            />
            <AppInput
              name="lookback_days"
              type="number"
              min={1}
              max={60}
              label={t("tolls.settings.lookback_days")}
              description={t("tolls.settings.lookback_days_hint")}
              disabled={!canWrite}
            />
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}
