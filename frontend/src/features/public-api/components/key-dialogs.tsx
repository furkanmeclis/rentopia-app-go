"use client";

import { useMemo, useState } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AppForm,
  AppInput,
  AppMultiCombobox,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
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
  CHANNELS,
  CHANNEL_LABEL_KEYS,
  KEY_KINDS,
  KEY_KIND_LABEL_KEYS,
  SCOPES,
  SCOPE_LABEL_KEYS,
} from "@/features/public-api/constants";
import {
  useCreatePublicApiKey,
  useRotatePublicApiKey,
  useUpdatePublicApiKey,
} from "@/features/public-api/hooks/use-public-api";
import type {
  IssuedPublicApiKey,
  PublicApiChannel,
  PublicApiKey,
  PublicApiKeyKind,
  PublicApiScope,
} from "@/features/public-api/types";
import { useLocale } from "@/providers/locale-provider";

const KEY_FORM = "public-api-key-form";
const GRACE_OPTIONS = [0, 1, 24, 72, 168];

type KeyValues = {
  kind: PublicApiKeyKind;
  name: string;
  channel: PublicApiChannel;
  scopes: string[];
  origins: string;
  rate_limit_per_minute: string;
};

function originList(raw: string): string[] {
  return raw
    .split(/[\s,]+/)
    .map((v) => v.trim())
    .filter(Boolean);
}

function OriginsField() {
  const { t } = useLocale();
  const kind = useWatch({ name: "kind" }) as PublicApiKeyKind;
  if (kind !== "publishable") return null;
  return (
    <AppTextarea
      name="origins"
      rows={3}
      label={t("public_api.keys.fields.allowed_origins")}
      description={t("public_api.keys.fields.allowed_origins_hint")}
      placeholder="https://www.ornek-rentacar.com.tr"
    />
  );
}

/**
 * Issue a key (`key` = null) or edit an active one: name, scopes, allowed
 * origins (publishable keys) and the per-minute rate limit. Kind and sales
 * channel are fixed once issued.
 */
