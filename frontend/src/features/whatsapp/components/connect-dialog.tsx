"use client";

import { ShieldAlert } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { problemText } from "@/features/notification-engine/lib/format";
import { PROVIDER_LABEL_KEYS } from "@/features/whatsapp/constants";
import { useConnectWhatsApp } from "@/features/whatsapp/hooks/use-whatsapp";
import type { WhatsAppSession } from "@/features/whatsapp/types";
import { useLocale } from "@/providers/locale-provider";

/**
 * "WhatsApp bağla": the number risk notice (must be accepted) and, for a
 * first connection with more than one usable provider, the provider.
 */
export function ConnectDialog({
  open,
  onOpenChange,
  session,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  session: WhatsAppSession | undefined;
}) {
  const { t } = useLocale();
  const connect = useConnectWhatsApp();
  const usable = (session?.providers ?? []).filter((p) => p.configured);
  const firstConnection = !session?.account_uuid;
  const [provider, setProvider] = useState("");
  const [ack, setAck] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const chosen = provider || usable[0]?.code || "";

  const close = (next: boolean) => {
    if (!next) {
      setProblem(null);
      setAck(false);
    }
    onOpenChange(next);
  };

  const submit = async () => {
    setProblem(null);
    try {
      await connect.mutateAsync({
        acknowledge_risk: true,
        provider: firstConnection ? chosen : undefined,
      });
      close(false);
    } catch (error) {
      setProblem(problemText(error, t("whatsapp.errors.connect")));
    }
  };

  const unavailable = firstConnection && usable.length === 0;

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("whatsapp.connect.title")}
      description={t("whatsapp.connect.description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!ack || connect.isPending || unavailable}
            onClick={() => void submit()}
          >
            {t("whatsapp.connect.submit")}
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
        {unavailable ? (
          <Alert variant="destructive">
            <AlertDescription>
              {t("whatsapp.connect.unavailable")}
            </AlertDescription>
          </Alert>
        ) : null}
        <Alert>
          <ShieldAlert />
          <AlertTitle>{t("whatsapp.risk.title")}</AlertTitle>
          <AlertDescription>
            <ul className="list-disc space-y-1 pl-4">
              <li>{t("whatsapp.risk.unofficial")}</li>
              <li>{t("whatsapp.risk.ban")}</li>
              <li>{t("whatsapp.risk.transactional")}</li>
              <li>{t("whatsapp.risk.number")}</li>
            </ul>
          </AlertDescription>
        </Alert>
        {firstConnection && usable.length > 1 ? (
          <Field>
            <FieldLabel htmlFor="whatsapp-provider">
              {t("whatsapp.connect.provider")}
            </FieldLabel>
            <Select value={chosen} onValueChange={setProvider}>
              <SelectTrigger id="whatsapp-provider" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {usable.map((p) => (
                  <SelectItem key={p.code} value={p.code}>
                    {t(PROVIDER_LABEL_KEYS[p.code] ?? p.code)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FieldDescription>
              {t("whatsapp.connect.provider_help")}
            </FieldDescription>
          </Field>
        ) : null}
        <Field orientation="horizontal">
          <Checkbox
            id="whatsapp-ack"
            checked={ack}
            onCheckedChange={(v) => setAck(v === true)}
          />
          <FieldLabel htmlFor="whatsapp-ack" className="font-normal">
            {t("whatsapp.connect.acknowledge")}
          </FieldLabel>
        </Field>
      </div>
    </AppDialog>
  );
}
