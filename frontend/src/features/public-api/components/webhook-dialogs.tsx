"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSwitch,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  EVENT_LABEL_KEYS,
  WEBHOOK_EVENTS,
} from "@/features/public-api/constants";
import {
  useCreatePublicWebhook,
  useUpdatePublicWebhook,
} from "@/features/public-api/hooks/use-public-api";
import type {
  IssuedPublicWebhook,
  PublicWebhook,
  PublicWebhookEvent,
} from "@/features/public-api/types";
import { useLocale } from "@/providers/locale-provider";

const WEBHOOK_FORM = "public-webhook-form";

type WebhookValues = {
  name: string;
  url: string;
  events: string[];
  active: boolean;
};

/**
 * Add a webhook endpoint (`webhook` = null) or edit one: name, https URL,
 * subscribed reservation events and, when editing, whether it is active.
 */
export function WebhookFormDialog({
  open,
  webhook,
  onClose,
  onIssued,
}: {
  open: boolean;
  webhook: PublicWebhook | null;
  onClose: () => void;
  onIssued: (issued: IssuedPublicWebhook) => void;
}) {
  const { t } = useLocale();
  const create = useCreatePublicWebhook();
  const update = useUpdatePublicWebhook();
  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t("public_api.validation.name"))
          .max(120),
        url: z
          .string()
          .trim()
          .regex(/^https?:\/\/\S+$/i, t("public_api.validation.url"))
          .max(2048),
        events: z.array(z.string()).min(1, t("public_api.validation.events")),
        active: z.boolean(),
      }),
    [t],
  );
  const pending = create.isPending || update.isPending;
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t(
        webhook
          ? "public_api.webhooks.edit_title"
          : "public_api.webhooks.create_title",
      )}
      description={t("public_api.webhooks.form_description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={WEBHOOK_FORM} disabled={pending}>
            {t(webhook ? "form.save" : "public_api.webhooks.create")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<WebhookValues>
          key={webhook?.uuid ?? "new"}
          id={WEBHOOK_FORM}
          schema={schema}
          defaultValues={{
            name: webhook?.name ?? "",
            url: webhook?.url ?? "https://",
            events: webhook?.events ?? [...WEBHOOK_EVENTS],
            active: (webhook?.status ?? "active") === "active",
          }}
          onSubmit={async (values) => {
            const body = {
              name: values.name.trim(),
              url: values.url.trim(),
              events: values.events as PublicWebhookEvent[],
            };
            if (webhook) {
              await update.mutateAsync({
                uuid: webhook.uuid,
                body: {
                  ...body,
                  status: values.active ? "active" : "disabled",
                },
              });
              onClose();
              return;
            }
            const issued = await create.mutateAsync(body);
            onClose();
            onIssued(issued);
          }}
        >
          <div className="space-y-4">
            <AppInput
              name="name"
              label={t("public_api.webhooks.fields.name")}
            />
            <AppInput
              name="url"
              type="url"
              label={t("public_api.webhooks.fields.url")}
              description={t("public_api.webhooks.fields.url_hint")}
            />
            <AppMultiCombobox
              name="events"
              label={t("public_api.webhooks.fields.events")}
              options={WEBHOOK_EVENTS.map((e) => ({
                value: e,
                label: t(EVENT_LABEL_KEYS[e]!),
                hint: e,
              }))}
            />
            {webhook ? (
              <AppSwitch
                name="active"
                label={t("public_api.webhooks.fields.active")}
              />
            ) : null}
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