export function KeyFormDialog({
  open,
  apiKey,
  onClose,
  onIssued,
}: {
  open: boolean;
  apiKey: PublicApiKey | null;
  onClose: () => void;
  onIssued: (issued: IssuedPublicApiKey) => void;
}) {
  const { t } = useLocale();
  const create = useCreatePublicApiKey();
  const update = useUpdatePublicApiKey();
  const editing = Boolean(apiKey);
  const schema = useMemo(
    () =>
      z
        .object({
          kind: z.enum(["secret", "publishable"]),
          name: z
            .string()
            .trim()
            .min(1, t("public_api.validation.name"))
            .max(120),
          channel: z.enum(["web", "widget", "api"]),
          scopes: z.array(z.string()).min(1, t("public_api.validation.scopes")),
          origins: z.string(),
          rate_limit_per_minute: z
            .string()
            .trim()
            .refine((v) => {
              const n = Number(v);
              return Number.isInteger(n) && n >= 10 && n <= 6000;
            }, t("public_api.validation.rate_limit")),
        })
        .superRefine((v, ctx) => {
          if (v.kind === "publishable" && originList(v.origins).length === 0) {
            ctx.addIssue({
              code: "custom",
              path: ["origins"],
              message: t("public_api.validation.origins"),
            });
          }
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
        editing ? "public_api.keys.edit_title" : "public_api.keys.create_title",
      )}
      description={t("public_api.keys.form_description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={KEY_FORM} disabled={pending}>
            {t(editing ? "form.save" : "public_api.keys.create")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<KeyValues>
          key={apiKey?.uuid ?? "new"}
          id={KEY_FORM}
          schema={schema}
          defaultValues={{
            kind: apiKey?.kind ?? "secret",
            name: apiKey?.name ?? "",
            channel: apiKey?.channel ?? "api",
            scopes: apiKey?.scopes ?? [...SCOPES],
            origins: (apiKey?.allowed_origins ?? []).join("\n"),
            rate_limit_per_minute: String(apiKey?.rate_limit_per_minute ?? 120),
          }}
          onSubmit={async (values) => {
            const origins =
              values.kind === "publishable" ? originList(values.origins) : [];
            const scopes = values.scopes as PublicApiScope[];
            const rate = Number(values.rate_limit_per_minute);
            if (apiKey) {
              await update.mutateAsync({
                uuid: apiKey.uuid,
                body: {
                  name: values.name.trim(),
                  scopes,
                  allowed_origins: origins,
                  rate_limit_per_minute: rate,
                },
              });
              onClose();
              return;
            }
            const issued = await create.mutateAsync({
              kind: values.kind,
              name: values.name.trim(),
              channel: values.channel,
              scopes,
              allowed_origins: origins,
              rate_limit_per_minute: rate,
            });
            onClose();
            onIssued(issued);
          }}
        >
          <div className="space-y-4">
            {editing ? null : (
              <div className="grid gap-4 sm:grid-cols-2">
                <AppSelect
                  name="kind"
                  label={t("public_api.keys.fields.kind")}
                  options={KEY_KINDS.map((k) => ({
                    value: k,
                    label: t(KEY_KIND_LABEL_KEYS[k]),
                  }))}
                />
                <AppSelect
                  name="channel"
                  label={t("public_api.keys.fields.channel")}
                  description={t("public_api.keys.fields.channel_hint")}
                  options={CHANNELS.map((c) => ({
                    value: c,
                    label: t(CHANNEL_LABEL_KEYS[c]),
                  }))}
                />
              </div>
            )}
            <AppInput name="name" label={t("public_api.keys.fields.name")} />
            <AppMultiCombobox
              name="scopes"
              label={t("public_api.keys.fields.scopes")}
              options={SCOPES.map((s) => ({
                value: s,
                label: t(SCOPE_LABEL_KEYS[s]),
                hint: s,
              }))}
            />
            <OriginsField />
            <AppInput
              name="rate_limit_per_minute"
              type="number"
              inputMode="numeric"
              min={10}
              max={6000}
              label={t("public_api.keys.fields.rate_limit")}
              description={t("public_api.keys.fields.rate_limit_hint")}
            />
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}

/**
 * Rotate a key: a successor with the same settings is issued; the old key
 * keeps working for the chosen grace period (0 = revoked now).
 */
export function RotateKeyDialog({
  apiKey,
  onClose,
  onIssued,
}: {
  apiKey: PublicApiKey | null;
  onClose: () => void;
  onIssued: (issued: IssuedPublicApiKey) => void;
}) {
  const { t } = useLocale();
  const rotate = useRotatePublicApiKey();
  const [grace, setGrace] = useState("24");
  return (
    <AppDialog
      open={Boolean(apiKey)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("public_api.keys.rotate_title")}
      description={t("public_api.keys.rotate_description")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={rotate.isPending}
            onClick={() => {
              if (!apiKey) return;
              rotate.mutate(
                { uuid: apiKey.uuid, graceHours: Number(grace) },
                {
                  onSuccess: (issued) => {
                    onClose();
                    onIssued(issued);
                  },
                },
              );
            }}
          >
            {t("public_api.keys.actions.rotate")}
          </Button>
        </>
      }
    >
      <div className="space-y-2">
        <Label htmlFor="public-api-grace">
          {t("public_api.keys.fields.grace")}
        </Label>
        <Select value={grace} onValueChange={setGrace}>
          <SelectTrigger id="public-api-grace" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {GRACE_OPTIONS.map((h) => (
              <SelectItem key={h} value={String(h)}>
                {h === 0
                  ? t("public_api.keys.grace_now")
                  : t("public_api.keys.grace_hours", { count: h })}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </AppDialog>
  );
}
