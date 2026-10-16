"use client";

import { useCallback, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { customersService } from "@/features/customers/services/customers.service";
import {
  CHANNELS,
  CHANNEL_LABEL_KEYS,
  EVENT_LABEL_KEYS,
} from "@/features/notification-engine/constants";
import {
  useNotificationEvents,
  useSendTest,
} from "@/features/notification-engine/hooks/use-notification-engine";
import { problemText } from "@/features/notification-engine/lib/format";
import type { NotificationDispatch } from "@/features/notification-engine/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

const ALL = "__rule__";

/** Sends one event with sample values; opens the result's attempts. */
export function TestDialog({
  open,
  onOpenChange,
  onSent,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSent: (dispatch: NotificationDispatch) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const events = useNotificationEvents();
  const send = useSendTest();
  const [event, setEvent] = useState("reservation.confirmed");
  const [channel, setChannel] = useState(ALL);
  const [customer, setCustomer] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  const audience =
    events.data?.events.find((e) => e.code === event)?.audience ?? "customer";

  const loadOptions = useCallback(
    async (query: string): Promise<ComboboxOption[]> => {
      try {
        const page = await customersService.list(slug, {
          limit: 20,
          offset: 0,
          q: query || undefined,
          anonymized: "false",
        });
        return page.items.map((c) => ({
          value: c.uuid,
          label: c.display_name,
          description: c.phone_masked ?? undefined,
        }));
      } catch {
        return [];
      }
    },
    [slug],
  );

  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };

  const submit = async () => {
    setProblem(null);
    try {
      const out = await send.mutateAsync({
        event,
        channel:
          channel === ALL ? undefined : (channel as (typeof CHANNELS)[number]),
        customer_uuid:
          audience === "customer" && customer ? customer : undefined,
      });
      close(false);
      onSent(out);
    } catch (error) {
      setProblem(problemText(error, t("notification_engine.errors.test")));
    }
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("notification_engine.test.title")}
      description={t("notification_engine.test.description")}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button
            type="button"
            disabled={send.isPending || (audience === "customer" && !customer)}
            onClick={() => void submit()}
          >
            {t("notification_engine.test.send")}
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
          <FieldLabel htmlFor="test-event">
            {t("notification_engine.test.event")}
          </FieldLabel>
          <Select value={event} onValueChange={setEvent}>
            <SelectTrigger id="test-event" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(events.data?.events ?? []).map((e) => (
                <SelectItem key={e.code} value={e.code}>
                  {t(EVENT_LABEL_KEYS[e.code] ?? e.code)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Field>
          <FieldLabel htmlFor="test-channel">
            {t("notification_engine.test.channel")}
          </FieldLabel>
          <Select value={channel} onValueChange={setChannel}>
            <SelectTrigger id="test-channel" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={ALL}>
                {t("notification_engine.test.channel_rule")}
              </SelectItem>
              {CHANNELS.map((c) => (
                <SelectItem key={c} value={c}>
                  {t(CHANNEL_LABEL_KEYS[c])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        {audience === "customer" ? (
          <Field>
            <FieldLabel htmlFor="test-customer">
              {t("notification_engine.test.customer")}
            </FieldLabel>
            <AsyncCombobox
              id="test-customer"
              value={customer}
              onValueChange={setCustomer}
              loadOptions={loadOptions}
              placeholder={t("notification_engine.test.customer_placeholder")}
              searchPlaceholder={t("notification_engine.test.customer_search")}
              emptyText={t("notification_engine.test.customer_empty")}
              clearable
            />
          </Field>
        ) : (
          <FieldDescription>
            {t("notification_engine.test.staff_help")}
          </FieldDescription>
        )}
      </div>
    </AppDialog>
  );
}
