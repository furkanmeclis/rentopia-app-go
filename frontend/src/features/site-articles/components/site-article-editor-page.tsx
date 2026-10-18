"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { ArrowLeft, EyeOff, Trash2, Upload } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  FormProvider,
  useForm,
  useWatch,
  type FieldErrors,
  type UseFormReturn,
} from "react-hook-form";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import {
  AppInput,
  AppSwitch,
  AppTextarea,
  FormActions,
  FormFieldShell,
  FormSection,
} from "@/components/forms";
import { RichTextEditor } from "@/components/plate";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  SlugField,
  SortField,
  TagsField,
} from "@/features/site-articles/components/article-fields";
import {
  useCreateSiteArticle,
  useDeleteSiteArticle,
  useSetSiteArticleStatus,
  useSiteArticle,
  useUpdateSiteArticle,
} from "@/features/site-articles/hooks/use-site-articles";
import {
  STATUS_LABEL_KEYS,
  STATUS_TONES,
  articleTitle,
} from "@/features/site-articles/lib/article-display";
import {
  ARTICLE_FIELD_LIMITS,
  ARTICLE_LOCALES,
  articleBodies,
  articleToForm,
  bodyFingerprint,
  bodyProblemFromField,
  bodyProblemFromIssue,
  createArticleSchema,
  emptyArticleForm,
  formToInput,
  slugify,
  type ArticleBodies,
  type ArticleFormValues,
  type BodyProblem,
} from "@/features/site-articles/lib/article-form";
import type {
  ArticleLocale,
  SiteArticle,
} from "@/features/site-articles/types";
import {
  MediaField,
  MediaPickerDialog,
} from "@/features/site/components/site-media";
import { LengthHint } from "@/features/site/components/site-fields";
import { useSiteMedia } from "@/features/site/hooks/use-site-media";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { isApiError, type ApiError } from "@/lib/api";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type T = (key: string, vars?: Record<string, string | number>) => string;

/** Localized message of an API validation detail code. */
function detailMessage(t: T, code: string | undefined, fallback?: string) {
  const known = [
    "required",
    "invalid",
    "too_long",
    "plain_text",
    "unknown",
    "too_many",
    "too_deep",
    "too_large",
    "not_found",
    "taken",
  ];
  return code && known.includes(code)
    ? t(`site.articles.errors.codes.${code}`)
    : fallback || t("site.articles.errors.codes.invalid");
}

function hasErrorUnder(errors: FieldErrors, locale: ArticleLocale): boolean {
  return ["slug", "title", "summary", "seo_title", "seo_description"].some(
    (key) =>
      Boolean(
        (errors as Record<string, Record<string, unknown>>)[key]?.[locale],
      ),
  );
}

/** Problems of one locale's body (client check or API paths). */
function BodyProblems({ problems }: { problems: BodyProblem[] }) {
  const { t } = useLocale();
  if (!problems.length) return null;
  return (
    <Alert variant="destructive">
      <AlertTitle>{t("site.articles.body_errors.title")}</AlertTitle>
      <AlertDescription>
        <ul className="list-disc space-y-0.5 ps-4">
          {problems.slice(0, 12).map((p, i) => {
            const where = p.block
              ? t("site.articles.body_errors.block", { n: p.block })
              : t("site.articles.body_errors.document");
            const what =
              p.key === "href"
                ? t("site.articles.body_errors.link")
                : p.key === "media_uuid"
                  ? p.code === "not_found"
                    ? t("site.articles.body_errors.image_missing")
                    : t("site.articles.body_errors.image")
                  : p.key === "alt"
                    ? t("site.articles.body_errors.alt")
                    : detailMessage(t, p.code);
            return (
              <li key={i}>
                {where}: {what}
              </li>
            );
          })}
        </ul>
        {problems.length > 12 ? (
          <p className="mt-1">
            {t("site.articles.body_errors.more", {
              count: problems.length - 12,
            })}
          </p>
        ) : null}
      </AlertDescription>
    </Alert>
  );
}

