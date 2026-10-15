"use client";

import { useCallback, useMemo, useState, type ReactNode } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppTextarea } from "@/components/forms";
import { Button } from "@/components/ui/button";
import type { BulkActionDef } from "@/features/bulk-engine";
import { MemberPickerField } from "@/features/reservations/components/member-picker";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "reservation-bulk-params";

type Request = {
  action: BulkActionDef;
  resolve: (params: Record<string, string> | null) => void;
};

type Values = { reason: string; user_uuid: string };

/**
 * Params of the reservation bulk actions: `cancel` asks the reason,
 * `assign_staff` the member (empty clears). Plugs into `BulkActionMenu`.
 */
export function useReservationBulkParams(): {
  resolveParams: (
    action: BulkActionDef,
  ) => Promise<Record<string, string> | null | undefined>;
  dialog: ReactNode;
} {
  const { t } = useLocale();
  const [request, setRequest] = useState<Request | null>(null);
  const isCancel = request?.action.id === "cancel";
  const schema = useMemo(
    () =>
      z.object({
        reason: isCancel
          ? z
              .string()
              .trim()
              .min(3, t("reservations.validation.reason"))
              .max(500, t("reservations.validation.reason"))
          : z.string(),
        user_uuid: z.string(),
      }),
    [isCancel, t],
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
      description={
        isCancel
          ? t("reservations.bulk.cancel_hint")
          : t("reservations.bulk.assign_hint")
      }
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => finish(null)}>
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={FORM_ID}
            variant={isCancel ? "destructive" : "default"}
          >
            {t("reservations.bulk.continue")}
          </Button>
        </>
      }
    >
      {request ? (
        <AppForm<Values>
          key={request.action.id}
          id={FORM_ID}
          className="space-y-4"
          schema={schema}
          defaultValues={{ reason: "", user_uuid: "" }}
          onSubmit={async (values) =>
            finish(
              isCancel
                ? { reason: values.reason.trim() }
                : { user_uuid: values.user_uuid },
            )
          }
        >
          {isCancel ? (
            <AppTextarea
              name="reason"
              label={t("reservations.fields.cancel_reason")}
              rows={3}
            />
          ) : (
            <MemberPickerField
              name="user_uuid"
              label={t("reservations.fields.assigned_user")}
              description={t("reservations.assign.clear_hint")}
            />
          )}
        </AppForm>
      ) : null}
    </AppDialog>
  );

  return { resolveParams, dialog };
}
