"use client";

import { useMemo } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "reservation-reason-form";

/**
 * Confirmation dialog with a reason (cancel, no-show, resolve a task…).
 * `required` reasons need 3-500 characters; optional ones may stay empty.
 */
export function ReasonDialog({
  open,
  title,
  description,
  label,
  confirmLabel,
  destructive = false,
  required = true,
  pending = false,
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title: string;
  description?: string;
  label: string;
  confirmLabel: string;
  destructive?: boolean;
  required?: boolean;
  pending?: boolean;
  onConfirm: (reason: string) => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const schema = useMemo(
    () =>
      z.object({
        reason: required
          ? z
              .string()
              .trim()
              .min(3, t("reservations.validation.reason"))
              .max(500, t("reservations.validation.reason"))
          : z.string().max(1000),
      }),
    [required, t],
  );
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !pending) onCancel();
      }}
      title={title}
      description={description}
      className="sm:max-w-md"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={onCancel}
            disabled={pending}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            variant={destructive ? "destructive" : "default"}
            disabled={pending}
          >
            {confirmLabel}
          </Button>
        </>
      }
    >
      {open ? (
        <AppForm<{ reason: string }>
          id={FORM_ID}
          className="space-y-4"
          schema={schema}
          defaultValues={{ reason: "" }}
          onSubmit={async (values) => onConfirm(values.reason.trim())}
        >
          <AppTextarea name="reason" label={label} rows={3} />
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