/** Keeps a locale's slug on the title while its auto mode is on. */
function AutoSlug({
  locale,
  auto,
  form,
}: {
  locale: ArticleLocale;
  auto: boolean;
  form: UseFormReturn<ArticleFormValues>;
}) {
  const title = useWatch({ control: form.control, name: `title.${locale}` });
  useEffect(() => {
    if (!auto) return;
    const next = slugify(title ?? "");
    if (form.getValues(`slug.${locale}`) !== next) {
      form.setValue(`slug.${locale}`, next, { shouldDirty: true });
    }
  }, [auto, form, locale, title]);
  return null;
}

function LocalePanel({
  locale,
  form,
  disabled,
  auto,
  setAuto,
  initialBody,
  onBodyChange,
  problems,
}: {
  locale: ArticleLocale;
  form: UseFormReturn<ArticleFormValues>;
  disabled: boolean;
  auto: boolean;
  setAuto: (auto: boolean) => void;
  initialBody: ArticleBodies[ArticleLocale];
  onBodyChange: (value: ArticleBodies[ArticleLocale]) => void;
  problems: BodyProblem[];
}) {
  const { t } = useLocale();
  const media = useSiteMedia();
  const lookup = useMemo(() => {
    const map = new Map<string, { url?: string; alt?: string }>();
    for (const item of media.data?.items ?? []) {
      let url: string | undefined;
      try {
        url = assertServiceMediaURL(item.url) ?? undefined;
      } catch {
        url = undefined;
      }
      map.set(item.uuid, {
        url,
        alt:
          (locale === "en"
            ? item.alt.en || item.alt.tr
            : item.alt.tr || item.alt.en) ?? "",
      });
    }
    return map;
  }, [locale, media.data]);
  const resolveMedia = useCallback(
    (uuid: string) => lookup.get(uuid),
    [lookup],
  );
  return (
    <div className="space-y-4">
      <AutoSlug locale={locale} auto={auto} form={form} />
      <FormSection
        title={t("site.articles.sections.content", {
          locale: t(`site.locales.${locale}`),
        })}
        description={t("site.articles.sections.content_hint")}
      >
        <AppInput
          name={`title.${locale}`}
          label={t("site.articles.fields.title")}
          labelAction={
            <LengthHint
              name={`title.${locale}`}
              max={ARTICLE_FIELD_LIMITS.title}
            />
          }
          maxLength={ARTICLE_FIELD_LIMITS.title}
          disabled={disabled}
        />
        <SlugField
          locale={locale}
          auto={auto}
          onAutoChange={setAuto}
          disabled={disabled}
        />
        <div className="space-y-1">
          <AppTextarea
            name={`summary.${locale}`}
            label={t("site.articles.fields.summary")}
            description={t("site.articles.fields.summary_hint")}
            rows={3}
            maxLength={ARTICLE_FIELD_LIMITS.summary}
            disabled={disabled}
          />
          <LengthHint
            name={`summary.${locale}`}
            max={ARTICLE_FIELD_LIMITS.summary}
          />
        </div>
        <FormFieldShell
          name={`body-${locale}`}
          label={t("site.articles.fields.body")}
          description={t("site.articles.fields.body_hint")}
        >
          <RichTextEditor
            id={`body-${locale}`}
            lang={locale}
            initialValue={initialBody}
            onChange={onBodyChange}
            disabled={disabled}
            invalid={problems.length > 0}
            placeholder={t("site.articles.fields.body_placeholder")}
            resolveMedia={resolveMedia}
            renderImagePicker={({ open, onOpenChange, onSelect }) => (
              <MediaPickerDialog
                open={open}
                onOpenChange={onOpenChange}
                onSelect={onSelect}
                canUpload={!disabled}
              />
            )}
          />
        </FormFieldShell>
        <BodyProblems problems={problems} />
      </FormSection>
      <FormSection
        title={t("site.articles.sections.seo")}
        description={t("site.articles.sections.seo_hint")}
      >
        <AppInput
          name={`seo_title.${locale}`}
          label={t("site.articles.fields.seo_title")}
          labelAction={
            <LengthHint
              name={`seo_title.${locale}`}
              max={ARTICLE_FIELD_LIMITS.seoTitle}
            />
          }
          maxLength={ARTICLE_FIELD_LIMITS.seoTitle}
          disabled={disabled}
        />
        <div className="space-y-1">
          <AppTextarea
            name={`seo_description.${locale}`}
            label={t("site.articles.fields.seo_description")}
            rows={3}
            maxLength={ARTICLE_FIELD_LIMITS.seoDescription}
            disabled={disabled}
          />
          <LengthHint
            name={`seo_description.${locale}`}
            max={ARTICLE_FIELD_LIMITS.seoDescription}
          />
        </div>
      </FormSection>
    </div>
  );
}

