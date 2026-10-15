"use client";

import { AlertTriangle, CheckCircle2, Copy, Eye, Save } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useRef, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EditorX,
  type EditorXContent,
  type EditorXHandle,
} from "@/components/editor/editor-x";
import {
  EntityActions,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { HtmlPreview } from "@/features/contracts/components/html-preview";
import { PlaceholderPicker } from "@/features/contracts/components/placeholder-picker";
import {
  CONTRACT_KINDS,
  KIND_LABEL_KEYS,
  TEMPLATE_STATUS_LABEL_KEYS,
  TEMPLATE_STATUS_TONES,
  placeholderLabel,
} from "@/features/contracts/constants";
import {
  useContractTemplate,
  useCreateTemplate,
  usePreview,
  useTemplateAction,
  useUpdateTemplate,
} from "@/features/contracts/hooks/use-contracts";
import type {
  ContractKind,
  ContractPreview,
  ContractTemplate,
} from "@/features/contracts/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const FORM_ID = "contract-template-form";

type FormValues = { name: string; kind: ContractKind };

/** Serialized editor state of a saved template, when it has one. */
function initialJsonOf(template: ContractTemplate | undefined) {
  const content = template?.content as { root?: unknown } | undefined;
  return content && typeof content === "object" && content.root
    ? JSON.stringify(content)
    : undefined;
}

function problemText(t: (key: string) => string, error: unknown) {
  if (!(error instanceof ApiError)) return String(error);
  const unknown = error.details.find((d) => d.code === "unknown_placeholder");
  if (unknown) return `${t("contracts.problems.unknown")} ${unknown.message}`;
  const code = error.details[0]?.code;
  if (code === "not_draft") return t("contracts.problems.not_draft");
  return error.details.length
    ? error.details.map((d) => d.message).join(" · ")
    : error.message;
}

function Editor({
  template,
  editable,
}: {
  template?: ContractTemplate;
  editable: boolean;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const { slug } = useTenant();
  const { can } = usePermission();
  const editorRef = useRef<EditorXHandle>(null);
  const content = useRef<EditorXContent | null>(null);
  const [dirty, setDirty] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const [preview, setPreview] = useState<ContractPreview | null>(null);
  const [tab, setTab] = useState("editor");
  const [confirm, setConfirm] = useState<"activate" | "new_version" | null>(
    null,
  );
  const create = useCreateTemplate();
  const update = useUpdateTemplate(template?.uuid ?? "");
  const render = usePreview();
  const action = useTemplateAction();
  const initialJson = initialJsonOf(template);
  const legacyHtml = Boolean(template?.body_html) && !initialJson;

  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(2, t("contracts.validation.name"))
          .max(160, t("contracts.validation.name")),
        kind: z.enum(CONTRACT_KINDS as [ContractKind, ...ContractKind[]]),
      }),
    [t],
  );

  const currentHtml = () => content.current?.html ?? template?.body_html ?? "";
  const runPreview = async (kind: ContractKind) => {
    setProblem(null);
    try {
      const out = await render.mutateAsync({ kind, body_html: currentHtml() });
      setPreview(out);
    } catch (error) {
      setProblem(problemText(t, error));
    }
  };

  const save = async (values: FormValues) => {
    setProblem(null);
    const body = {
      name: values.name.trim(),
      ...(content.current
        ? {
            content: JSON.parse(content.current.json) as Record<string, never>,
            body_html: content.current.html,
          }
        : {}),
    };
    try {
      if (template) {
        await update.mutateAsync(body);
        setDirty(false);
      } else {
        const created = await create.mutateAsync({
          ...body,
          kind: values.kind,
          body_html: content.current?.html ?? "",
        });
        router.push(
          routes.tenant.settings.contractTemplates.detail(slug, created.uuid),
        );
      }
    } catch (error) {
      setProblem(problemText(t, error));
    }
  };

  const kind: ContractKind = template?.kind ?? "rental_contract";
  const canActivate =
    template?.status === "draft" &&
    can(permissions.tenant.contracts.activate) &&
    !dirty;
  const canCopy =
    template &&
    template.status !== "draft" &&
    can(permissions.tenant.contracts.write);

  return (
    <div className="space-y-6">
      {template ? (
        <EntityHeader
          title={template.name}
          subtitle={`${t(KIND_LABEL_KEYS[template.kind])} · v${template.version}`}
          badges={
            <>
              <StatusChip
                label={t(TEMPLATE_STATUS_LABEL_KEYS[template.status])}
                tone={TEMPLATE_STATUS_TONES[template.status]}
              />
              {template.used_by > 0 ? (
                <Badge variant="outline">
                  {t("contracts.used_by_badge", { count: template.used_by })}
                </Badge>
              ) : null}
            </>
          }
        >
          <EntityActions>
            {canActivate ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setConfirm("activate")}
              >
                <CheckCircle2 className="size-4" />
                {t("contracts.actions.activate")}
              </Button>
            ) : null}
            {canCopy ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setConfirm("new_version")}
              >
                <Copy className="size-4" />
                {t("contracts.actions.new_version")}
              </Button>
            ) : null}
            {editable ? (
              <Button
                type="submit"
                size="sm"
                form={FORM_ID}
                disabled={create.isPending || update.isPending}
              >
                <Save className="size-4" />
                {t("contracts.actions.save")}
              </Button>
            ) : null}
          </EntityActions>
        </EntityHeader>
      ) : null}

      {!editable && template ? (
        <Alert>
          <AlertTriangle className="size-4" />
          <AlertDescription>{t("contracts.editor.read_only")}</AlertDescription>
        </Alert>
      ) : null}
      {problem ? (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{t("contracts.problems.title")}</AlertTitle>
          <AlertDescription>{problem}</AlertDescription>
        </Alert>
      ) : null}

      <EntitySectionCard title={t("contracts.editor.details")}>
        <AppForm<FormValues>
          id={FORM_ID}
          className="grid gap-4 sm:grid-cols-2"
          schema={schema}
          defaultValues={{ name: template?.name ?? "", kind }}
          onSubmit={save}
        >
          <>
            <AppInput
              name="name"
              label={t("contracts.fields.name")}
              disabled={!editable}
            />
            <AppSelect
              name="kind"
              label={t("contracts.fields.kind")}
              description={
                template ? undefined : t("contracts.fields.kind_hint")
              }
              disabled={Boolean(template)}
              options={CONTRACT_KINDS.map((k) => ({
                value: k,
                label: t(KIND_LABEL_KEYS[k]),
              }))}
            />
            {!template ? (
              <div className="flex justify-end sm:col-span-2">
                <Button type="submit" disabled={create.isPending}>
                  <Save className="size-4" />
                  {t("contracts.actions.save_draft")}
                </Button>
              </div>
            ) : null}
          </>
        </AppForm>
      </EntitySectionCard>

      <Tabs value={tab} onValueChange={setTab} className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <TabsList>
            <TabsTrigger value="editor">
              {t("contracts.editor.tab")}
            </TabsTrigger>
            <TabsTrigger value="preview" onClick={() => void runPreview(kind)}>
              {t("contracts.editor.preview_tab")}
            </TabsTrigger>
          </TabsList>
          <div className="flex items-center gap-2">
            {tab === "editor" ? (
              <PlaceholderPicker
                disabled={!editable}
                onInsert={(token) => editorRef.current?.insertText(token)}
              />
            ) : (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={render.isPending}
                onClick={() => void runPreview(kind)}
              >
                <Eye className="size-4" />
                {t("contracts.editor.refresh_preview")}
              </Button>
            )}
          </div>
        </div>
        <TabsContent value="editor" className="space-y-3">
          {legacyHtml ? (
            <Alert>
              <AlertTriangle className="size-4" />
              <AlertDescription>
                {t("contracts.editor.legacy_html")}
              </AlertDescription>
            </Alert>
          ) : null}
          <EditorX
            key={template?.uuid ?? "new"}
            initialJson={initialJson}
            editable={editable}
            placeholder={t("contracts.editor.placeholder")}
            editorRef={editorRef}
            onContentChange={(next) => {
              content.current = next;
              setDirty(true);
            }}
          />
          <p className="text-muted-foreground text-xs">
            {t("contracts.editor.hint")}
          </p>
        </TabsContent>
        <TabsContent value="preview" className="space-y-3">
          {preview && preview.unknown.length > 0 ? (
            <Alert variant="destructive">
              <AlertTriangle className="size-4" />
              <AlertTitle>{t("contracts.problems.unknown")}</AlertTitle>
              <AlertDescription>
                {preview.unknown.map((k) => `{{${k}}}`).join(", ")}
              </AlertDescription>
            </Alert>
          ) : null}
          {preview && preview.empty.length > 0 ? (
            <Alert>
              <AlertTriangle className="size-4" />
              <AlertDescription>
                {t("contracts.editor.empty_values")}{" "}
                {preview.empty.map((k) => placeholderLabel(t, k)).join(", ")}
              </AlertDescription>
            </Alert>
          ) : null}
          {render.isPending && !preview ? (
            <Loading label={t("common.loading")} />
          ) : null}
          {preview ? (
            <HtmlPreview
              html={preview.html}
              title={t("contracts.editor.preview_tab")}
            />
          ) : null}
          <p className="text-muted-foreground text-xs">
            {t("contracts.editor.preview_hint")}
          </p>
        </TabsContent>
      </Tabs>

      {template ? (
        <ConfirmDialog
          open={Boolean(confirm)}
          title={
            confirm === "activate"
              ? t("contracts.activate.title")
              : t("contracts.new_version.title")
          }
          description={
            confirm === "activate"
              ? t("contracts.activate.description", {
                  name: template.name,
                  version: template.version,
                })
              : t("contracts.new_version.description", { name: template.name })
          }
          confirmLabel={
            confirm === "activate"
              ? t("contracts.actions.activate")
              : t("contracts.actions.new_version")
          }
          isPending={action.isPending}
          onCancel={() => setConfirm(null)}
          onConfirm={() => {
            if (!confirm) return;
            action.mutate(
              { uuid: template.uuid, action: confirm },
              {
                onSuccess: (out) => {
                  const next = confirm;
                  setConfirm(null);
                  if (next === "new_version" && out && "uuid" in out) {
                    router.push(
                      routes.tenant.settings.contractTemplates.detail(
                        slug,
                        out.uuid,
                      ),
                    );
                  }
                },
              },
            );
          }}
        />
      ) : null}
    </div>
  );
}

/**
 * Contract template editor: rich text (Lexical) with a placeholder picker
 * and a preview filled with sample data. Only drafts are edited; an
 * active or archived version is copied into a new draft.
 */
export function ContractTemplateEditorPage({ uuid }: { uuid?: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const query = useContractTemplate(uuid ?? "");
  const template = query.data;
  const title = uuid
    ? (template?.name ?? t("contracts.detail_title"))
    : t("contracts.create_title");
  const write = can(permissions.tenant.contracts.write);

  return (
    <EntityPage
      title={title}
      description={t("contracts.editor.description")}
      permission={
        uuid
          ? permissions.tenant.contracts.read
          : permissions.tenant.contracts.write
      }
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("contracts.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("contracts.title"),
          href: routes.tenant.settings.contractTemplates.root(slug),
        },
        { label: title },
      ]}
    >
      {uuid && query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {uuid && query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("contracts.not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {!uuid ? <Editor editable={write} /> : null}
      {uuid && template ? (
        <Editor
          key={template.uuid}
          template={template}
          editable={write && template.status === "draft"}
        />
      ) : null}
    </EntityPage>
  );
}
