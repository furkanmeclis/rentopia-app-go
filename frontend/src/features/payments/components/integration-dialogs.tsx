"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppPassword, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { INTEGRATION_FIELD_LABEL_KEYS } from "@/features/payments/constants";
import {
  useConnectIntegration,
  useIntegrationsMeta,
  useUpdateIntegration,
} from "@/features/payments/hooks/use-payments";
import type { IntegrationAccount } from "@/features/payments/types";
import { reasonText } from "@/features/reservations/constants";
import { useLocale } from "@/providers/locale-provider";

const CONNECT_FORM = "integration-connect-form";
const EDIT_FORM = "integration-edit-form";

type ConnectValues = {
  display_name: string;
  sandbox: boolean;
  api_key: string;
  secret_key: string;
};

/**
 * Connect iyzico with the organization's own merchant keys. The keys are
 * sent once and never shown again (the API answers with masked hints).
 */
export function ConnectIntegrationDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const connect = useConnectIntegration();
  const meta = useIntegrationsMeta();
  const provider = meta.data?.providers.find((p) => p.provider === "iyzico");
  const schema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t("payments.integrations.validation.display_name"))
          .max(120),
        sandbox: z.boolean(),
        api_key: z
          .string()
          .trim()
          .min(1, t("payments.integrations.validation.required")),
        secret_key: z
          .string()
          .trim()
          .min(1, t("payments.integrations.validation.required")),
      }),
    [t],
  );
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("payments.integrations.connect_title")}
      description={t("payments.integrations.connect_description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={CONNECT_FORM}
            disabled={connect.isPending}
          >
            {t("payments.integrations.connect")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<ConnectValues>
          id={CONNECT_FORM}
          schema={schema}
          defaultValues={{
            display_name: "iyzico",
            sandbox: true,
            api_key: "",
            secret_key: "",
          }}
          onSubmit={async (values) => {
            await connect.mutateAsync({
              provider: "iyzico",
              display_name: values.display_name.trim(),
              mode: values.sandbox ? "sandbox" : "live",
              credentials: {
                api_key: values.api_key.trim(),
                secret_key: values.secret_key.trim(),
              },
            });
            onClose();
          }}
        >
          <div className="space-y-4">
            <AppInput
              name="display_name"
              label={t("payments.integrations.fields.display_name")}
            />
            <AppSwitch
              name="sandbox"
              label={t("payments.integrations.fields.sandbox")}
              description={t("payments.integrations.fields.sandbox_hint")}
            />
            {(
              provider?.fields ?? [
                { key: "api_key", secret: true },
                { key: "secret_key", secret: true },
              ]
            ).map((f) => (
              <AppPassword
                key={f.key}
                name={f.key}
                label={reasonText(t, INTEGRATION_FIELD_LABEL_KEYS, f.key)}
                autoComplete="off"
              />
            ))}
            <p className="text-muted-foreground text-xs">
              {t("payments.integrations.secrets_note")}
            </p>
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

type EditValues = {
  display_name: string;
  sandbox: boolean;
  api_key: string;
  secret_key: string;
};

/** Rename, switch sandbox / live, rotate keys (empty = keep). */
export function EditIntegrationDialog({
  account,
  onClose,
}: {
  account: IntegrationAccount | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const update = useUpdateIntegration();
  const schema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t("payments.integrations.validation.display_name"))
          .max(120),
        sandbox: z.boolean(),
        api_key: z.string(),
        secret_key: z.string(),
      }),
    [t],
  );
  return (
    <AppDialog
      open={Boolean(account)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("payments.integrations.edit_title")}
      description={t("payments.integrations.edit_description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={EDIT_FORM} disabled={update.isPending}>
            {t("payments.integrations.save")}
          </Button>
        </>
      }
    >
      {account ? (
        <AppForm<EditValues>
          id={EDIT_FORM}
          schema={schema}
          defaultValues={{
            display_name: account.display_name,
            sandbox: account.mode === "sandbox",
            api_key: "",
            secret_key: "",
          }}
          onSubmit={async (values) => {
            const credentials: Record<string, string> = {};
            if (values.api_key.trim())
              credentials.api_key = values.api_key.trim();
            if (values.secret_key.trim())
              credentials.secret_key = values.secret_key.trim();
            await update.mutateAsync({
              uuid: account.uuid,
              body: {
                display_name: values.display_name.trim(),
                mode: values.sandbox ? "sandbox" : "live",
                credentials: Object.keys(credentials).length
                  ? credentials
                  : null,
              },
            });
            onClose();
          }}
        >
          <div className="space-y-4">
            <AppInput
              name="display_name"
              label={t("payments.integrations.fields.display_name")}
            />
            <AppSwitch
              name="sandbox"
              label={t("payments.integrations.fields.sandbox")}
              description={t("payments.integrations.fields.sandbox_hint")}
            />
            {(["api_key", "secret_key"] as const).map((key) => (
              <AppPassword
                key={key}
                name={key}
                label={reasonText(t, INTEGRATION_FIELD_LABEL_KEYS, key)}
                placeholder={account.credentials_hint[key] ?? ""}
                description={t("payments.integrations.rotate_hint")}
                autoComplete="off"
              />
            ))}
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
