"use client";

import { useMemo, useState } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppInput, AppPassword, AppSwitch } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  INTEGRATION_FIELD_LABEL_KEYS,
  INTEGRATION_PROVIDER_HINT_KEYS,
  INTEGRATION_PROVIDER_LABEL_KEYS,
} from "@/features/payments/constants";
import {
  useConnectIntegration,
  useIntegrationsMeta,
  useTestSms,
  useUpdateIntegration,
} from "@/features/payments/hooks/use-payments";
import type {
  IntegrationAccount,
  IntegrationProvider,
  IntegrationProviderCode,
} from "@/features/payments/types";
import { reasonText } from "@/features/reservations/constants";
import { useLocale } from "@/providers/locale-provider";

const CONNECT_FORM = "integration-connect-form";
const EDIT_FORM = "integration-edit-form";
const TEST_SMS_FORM = "integration-test-sms-form";

type Field = IntegrationProvider["fields"][number];

/** iyzico's fields until the meta call answers. */
const FALLBACK_FIELDS: Field[] = [
  {
    key: "api_key",
    label_key: "payments.integrations.fields.api_key",
    secret: true,
    required: true,
  },
  {
    key: "secret_key",
    label_key: "payments.integrations.fields.secret_key",
    secret: true,
    required: true,
  },
];

function CredentialInput({
  field,
  placeholder,
  description,
}: {
  field: Field;
  placeholder?: string;
  description?: string;
}) {
  const { t } = useLocale();
  const label = reasonText(t, INTEGRATION_FIELD_LABEL_KEYS, field.key);
  return field.secret ? (
    <AppPassword
      name={`credentials.${field.key}`}
      label={label}
      placeholder={placeholder}
      description={description}
      autoComplete="off"
    />
  ) : (
    <AppInput
      name={`credentials.${field.key}`}
      label={label}
      placeholder={placeholder}
      description={description}
      autoComplete="off"
    />
  );
}

type ConnectValues = {
  display_name: string;
  sandbox: boolean;
  credentials: Record<string, string>;
};

