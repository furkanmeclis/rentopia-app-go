"use client";

import { useCallback, useState, type ReactNode } from "react";
import { z } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import type { BulkActionDef } from "@/features/bulk-engine";
import { useCategoryOptions } from "@/features/finance/components/cash-dialogs";
import { useLocale } from "@/providers/locale-provider";

const FORM_ID = "finance-bulk-category";

type Request = {
  action: BulkActionDef;
  resolve: (params: Record<string, string> | null) => void;
};

/**
 * Collects the `category_uuid` param of the "set category" bulk action of
 * movements and expenses; `resolveParams` plugs into `BulkActionMenu`.
 */
export function useCategoryBulkParams(): {
  resolveParams: (
    action: BulkActionDef,
  ) => Promise<Record<string, string> | null | undefined>;
  dialog: ReactNode;
} {
  const { t } = useLocale();
  const [request, setRequest] = useState<Request | null>(null);
  const options = useCategoryOptions(Boolean(request), false);
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
      description={t("finance.bulk.category_hint")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => finish(null)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID}>
            {t("finance.bulk.continue")}
          </Button>
        </>
      }
    >
      {request ? (
        <AppForm<{ category_uuid: string }>
          id={FORM_ID}
          className="space-y-4"
          schema={z.object({
            category_uuid: z.string().min(1, t("finance.validation.required")),
          })}
          defaultValues={{ category_uuid: "" }}
          onSubmit={async (v) => finish({ category_uuid: v.category_uuid })}
        >
          <AppSelect
            name="category_uuid"
            label={t("finance.fields.category")}
            placeholder={t("finance.fields.choose_category")}
            options={options}
          />
        </AppForm>
      ) : null}
    </AppDialog>
  );
  return { resolveParams, dialog };
}
