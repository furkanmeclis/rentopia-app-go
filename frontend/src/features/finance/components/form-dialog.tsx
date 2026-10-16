"use client";

import type { ReactNode } from "react";
import type { DefaultValues, FieldValues } from "react-hook-form";
import type { ZodType } from "zod";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

type FinanceFormDialogProps<T extends FieldValues> = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  formId: string;
  title: string;
  description?: string;
  submitLabel: string;
  pending?: boolean;
  problem?: string | null;
  schema: ZodType<T>;
  defaultValues: DefaultValues<T>;
  onSubmit: (values: T) => Promise<void>;
  children: ReactNode;
  destructive?: boolean;
};

/**
 * The dialog shape of every finance action: an `AppForm` in an `AppDialog`,
 * cancel / submit in the footer and an inline error above the fields.
 */
export function FinanceFormDialog<T extends FieldValues>({
  open,
  onOpenChange,
  formId,
  title,
  description,
  submitLabel,
  pending,
  problem,
  schema,
  defaultValues,
  onSubmit,
  children,
  destructive,
}: FinanceFormDialogProps<T>) {
  const { t } = useLocale();
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description={description}
      className="max-h-[90dvh] overflow-y-auto sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
          >
            {t("form.cancel")}
          </Button>
          <Button
            type="submit"
            form={formId}
            disabled={pending}
            variant={destructive ? "destructive" : "default"}
          >
            {submitLabel}
          </Button>
        </>
      }
    >
      {open ? (
        <div className="space-y-4">
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <AppForm<T>
            id={formId}
            className="space-y-4"
            schema={schema}
            defaultValues={defaultValues}
            onSubmit={onSubmit}
          >
            {children}
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}