/**
 * Connect a provider with the organization's own credentials: iyzico
 * (payments) or Netgsm (SMS). The secrets are sent once and never shown
 * again (the API answers with masked hints).
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
  const providers = meta.data?.providers ?? [];
  const [providerCode, setProviderCode] =
    useState<IntegrationProviderCode>("iyzico");
  const provider = providers.find((p) => p.provider === providerCode);
  const fields = useMemo(
    () =>
      provider?.fields ?? (providerCode === "iyzico" ? FALLBACK_FIELDS : []),
    [provider, providerCode],
  );
  const schema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t("payments.integrations.validation.display_name"))
          .max(120),
        sandbox: z.boolean(),
        credentials: z.object(
          Object.fromEntries(
            fields.map((f) => [
              f.key,
              f.required
                ? z
                    .string()
                    .trim()
                    .min(1, t("payments.integrations.validation.required"))
                : z.string(),
            ]),
          ),
        ),
      }),
    [fields, t],
  );
  const providerOptions = (
    providers.length
      ? providers.map((p) => p.provider)
      : (["iyzico"] as IntegrationProviderCode[])
  ).map((code) => ({
    value: code,
    label: reasonText(t, INTEGRATION_PROVIDER_LABEL_KEYS, code),
  }));
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
        <div className="space-y-4">
          <ProviderPicker
            value={providerCode}
            options={providerOptions}
            onChange={setProviderCode}
          />
          <AppForm<ConnectValues>
            key={providerCode}
            id={CONNECT_FORM}
            schema={schema}
            defaultValues={{
              display_name: reasonText(
                t,
                INTEGRATION_PROVIDER_LABEL_KEYS,
                providerCode,
              ),
              sandbox: true,
              credentials: Object.fromEntries(fields.map((f) => [f.key, ""])),
            }}
            onSubmit={async (values) => {
              const credentials: Record<string, string> = {};
              for (const f of fields) {
                const v = (values.credentials[f.key] ?? "").trim();
                if (v) credentials[f.key] = v;
              }
              await connect.mutateAsync({
                provider: providerCode,
                kind: provider?.kind,
                display_name: values.display_name.trim(),
                mode: values.sandbox ? "sandbox" : "live",
                credentials,
              });
              onClose();
            }}
          >
            <div className="space-y-4">
              <p className="text-muted-foreground text-sm">
                {reasonText(t, INTEGRATION_PROVIDER_HINT_KEYS, providerCode)}
              </p>
              <AppInput
                name="display_name"
                label={t("payments.integrations.fields.display_name")}
              />
              <AppSwitch
                name="sandbox"
                label={t("payments.integrations.fields.sandbox")}
                description={t("payments.integrations.fields.sandbox_hint")}
              />
              {fields.map((f) => (
                <CredentialInput key={f.key} field={f} />
              ))}
              <p className="text-muted-foreground text-xs">
                {t("payments.integrations.secrets_note")}
              </p>
            </div>
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}

/** Provider choice outside the credential form (the form remounts per provider). */
function ProviderPicker({
  value,
  options,
  onChange,
}: {
  value: IntegrationProviderCode;
  options: { value: string; label: string }[];
  onChange: (value: IntegrationProviderCode) => void;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-2">
      <Label htmlFor="integration-provider">
        {t("payments.integrations.fields.provider")}
      </Label>
      <Select
        value={value}
        onValueChange={(next) => onChange(next as IntegrationProviderCode)}
      >
        <SelectTrigger id="integration-provider" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

type EditValues = {
  display_name: string;
  sandbox: boolean;
  credentials: Record<string, string>;
};

/** Rename, switch sandbox / live, rotate secrets (empty = keep). */
export function EditIntegrationDialog({
  account,
  onClose,
}: {
  account: IntegrationAccount | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const update = useUpdateIntegration();
  const meta = useIntegrationsMeta();
  const fields = useMemo(
    () =>
      meta.data?.providers.find((p) => p.provider === account?.provider)
        ?.fields ?? (account?.provider === "iyzico" ? FALLBACK_FIELDS : []),
    [account?.provider, meta.data],
  );
  const schema = useMemo(
    () =>
      z.object({
        display_name: z
          .string()
          .trim()
          .min(1, t("payments.integrations.validation.display_name"))
          .max(120),
        sandbox: z.boolean(),
        credentials: z.object(
          Object.fromEntries(fields.map((f) => [f.key, z.string()])),
        ),
      }),
    [fields, t],
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
            credentials: Object.fromEntries(fields.map((f) => [f.key, ""])),
          }}
          onSubmit={async (values) => {
            const credentials: Record<string, string> = {};
            for (const f of fields) {
              const v = (values.credentials[f.key] ?? "").trim();
              if (v) credentials[f.key] = v;
            }
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
            {fields.map((f) => (
              <CredentialInput
                key={f.key}
                field={f}
                placeholder={account.credentials_hint[f.key] ?? ""}
                description={t("payments.integrations.rotate_hint")}
              />
            ))}
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/**
 * Send a test SMS through an SMS account (development accounts go to the
 * Netgsm fake or the log provider). The number is shown masked afterwards.
 */
export function TestSmsDialog({
  account,
  onClose,
}: {
  account: IntegrationAccount | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const send = useTestSms();
  const schema = useMemo(
    () =>
      z.object({
        phone: z
          .string()
          .trim()
          .min(7, t("payments.integrations.sms.validation.phone"))
          .max(32),
      }),
    [t],
  );
  return (
    <AppDialog
      open={Boolean(account)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("payments.integrations.sms.test_title")}
      description={t("payments.integrations.sms.test_description")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={TEST_SMS_FORM} disabled={send.isPending}>
            {t("payments.integrations.sms.send")}
          </Button>
        </>
      }
    >
      {account ? (
        <AppForm<{ phone: string }>
          id={TEST_SMS_FORM}
          schema={schema}
          defaultValues={{ phone: "" }}
          onSubmit={async (values) => {
            await send.mutateAsync({
              uuid: account.uuid,
              phone: values.phone.trim(),
            });
            onClose();
          }}
        >
          <AppInput
            name="phone"
            type="tel"
            label={t("payments.integrations.sms.phone")}
            placeholder="05xx xxx xx xx"
            description={t("payments.integrations.sms.phone_hint")}
            autoComplete="off"
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
