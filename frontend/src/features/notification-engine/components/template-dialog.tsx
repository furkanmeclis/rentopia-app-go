"use client";

import { Eye } from "lucide-react";
import { useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import {
  CHANNEL_LABEL_KEYS,
  EVENT_LABEL_KEYS,
  LANGUAGE_LABEL_KEYS,
} from "@/features/notification-engine/constants";
import { useSaveTemplate } from "@/features/notification-engine/hooks/use-notification-engine";
import { problemText } from "@/features/notification-engine/lib/format";
import { notificationEngineService } from "@/features/notification-engine/services/notification-engine.service";
import type {
  NotificationPreview,
  NotificationTemplate,
} from "@/features/notification-engine/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

function keyOf(t: NotificationTemplate) {
  return `${t.event}|${t.channel}|${t.language}`;
}

/** Edits one event / channel / language template with a live preview. */
export function TemplateDialog({
  template,
  onOpenChange,
}: {
  template: NotificationTemplate | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const save = useSaveTemplate();
  const [forKey, setForKey] = useState<string | null>(null);
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  const [preview, setPreview] = useState<NotificationPreview | null>(null);
  const [previewing, setPreviewing] = useState(false);

  if (template && forKey !== keyOf(template)) {
    setForKey(keyOf(template));
    setSubject(template.subject);
    setBody(template.body);
    setProblem(null);
    setPreview(null);
  }
  if (!template && forKey !== null) {
    setForKey(null);
  }

  const insert = (name: string) => setBody((b) => `${b}{{${name}}}`);

  const runPreview = async () => {
    if (!template) return;
    setPreviewing(true);
    try {
      setPreview(
        await notificationEngineService.preview(slug, {
          event: template.event,
          channel: template.channel,
          language: template.language,
          subject,
          body,
        }),
      );
    } catch (error) {
      setProblem(problemText(error, t("notification_engine.errors.preview")));
    } finally {
      setPreviewing(false);
    }
  };

  const submit = async () => {
    if (!template) return;
    setProblem(null);
    try {
      await save.mutateAsync({
        event: template.event,
        channel: template.channel,
        language: template.language,
        subject,
        body,
      });
      onOpenChange(false);
    } catch (error) {
      setProblem(problemText(error, t("notification_engine.errors.save")));
    }
  };

  const showSubject =
    template?.channel === "email" || template?.channel === "push";

  return (
    <AppDialog
      open={Boolean(template)}
      onOpenChange={onOpenChange}
      title={
        template
          ? `${t(EVENT_LABEL_KEYS[template.event] ?? template.event)} · ${t(
              CHANNEL_LABEL_KEYS[template.channel],
            )} · ${t(LANGUAGE_LABEL_KEYS[template.language])}`
          : t("notification_engine.templates.edit")
      }
      description={t("notification_engine.templates.edit_description")}
      className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            disabled={previewing}
            onClick={() => void runPreview()}
          >
            <Eye className="size-4" />
            {t("notification_engine.templates.preview")}
          </Button>
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
      {template ? (
        <div className="space-y-4">
          {problem ? (
            <Alert variant="destructive">
              <AlertDescription>{problem}</AlertDescription>
            </Alert>
          ) : null}
          {showSubject ? (
            <Field>
              <FieldLabel htmlFor="template-subject">
                {template.channel === "push"
                  ? t("notification_engine.templates.title_field")
                  : t("notification_engine.templates.subject")}
              </FieldLabel>
              <Input
                id="template-subject"
                value={subject}
                maxLength={200}
                onChange={(e) => setSubject(e.target.value)}
              />
            </Field>
          ) : null}
          <Field>
            <FieldLabel htmlFor="template-body">
              {t("notification_engine.templates.body")}
            </FieldLabel>
            <Textarea
              id="template-body"
              rows={6}
              value={body}
              maxLength={template.channel === "sms" ? 900 : 2000}
              onChange={(e) => setBody(e.target.value)}
            />
            <FieldDescription>
              {t("notification_engine.templates.body_help", {
                count: body.length,
              })}
            </FieldDescription>
          </Field>
          <div className="space-y-2">
            <p className="text-sm font-medium">
              {t("notification_engine.templates.variables")}
            </p>
            <div className="flex flex-wrap gap-1.5">
              {template.variables.map((v) => (
                <Button
                  key={v}
                  type="button"
                  size="sm"
                  variant="outline"
                  className="font-mono text-xs"
                  onClick={() => insert(v)}
                >
                  {`{{${v}}}`}
                </Button>
              ))}
            </div>
            <p className="text-muted-foreground text-xs">
              {t("notification_engine.templates.privacy")}
            </p>
          </div>
          {preview ? (
            <div className="bg-muted/40 space-y-2 rounded-md border p-3">
              <p className="text-muted-foreground text-xs font-medium uppercase">
                {t("notification_engine.templates.preview")}
              </p>
              {showSubject && preview.subject ? (
                <p className="text-sm font-semibold">{preview.subject}</p>
              ) : null}
              <p className="text-sm whitespace-pre-wrap">{preview.body}</p>
              {preview.unknown.length ? (
                <div className="flex flex-wrap items-center gap-1">
                  <span className="text-destructive text-xs">
                    {t("notification_engine.templates.unknown_variables")}
                  </span>
                  {preview.unknown.map((u) => (
                    <Badge key={u} variant="danger">
                      {u}
                    </Badge>
                  ))}
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </AppDialog>
  );
}
