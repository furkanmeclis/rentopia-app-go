"use client";

import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { problemText } from "@/features/notification-engine/lib/format";
import { useSendWhatsAppTest } from "@/features/whatsapp/hooks/use-whatsapp";
import { useLocale } from "@/providers/locale-provider";

/** Sends the fixed test text to a typed number. */
export function TestDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const send = useSendWhatsAppTest();
  const [phone, setPhone] = useState("");
  const [problem, setProblem] = useState<string | null>(null);

  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };

  const submit = async () => {
    setProblem(null);
    try {
      await send.mutateAsync({ phone });
      close(false);
    } catch (error) {
      setProblem(problemText(error, t("whatsapp.errors.test")));
    }
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("whatsapp.test.title")}
      description={t("whatsapp.test.description")}
      className="sm:max-w-md"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={send.isPending || phone.trim().length < 10}
            onClick={() => void submit()}
          >
            {t("whatsapp.test.send")}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {problem ? (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
        <Field>
          <FieldLabel htmlFor="whatsapp-test-phone">
            {t("whatsapp.test.phone")}
          </FieldLabel>
          <Input
            id="whatsapp-test-phone"
            type="tel"
            inputMode="tel"
            autoComplete="off"
            value={phone}
            placeholder="0532 123 45 67"
            onChange={(e) => setPhone(e.target.value)}
          />
          <FieldDescription>{t("whatsapp.test.phone_help")}</FieldDescription>
        </Field>
      </div>
    </AppDialog>
  );
}
