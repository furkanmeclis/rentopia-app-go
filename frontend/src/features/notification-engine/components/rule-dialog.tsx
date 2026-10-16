"use client";

import { ArrowDown, ArrowUp } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldLabel,
} from "@/components/ui/field";
import { Switch } from "@/components/ui/switch";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  CHANNEL_LABEL_KEYS,
  EVENT_LABEL_KEYS,
  OFFSET_PRESETS,
} from "@/features/notification-engine/constants";
import { useSaveRule } from "@/features/notification-engine/hooks/use-notification-engine";
import {
  offsetLabel,
  problemText,
} from "@/features/notification-engine/lib/format";
import type {
  NotificationChannel,
  NotificationChannelInfo,
  NotificationEvent,
} from "@/features/notification-engine/types";
import { useLocale } from "@/providers/locale-provider";

type Draft = {
  enabled: boolean;
  fallback: boolean;
  /** Every channel in the chosen order; `on` = used by the rule. */
  order: { name: NotificationChannel; on: boolean }[];
  offsets: string[];
};

function draftOf(
  event: NotificationEvent,
  channels: NotificationChannelInfo[],
): Draft {
  const used = event.rule.channels as NotificationChannel[];
  const rest = channels.map((c) => c.name).filter((c) => !used.includes(c));
  return {
    enabled: event.rule.enabled,
    fallback: event.rule.fallback,
    order: [
      ...used.map((name) => ({ name, on: true })),
      ...rest.map((name) => ({ name, on: false })),
    ],
    offsets: event.rule.reminder_offsets.map(String),
  };
}

/** Edits one event's rule: channel order, fallback, reminders. */
export function RuleDialog({
  event,
  channels,
  onOpenChange,
}: {
  event: NotificationEvent | null;
  channels: NotificationChannelInfo[];
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const save = useSaveRule();
  const [draft, setDraft] = useState<Draft | null>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const [forEvent, setForEvent] = useState<string | null>(null);

  if (event && forEvent !== event.code) {
    setForEvent(event.code);
    setDraft(draftOf(event, channels));
    setProblem(null);
  }
  if (!event && forEvent !== null) {
    setForEvent(null);
  }

  const available = new Map(channels.map((c) => [c.name, c.available]));
  const move = (index: number, delta: number) => {
    if (!draft) return;
    const next = [...draft.order];
    const target = index + delta;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target], next[index]];
    setDraft({ ...draft, order: next });
  };
  const offsetOptions = Array.from(
    new Set([...OFFSET_PRESETS, ...(draft?.offsets ?? []).map(Number)]),
  ).sort((a, b) => b - a);

  const submit = async () => {
    if (!event || !draft) return;
    setProblem(null);
    try {
      await save.mutateAsync({
        event: event.code,
        enabled: draft.enabled,
        fallback: draft.fallback,
        channels: draft.order.filter((c) => c.on).map((c) => c.name),
        reminder_offsets: event.reminder
          ? draft.offsets.map(Number).sort((a, b) => b - a)
          : [],
      });
      onOpenChange(false);
    } catch (error) {
      setProblem(problemText(error, t("notification_engine.errors.save")));
    }
  };

  return (
    <AppDialog
      open={Boolean(event)}
      onOpenChange={onOpenChange}
      title={
        event
          ? t(EVENT_LABEL_KEYS[event.code] ?? event.code)
          : t("notification_engine.rules.edit")
      }
      description={t("notification_engine.rules.edit_description")}
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
            type="button"
            disabled={save.isPending}
            onClick={() => void submit()}
          >
            {t("form.save")}
          </Button>
        </>
      }
    >
      {event && draft ? (
        <div className="space-y-5">
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          <Field orientation="horizontal" className="justify-between gap-4">
            <FieldContent>
              <FieldLabel htmlFor="rule-enabled">
                {t("notification_engine.rules.enabled")}
              </FieldLabel>
              <FieldDescription>
                {t("notification_engine.rules.enabled_help")}
              </FieldDescription>
            </FieldContent>
            <Switch
              id="rule-enabled"
              checked={draft.enabled}
              onCheckedChange={(v) => setDraft({ ...draft, enabled: v })}
            />
          </Field>

          <div className="space-y-2">
            <p className="text-sm font-medium">
              {t("notification_engine.rules.channels")}
            </p>
            <p className="text-muted-foreground text-xs">
              {t("notification_engine.rules.channels_help")}
            </p>
            <ul className="divide-y rounded-md border">
              {draft.order.map((c, i) => (
                <li key={c.name} className="flex items-center gap-3 px-3 py-2">
                  <Checkbox
                    id={`rule-channel-${c.name}`}
                    checked={c.on}
                    onCheckedChange={(v) => {
                      const next = [...draft.order];
                      next[i] = { ...c, on: v === true };
                      setDraft({ ...draft, order: next });
                    }}
                  />
                  <label
                    htmlFor={`rule-channel-${c.name}`}
                    className="flex flex-1 items-center gap-2 text-sm"
                  >
                    {t(CHANNEL_LABEL_KEYS[c.name])}
                    {available.get(c.name) === false ? (
                      <Badge variant="outline">
                        {t("notification_engine.channels.not_installed")}
                      </Badge>
                    ) : null}
                  </label>
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("notification_engine.rules.move_up")}
                    disabled={i === 0}
                    onClick={() => move(i, -1)}
                  >
                    <ArrowUp className="size-4" />
                  </Button>
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("notification_engine.rules.move_down")}
                    disabled={i === draft.order.length - 1}
                    onClick={() => move(i, 1)}
                  >
                    <ArrowDown className="size-4" />
                  </Button>
                </li>
              ))}
            </ul>
          </div>

          <Field orientation="horizontal" className="justify-between gap-4">
            <FieldContent>
              <FieldLabel htmlFor="rule-fallback">
                {t("notification_engine.rules.fallback")}
              </FieldLabel>
              <FieldDescription>
                {t("notification_engine.rules.fallback_help")}
              </FieldDescription>
            </FieldContent>
            <Switch
              id="rule-fallback"
              checked={draft.fallback}
              onCheckedChange={(v) => setDraft({ ...draft, fallback: v })}
            />
          </Field>

          {event.reminder ? (
            <div className="space-y-2">
              <p className="text-sm font-medium">
                {t("notification_engine.rules.reminders")}
              </p>
              <p className="text-muted-foreground text-xs">
                {t("notification_engine.rules.reminders_help")}
              </p>
              <ToggleGroup
                type="multiple"
                variant="outline"
                className="flex flex-wrap justify-start gap-2"
                value={draft.offsets}
                onValueChange={(v) =>
                  setDraft({ ...draft, offsets: v.slice(0, 5) })
                }
              >
                {offsetOptions.map((o) => (
                  <ToggleGroupItem key={o} value={String(o)} size="sm">
                    {offsetLabel(t, o)}
                  </ToggleGroupItem>
                ))}
              </ToggleGroup>
            </div>
          ) : null}
        </div>
      ) : null}
    </AppDialog>
  );
}
