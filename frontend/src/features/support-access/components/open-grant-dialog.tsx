"use client";

import { useMemo } from "react";
import { useWatch } from "react-hook-form";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppSelect, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  DURATION_LABEL_KEYS,
  SCOPE_HINT_KEYS,
  SCOPE_LABEL_KEYS,
  SUPPORT_DURATIONS,
  SUPPORT_SCOPES,
} from "@/features/support-access/constants";
import { useOpenSupportGrant } from "@/features/support-access/hooks/use-support-access";
import type { SupportGrantScope } from "@/features/support-access/types";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "support-access-open-form";

type Values = {
  scope: SupportGrantScope;
  duration_hours: string;
  note: string;
};

function ScopeHint() {
  const { t } = useLocale();
  const scope = useWatch({ name: "scope" }) as SupportGrantScope | undefined;
  if (!scope) return null;
  return (
    <p className="text-muted-foreground text-sm">{t(SCOPE_HINT_KEYS[scope])}</p>
  );
}

/**
 * Opens a support access grant: scope (read-only / full), duration
 * (1 hour – 7 days) and an optional note for the support team. A new grant
 * replaces the active one.
 */
export function OpenGrantDialog({
  open,
  hasActive,
  onClose,
}: {
  open: boolean;
  hasActive: boolean;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const openGrant = useOpenSupportGrant();
  const schema = useMemo(
    () =>
      z.object({
        scope: z.enum(["read_only", "full"]),
        duration_hours: z
          .string()
          .refine(
            (v) =>
              SUPPORT_DURATIONS.includes(
                Number(v) as (typeof SUPPORT_DURATIONS)[number],
              ),
            t("support_access.validation.duration"),
          ),
        note: z.string().max(500, t("support_access.validation.note")),
      }),
    [t],
  );
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("support_access.open.title")}
      description={t(
        hasActive
          ? "support_access.open.description_replace"
          : "support_access.open.description",
      )}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={openGrant.isPending}>
            {t("support_access.open.submit")}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<Values>
          id={FORM_ID}
          schema={schema}
          defaultValues={{ scope: "read_only", duration_hours: "24", note: "" }}
          onSubmit={async (values) => {
            await openGrant.mutateAsync({
              scope: values.scope,
              duration_hours: Number(values.duration_hours),
              note: values.note.trim(),
            });
            onClose();
          }}
        >
          <div className="space-y-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <AppSelect
                name="scope"
                label={t("support_access.fields.scope")}
                options={SUPPORT_SCOPES.map((s) => ({
                  value: s,
                  label: t(SCOPE_LABEL_KEYS[s]),
                }))}
              />
              <AppSelect
                name="duration_hours"
                label={t("support_access.fields.duration")}
                options={SUPPORT_DURATIONS.map((h) => ({
                  value: String(h),
                  label: t(DURATION_LABEL_KEYS[h]),
                }))}
              />
            </div>
            <ScopeHint />
            <AppTextarea
              name="note"
              rows={3}
              label={t("support_access.fields.note")}
              description={t("support_access.fields.note_hint")}
              placeholder={t("support_access.fields.note_placeholder")}
            />
          </div>
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
