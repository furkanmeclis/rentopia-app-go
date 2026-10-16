"use client";

import { Check, Copy, KeyRound } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";

export type ShownSecret = {
  title: string;
  description: string;
  secret: string;
  /** Shown once: the panel cannot show it again. */
  once: boolean;
  /** Optional embed / usage snippet. */
  snippet?: string;
};

function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <Button
      type="button"
      variant="outline"
      size="icon"
      aria-label={label}
      onClick={() => {
        void navigator.clipboard.writeText(value).then(() => {
          setCopied(true);
          window.setTimeout(() => setCopied(false), 2000);
        });
      }}
    >
      {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
    </Button>
  );
}

/**
 * Shows a new API key or webhook signing secret with a copy button. Secret
 * keys and signing secrets are shown once: the API stores only a hash or
 * the sealed value.
 */
export function SecretDialog({
  shown,
  onClose,
}: {
  shown: ShownSecret | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  return (
    <AppDialog
      open={Boolean(shown)}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={shown?.title ?? ""}
      description={shown?.description}
      className="sm:max-w-xl"
      footer={
        <Button type="button" onClick={onClose}>
          {t("public_api.secret.done")}
        </Button>
      }
    >
      {shown ? (
        <div className="space-y-4">
          {shown.once ? (
            <Alert>
              <KeyRound className="size-4" />
              <AlertTitle>{t("public_api.secret.once_title")}</AlertTitle>
              <AlertDescription>
                {t("public_api.secret.once_description")}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="public-api-secret">
              {t("public_api.secret.value")}
            </Label>
            <div className="flex gap-2">
              <Input
                id="public-api-secret"
                readOnly
                value={shown.secret}
                className="font-mono text-xs"
              />
              <CopyButton
                value={shown.secret}
                label={t("public_api.secret.copy")}
              />
            </div>
          </div>
          {shown.snippet ? (
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="public-api-snippet">
                  {t("public_api.secret.snippet")}
                </Label>
                <CopyButton
                  value={shown.snippet}
                  label={t("public_api.secret.copy")}
                />
              </div>
              <Textarea
                id="public-api-snippet"
                readOnly
                rows={8}
                value={shown.snippet}
                className="font-mono text-xs"
              />
            </div>
          ) : null}
        </div>
      ) : null}
    </AppDialog>
  );
}