function ArticleForm({
  article,
  canManage,
  onReload,
}: {
  article: SiteArticle | null;
  canManage: boolean;
  onReload: () => void;
}) {
  const { t, locale: uiLocale } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const schema = useMemo(() => createArticleSchema(t), [t]);
  const form = useForm<ArticleFormValues>({
    resolver: zodResolver(schema as never),
    defaultValues: article ? articleToForm(article) : emptyArticleForm(),
  });
  const create = useCreateSiteArticle();
  const update = useUpdateSiteArticle(article?.uuid ?? "");
  const status = useSetSiteArticleStatus(article?.uuid ?? "");
  const remove = useDeleteSiteArticle();

  const [initialBodies] = useState(() => articleBodies(article));
  const bodies = useRef<ArticleBodies>(initialBodies);
  const [baseline, setBaseline] = useState(() => ({
    tr: bodyFingerprint(initialBodies.tr),
    en: bodyFingerprint(initialBodies.en),
  }));
  const [bodyDirty, setBodyDirty] = useState({ tr: false, en: false });
  const [problems, setProblems] = useState<
    Record<ArticleLocale, BodyProblem[]>
  >({ tr: [], en: [] });
  const [auto, setAuto] = useState<Record<ArticleLocale, boolean>>(() => ({
    tr: !article?.slug?.tr,
    en: !article?.slug?.en,
  }));
  const [tab, setTab] = useState<ArticleLocale>(
    uiLocale === "en" && !(article?.title?.tr && !article?.title?.en)
      ? "en"
      : "tr",
  );
  const [conflict, setConflict] = useState(false);
  const [confirm, setConfirm] = useState<
    "delete" | "publish" | "unpublish" | null
  >(null);

  const disabled = !canManage;
  const { isDirty: fieldsDirty, isSubmitting, errors } = form.formState;
  const dirty = fieldsDirty || bodyDirty.tr || bodyDirty.en;
  const busy =
    isSubmitting || create.isPending || update.isPending || status.isPending;

  useEffect(() => {
    if (!dirty) return;
    const warn = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);

  const onBodyChange =
    (loc: ArticleLocale) => (value: ArticleBodies[ArticleLocale]) => {
      bodies.current = { ...bodies.current, [loc]: value };
      const changed = bodyFingerprint(value) !== baseline[loc];
      setBodyDirty((prev) =>
        prev[loc] === changed ? prev : { ...prev, [loc]: changed },
      );
      if (problems[loc].length) setProblems((prev) => ({ ...prev, [loc]: [] }));
    };

  /** Maps an API error onto the fields, the body summaries or a toast. */
  const handleError = (error: unknown) => {
    if (!isApiError(error)) {
      appToast.error(t("common.error_generic"));
      return;
    }
    const e = error as ApiError;
    if (e.code === "VERSION_CONFLICT") {
      setConflict(true);
      return;
    }
    if (e.code === "ARTICLE_SLUG_TAKEN") {
      const values = form.getValues();
      for (const loc of ARTICLE_LOCALES) {
        if (values.slug[loc]) {
          form.setError(`slug.${loc}`, {
            type: "server",
            message: t("site.articles.errors.codes.taken"),
          });
        }
      }
      appToast.error(t("site.articles.errors.slug_taken"));
      return;
    }
    if (e.isValidation && e.details.length) {
      const next: Record<ArticleLocale, BodyProblem[]> = { tr: [], en: [] };
      let firstLocale: ArticleLocale | null = null;
      let other = false;
      for (const d of e.details) {
        const field = d.field ?? "";
        const body = bodyProblemFromField(field, d.code ?? "");
        if (body) {
          next[body.locale].push(body);
          firstLocale ??= body.locale;
          continue;
        }
        const m =
          /^(slug|title|summary|seo_title|seo_description)\.(tr|en)$/.exec(
            field,
          );
        if (m) {
          form.setError(`${m[1]}.${m[2]}` as `slug.tr`, {
            type: "server",
            message: detailMessage(t, d.code, d.message),
          });
          firstLocale ??= m[2] as ArticleLocale;
        } else if (field === "slug") {
          form.setError("slug.tr", {
            type: "server",
            message: t("site.articles.errors.one_locale"),
          });
          firstLocale ??= "tr";
        } else if (field.startsWith("tags")) {
          form.setError("tags", {
            type: "server",
            message: detailMessage(t, d.code, d.message),
          });
        } else if (field === "cover_media_uuid") {
          form.setError("cover_media_uuid", {
            type: "server",
            message: t("site.articles.errors.cover_missing"),
          });
        } else if (field === "sort") {
          form.setError("sort", {
            type: "server",
            message: detailMessage(t, d.code, d.message),
          });
        } else {
          other = true;
        }
      }
      setProblems(next);
      if (firstLocale) setTab(firstLocale);
      appToast.error(
        other
          ? e.message || t("site.articles.toast.fix_errors")
          : t("site.articles.toast.fix_errors"),
      );
      return;
    }
    appToast.error(e.message || t("common.error_generic"));
  };

  const onValid = async (values: ArticleFormValues) => {
    setConflict(false);
    const result = formToInput(values, bodies.current);
    if (!result.ok) {
      const next = {
        tr: result.issues.tr.map((i) => bodyProblemFromIssue("tr", i)),
        en: result.issues.en.map((i) => bodyProblemFromIssue("en", i)),
      };
      setProblems(next);
      setTab(next.tr.length ? "tr" : "en");
      appToast.error(t("site.articles.toast.fix_errors"));
      return;
    }
    setProblems({ tr: [], en: [] });
    try {
      if (!article) {
        const created = await create.mutateAsync(result.input);
        appToast.success(t("site.articles.toast.created"));
        form.reset(values);
        setBodyDirty({ tr: false, en: false });
        router.replace(
          routes.tenant.settings.siteArticles.detail(slug, created.uuid),
        );
        return;
      }
      await update.mutateAsync({ ...result.input, version: article.version });
      form.reset(values);
      setBaseline({
        tr: bodyFingerprint(bodies.current.tr),
        en: bodyFingerprint(bodies.current.en),
      });
      setBodyDirty({ tr: false, en: false });
      // A saved slug no longer follows the title (its URL is public).
      setAuto((prev) => ({
        tr: prev.tr && !values.slug.tr,
        en: prev.en && !values.slug.en,
      }));
      appToast.success(
        t(
          article.status === "published"
            ? "site.articles.toast.saved_live"
            : "site.articles.toast.saved",
        ),
      );
    } catch (error) {
      handleError(error);
    }
  };
  const onInvalid = (fieldErrors: FieldErrors<ArticleFormValues>) => {
    const tr = hasErrorUnder(fieldErrors, "tr");
    const en = hasErrorUnder(fieldErrors, "en");
    if (tr || en) setTab(tr ? "tr" : "en");
    appToast.error(t("site.articles.toast.fix_errors"));
  };

  const setPublished = (publish: boolean) => {
    if (!article) return;
    status.mutate(
      { publish, version: article.version },
      {
        onSuccess: () =>
          appToast.success(
            t(
              publish
                ? "site.articles.toast.published"
                : "site.articles.toast.unpublished",
            ),
          ),
        onError: handleError,
        onSettled: () => setConfirm(null),
      },
    );
  };

  const title = article ? articleTitle(article, uiLocale) : "";

  return (
    <EntityPage
      title={
        article
          ? title || t("site.articles.untitled")
          : t("site.articles.new_title")
      }
      description={t("site.articles.editor_description")}
      permission={permissions.tenant.site.read}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.tenant.settings.root(slug),
        },
        {
          label: t("site.articles.title"),
          href: routes.tenant.settings.siteArticles.root(slug),
        },
        {
          label: article
            ? title || t("site.articles.untitled")
            : t("site.articles.new_title"),
        },
      ]}
      actions={
        <div className="flex flex-wrap items-center gap-2">
          {article ? (
            <StatusChip
              label={t(STATUS_LABEL_KEYS[article.status])}
              tone={STATUS_TONES[article.status]}
            />
          ) : null}
          {dirty ? (
            <StatusChip label={t("site.articles.unsaved")} tone="warning" />
          ) : null}
          <Button asChild size="sm" variant="outline">
            <Link href={routes.tenant.settings.siteArticles.root(slug)}>
              <ArrowLeft className="size-4" />
              {t("site.articles.actions.back")}
            </Link>
          </Button>
          {article && canManage ? (
            <>
              {article.status === "published" ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={busy}
                  onClick={() => setConfirm("unpublish")}
                >
                  <EyeOff className="size-4" />
                  {t("site.articles.actions.unpublish")}
                </Button>
              ) : (
                <Button
                  type="button"
                  size="sm"
                  disabled={busy || dirty}
                  title={
                    dirty ? t("site.articles.publish_save_first") : undefined
                  }
                  onClick={() => setConfirm("publish")}
                >
                  <Upload className="size-4" />
                  {t("site.articles.actions.publish")}
                </Button>
              )}
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={busy}
                aria-label={t("site.articles.actions.delete")}
                onClick={() => setConfirm("delete")}
              >
                <Trash2 className="size-4" />
                {t("site.articles.actions.delete")}
              </Button>
            </>
          ) : null}
        </div>
      }
    >
      <FormProvider {...form}>
        <form
          className="space-y-4"
          noValidate
          onSubmit={(e) => void form.handleSubmit(onValid, onInvalid)(e)}
        >
          {conflict ? (
            <Alert variant="destructive">
              <AlertTitle>{t("site.articles.conflict.title")}</AlertTitle>
              <AlertDescription>
                <p>{t("site.articles.conflict.description")}</p>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="mt-2"
                  onClick={onReload}
                >
                  {t("site.articles.conflict.reload")}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {article?.status === "published" && canManage ? (
            <Alert>
              <AlertTitle>{t("site.articles.live_notice.title")}</AlertTitle>
              <AlertDescription>
                {t("site.articles.live_notice.description")}
              </AlertDescription>
            </Alert>
          ) : null}
          <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_20rem]">
            <Tabs
              value={tab}
              onValueChange={(v) => setTab(v as ArticleLocale)}
              className="min-w-0 space-y-4"
            >
              <TabsList>
                {ARTICLE_LOCALES.map((loc) => (
                  <TabsTrigger key={loc} value={loc}>
                    {t(`site.locales.${loc}`)}
                    {hasErrorUnder(errors, loc) || problems[loc].length ? (
                      <span
                        className="bg-destructive size-1.5 rounded-full"
                        aria-label={t("site.tab_has_errors")}
                      />
                    ) : null}
                  </TabsTrigger>
                ))}
              </TabsList>
              {ARTICLE_LOCALES.map((loc) => (
                <TabsContent
                  key={loc}
                  value={loc}
                  forceMount
                  className="data-[state=inactive]:hidden"
                >
                  <LocalePanel
                    locale={loc}
                    form={form}
                    disabled={disabled}
                    auto={auto[loc] && !disabled}
                    setAuto={(v) => setAuto((prev) => ({ ...prev, [loc]: v }))}
                    initialBody={initialBodies[loc]}
                    onBodyChange={onBodyChange(loc)}
                    problems={problems[loc]}
                  />
                </TabsContent>
              ))}
            </Tabs>
            <div className="space-y-4">
              <EntitySectionCard title={t("site.articles.sections.cover")}>
                <MediaField
                  name="cover_media_uuid"
                  label={t("site.articles.fields.cover")}
                  description={t("site.articles.fields.cover_hint")}
                  disabled={disabled}
                  canUpload={canManage}
                />
              </EntitySectionCard>
              <EntitySectionCard title={t("site.articles.sections.listing")}>
                <div className="space-y-4">
                  <AppSwitch
                    name="featured"
                    label={t("site.articles.fields.featured")}
                    description={t("site.articles.fields.featured_hint")}
                    disabled={disabled}
                  />
                  <SortField disabled={disabled} />
                  <TagsField disabled={disabled} />
                </div>
              </EntitySectionCard>
              {article ? (
                <EntitySectionCard title={t("site.articles.sections.info")}>
                  <dl className="grid gap-2 text-sm">
                    {(
                      [
                        ["published_at", article.published_at],
                        ["updated_at", article.updated_at],
                        ["created_at", article.created_at],
                      ] as const
                    ).map(([key, value]) => (
                      <div key={key} className="flex justify-between gap-3">
                        <dt className="text-muted-foreground">
                          {t(`site.articles.columns.${key}`)}
                        </dt>
                        <dd className="text-end tabular-nums">
                          {value ? datetime(value, undefined, uiLocale) : "—"}
                        </dd>
                      </div>
                    ))}
                  </dl>
                </EntitySectionCard>
              ) : null}
            </div>
          </div>
          {canManage ? (
            <FormActions className="justify-between">
              <p className="text-muted-foreground text-sm">
                {dirty
                  ? t("site.articles.save_bar.dirty")
                  : article?.status === "published"
                    ? t("site.articles.save_bar.live_hint")
                    : t("site.articles.save_bar.draft_hint")}
              </p>
              <div className="flex gap-2">
                {article ? (
                  <Button
                    type="button"
                    variant="outline"
                    disabled={!dirty || busy}
                    onClick={onReload}
                  >
                    {t("site.articles.save_bar.discard")}
                  </Button>
                ) : null}
                <Button
                  type="submit"
                  disabled={busy || (article !== null && !dirty)}
                >
                  {article
                    ? t("common.save")
                    : t("site.articles.save_bar.create")}
                </Button>
              </div>
            </FormActions>
          ) : null}
        </form>
      </FormProvider>
      {article ? (
        <>
          <ConfirmDialog
            open={confirm === "publish" || confirm === "unpublish"}
            title={t(
              confirm === "unpublish"
                ? "site.articles.confirm.unpublish_title"
                : "site.articles.confirm.publish_title",
            )}
            description={t(
              confirm === "unpublish"
                ? "site.articles.confirm.unpublish_description"
                : "site.articles.confirm.publish_description",
            )}
            confirmLabel={t(
              confirm === "unpublish"
                ? "site.articles.actions.unpublish"
                : "site.articles.actions.publish",
            )}
            cancelLabel={t("common.cancel")}
            isPending={status.isPending}
            onCancel={() => setConfirm(null)}
            onConfirm={() => setPublished(confirm === "publish")}
          />
          <ConfirmDialog
            open={confirm === "delete"}
            title={t("site.articles.confirm.delete_title")}
            description={t(
              article.status === "published"
                ? "site.articles.confirm.delete_live_description"
                : "site.articles.confirm.delete_description",
            )}
            confirmLabel={t("site.articles.actions.delete")}
            cancelLabel={t("common.cancel")}
            variant="destructive"
            isPending={remove.isPending}
            onCancel={() => setConfirm(null)}
            onConfirm={() =>
              remove.mutate(
                { uuid: article.uuid, version: article.version },
                {
                  onSuccess: () => {
                    appToast.success(t("site.articles.toast.deleted"));
                    router.push(routes.tenant.settings.siteArticles.root(slug));
                  },
                  onError: handleError,
                  onSettled: () => setConfirm(null),
                },
              )
            }
          />
        </>
      ) : null}
    </EntityPage>
  );
}

