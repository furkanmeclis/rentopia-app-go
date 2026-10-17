"use client";

import { useState } from "react";

import { EntitySectionCard } from "@/components/entity";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { problemText } from "@/features/notification-engine/lib/format";
import { RATE_LIMIT_MAX, RATE_LIMIT_MIN } from "@/features/whatsapp/constants";
import { useUpdateWhatsAppSettings } from "@/features/whatsapp/hooks/use-whatsapp";
import type { WhatsAppSession } from "@/features/whatsapp/types";
import { useLocale } from "@/providers/locale-provider";

/** Send limit of the session and the transactional-only policy. */
export function SettingsCard({ session }: { session: WhatsAppSession }) {
  const { t } = useLocale();
  const save = useUpdateWhatsAppSettings();
  const [value, setValue] = useState(String(session.rate_limit_per_minute));
  const [problem, setProblem] = useState<string | null>(null);
  const n = Number(value);
  const valid =
    Number.isInteger(n) && n >= RATE_LIMIT_MIN && n <= RATE_LIMIT_MAX;
  const dirty = n !== session.rate_limit_per_minute;
  const hasSession = session.status !== "not_connected";

  const submit = async () => {
    setProblem(null);
    try {
      await save.mutateAsync({ rate_limit_per_minute: n });
    } catch (error) {
      setProblem(problemText(error, t("whatsapp.errors.settings")));
    }
  };

  return (
    <EntitySectionCard title={t("whatsapp.settings.title")}>
      <div className="space-y-4">
        {problem ? (
          <Alert variant="destructive">
            <AlertDescription>{problem}</AlertDescription>
          </Alert>
        ) : null}
        <Field>
          <FieldLabel htmlFor="whatsapp-rate-limit">
            {t("whatsapp.settings.rate_limit")}
          </FieldLabel>
          <div className="flex flex-col gap-2 sm:flex-row">
            <Input
              id="whatsapp-rate-limit"
              type="number"
              inputMode="numeric"
              min={RATE_LIMIT_MIN}
              max={RATE_LIMIT_MAX}
              className="sm:w-32"
              value={value}
              disabled={!hasSession}
              onChange={(e) => setValue(e.target.value)}
            />
            <Button
              type="button"
              variant="outline"
              disabled={!hasSession || !valid || !dirty || save.isPending}
              onClick={() => void submit()}
            >
              {t("form.save")}
            </Button>
          </div>
          <FieldDescription>
            {t("whatsapp.settings.rate_limit_help", {
              min: RATE_LIMIT_MIN,
              max: RATE_LIMIT_MAX,
            })}
          </FieldDescription>
        </Field>
        <p className="text-muted-foreground text-sm">
          {t("whatsapp.settings.transactional_only")}
        </p>
      </div>
    </EntitySectionCard>
  );
}