/**
 * Ayarlar → Site rehberi → yazı (TEC-785): one guide article with tr / en
 * tabs (title, slug following the title, summary, Plate body, SEO), cover,
 * tags, featured and sort. Saving sends the `version` it loaded (409 →
 * reload); publishing needs the saved state.
 */
export function SiteArticleEditorPage({ uuid }: { uuid: string | null }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canManage = can(permissions.tenant.site.manage);
  const query = useSiteArticle(uuid);
  const [formKey, setFormKey] = useState(0);
  const reload = () => {
    void query.refetch().then(() => setFormKey((k) => k + 1));
  };

  if (uuid && query.isLoading) {
    return <Loading label={t("common.loading")} />;
  }
  if (uuid && (query.isError || !query.data)) {
    const notFound = isApiError(query.error) && query.error.isNotFound;
    return (
      <ErrorState
        title={t(notFound ? "site.articles.not_found" : "common.error_generic")}
        description={t(
          notFound
            ? "site.articles.not_found_description"
            : "site.articles.load_failed",
        )}
        onRetry={notFound ? undefined : () => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (!uuid && !canManage) {
    return (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t("site.articles.forbidden")}
      />
    );
  }
  return (
    <ArticleForm
      key={`${uuid ?? "new"}-${formKey}`}
      article={uuid ? (query.data ?? null) : null}
      canManage={canManage}
      onReload={reload}
    />
  );
}
