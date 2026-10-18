"use client";

import { BookOpenText, Eye, Redo2, Undo2, Upload } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import {
  useFormState,
  type FieldErrors,
  type UseFormReturn,
} from "react-hook-form";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntityPage } from "@/components/entity";
import { AppForm, FormActions } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { SiteBrandTab } from "@/features/site/components/site-brand-tab";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { SiteCatalogTab } from "@/features/site/components/site-catalog-tab";
import { SiteDomainsTab } from "@/features/site/components/site-domains-tab";
import { SiteContentTab } from "@/features/site/components/site-content-tab";
import { SiteGeneralTab } from "@/features/site/components/site-general-tab";
import { SiteImagesTab } from "@/features/site/components/site-images-tab";
import {
  SiteLivePreview,
  SitePreviewSheet,
} from "@/features/site/components/site-preview-sheet";
import { SitePublishTab } from "@/features/site/components/site-publish-tab";
import { SiteSectionsTab } from "@/features/site/components/site-sections-tab";
import { SiteSeoTab } from "@/features/site/components/site-seo-tab";
import { SiteThemeTab } from "@/features/site/components/site-theme-tab";
import { SiteWidgetTab } from "@/features/site/components/site-widget-tab";
import {
  usePublishSite,
  useSaveSite,
  useSiteSettings,
} from "@/features/site/hooks/use-site";
import {
  useFormHistory,
  useUndoShortcuts,
} from "@/features/site/hooks/use-form-history";
import {
  createSiteSchema,
  formToSaveInput,
  settingsToForm,
  type SiteFormValues,
} from "@/features/site/schemas/site-form";
import type { SiteSettings } from "@/features/site/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type Tab =
  | "general"
  | "theme"
  | "sections"
  | "content"
  | "brand"
  | "seo"
  | "images"
  | "publish"
  | "catalog"
  | "domains"
  | "widget";

const TABS: { id: Tab; labelKey: string; form: boolean }[] = [
  { id: "general", labelKey: "site.tabs.general", form: true },
  { id: "theme", labelKey: "site.tabs.theme", form: true },
  { id: "sections", labelKey: "site.tabs.sections", form: true },
  { id: "content", labelKey: "site.tabs.content", form: true },
  { id: "brand", labelKey: "site.tabs.brand", form: true },
  { id: "seo", labelKey: "site.tabs.seo", form: true },
  { id: "images", labelKey: "site.tabs.images", form: false },
  { id: "publish", labelKey: "site.tabs.publish", form: false },
  { id: "catalog", labelKey: "site.tabs.catalog", form: false },
  { id: "domains", labelKey: "site.tabs.domains", form: false },
  { id: "widget", labelKey: "site.tabs.widget", form: true },
];

/** The tab of a form field path (server and client errors). */
function tabOfPath(path: string): Tab {
  if (/^config\.templates\.[^.]+\.sections/.test(path)) return "sections";
  if (
    path.startsWith("config.theme") ||
    path.startsWith("config.layout") ||
    path.startsWith("config.template")
  )
    return "theme";
  if (path.startsWith("config.content") || path.startsWith("config.about"))
    return "content";
  if (path.startsWith("config.menus") || path.startsWith("config.brand"))
    return "brand";
  if (
    path.startsWith("config.seo") ||
    path.startsWith("config.contact") ||
    path.startsWith("config.analytics")
  )
    return "seo";
  if (path.startsWith("config.widget")) return "widget";
  return "general";
}

function errorPaths(errors: FieldErrors, prefix = ""): string[] {
  const out: string[] = [];
  for (const [key, value] of Object.entries(errors)) {
    if (!value || typeof value !== "object") continue;
    const path = prefix ? `${prefix}.${key}` : key;
    if ("message" in value && typeof value.message === "string") {
      out.push(path);
    } else {
      out.push(...errorPaths(value as FieldErrors, path));
    }
  }
  return out;
}

/** Reports the form's dirty state to the page (publish needs a saved draft). */
function DirtyReporter({ onChange }: { onChange: (dirty: boolean) => void }) {
  const { isDirty } = useFormState();
  useEffect(() => onChange(isDirty), [isDirty, onChange]);
  return null;
}

function SiteTabs({
  form,
  settings,
  canManage,
  tab,
  setTab,
  conflict,
  onReload,
  dirty,
  onPublish,
  publishing,
  onDraftReplaced,
}: {
  form: UseFormReturn<SiteFormValues>;
  settings: SiteSettings;
  canManage: boolean;
  tab: Tab;
  setTab: (tab: Tab) => void;
  conflict: boolean;
  onReload: () => void;
  dirty: boolean;
  onPublish: () => void;
  publishing: boolean;
  onDraftReplaced: () => void;
}) {
  const { t } = useLocale();
  const { errors, submitCount, isDirty, isSubmitting } = useFormState();
  const paths = errorPaths(errors);
  const tabsWithErrors = new Set(paths.map(tabOfPath));
  const firstErrorTab = paths.length ? tabOfPath(paths[0]) : null;
  const disabled = !canManage;
  const formTab = TABS.find((x) => x.id === tab)?.form ?? false;
  const history = useFormHistory(form);
  useUndoShortcuts(canManage, history.undo, history.redo);
  const registry = settings.theme_registry ?? [];

  useEffect(() => {
    if (submitCount > 0 && firstErrorTab) setTab(firstErrorTab);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [submitCount, firstErrorTab]);

  return (
    <>
      <div className="flex flex-wrap items-start gap-2">
        <TabsList className="h-auto min-w-0 flex-1 flex-wrap justify-start">
          {TABS.map((item) => (
            <TabsTrigger key={item.id} value={item.id} className="flex-none">
              {t(item.labelKey)}
              {tabsWithErrors.has(item.id) ? (
                <span
                  className="bg-destructive size-1.5 rounded-full"
                  aria-label={t("site.tab_has_errors")}
                />
              ) : null}
            </TabsTrigger>
          ))}
        </TabsList>
        {canManage ? (
          <div
            className="flex gap-1"
            role="group"
            aria-label={t("site.history.label")}
          >
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  disabled={!history.canUndo}
                  aria-label={t("site.history.undo")}
                  onClick={history.undo}
                >
                  <Undo2 className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t("site.history.undo_hint")}</TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  type="button"
                  size="icon"
                  variant="outline"
                  disabled={!history.canRedo}
                  aria-label={t("site.history.redo")}
                  onClick={history.redo}
                >
                  <Redo2 className="size-4" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>{t("site.history.redo_hint")}</TooltipContent>
            </Tooltip>
          </div>
        ) : null}
      </div>

      {conflict ? (
        <Alert variant="destructive">
          <AlertTitle>{t("site.conflict.title")}</AlertTitle>
          <AlertDescription>
            <p>{t("site.conflict.description")}</p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={onReload}
            >
              {t("site.conflict.reload")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}

      <TabsContent
        value="general"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <SiteGeneralTab settings={settings} canManage={canManage} />
      </TabsContent>
      <TabsContent
        value="theme"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <SiteThemeTab registry={registry} disabled={disabled} />
      </TabsContent>
      <TabsContent
        value="sections"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <SiteSectionsTab
          registry={registry}
          statuses={settings.section_statuses}
          savedTemplate={settings.config.template}
          disabled={disabled}
        />
      </TabsContent>
      <TabsContent
        value="content"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <SiteContentTab disabled={disabled} />
      </TabsContent>
      <TabsContent
        value="brand"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <SiteBrandTab disabled={disabled} />
      </TabsContent>
      <TabsContent
        value="seo"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <SiteSeoTab disabled={disabled} />
      </TabsContent>
      <TabsContent value="images">
        <SiteImagesTab settings={settings} canManage={canManage} />
      </TabsContent>
      <TabsContent value="publish">
        <SitePublishTab
          settings={settings}
          canManage={canManage}
          dirty={dirty}
          onPublish={onPublish}
          publishing={publishing}
          onDraftReplaced={onDraftReplaced}
        />
      </TabsContent>
      <TabsContent value="catalog">
        <SiteCatalogTab canManage={canManage} />
      </TabsContent>
      <TabsContent value="domains">
        <SiteDomainsTab canManage={canManage} />
      </TabsContent>
      <TabsContent
        value="widget"
        forceMount
        className="data-[state=inactive]:hidden"
      >
        <SiteWidgetTab disabled={disabled} />
      </TabsContent>

      {canManage && formTab ? (
        <FormActions className="justify-between">
          <p className="text-muted-foreground text-sm">
            {isDirty ? t("site.save_bar.dirty") : t("site.save_bar.draft_hint")}
          </p>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={!isDirty || isSubmitting}
              onClick={onDraftReplaced}
            >
              {t("site.save_bar.discard")}
            </Button>
            <Button type="submit" disabled={!isDirty || isSubmitting}>
              {t("site.save_bar.save")}
            </Button>
          </div>
        </FormActions>
      ) : null}
    </>
  );
}

/** The form, mounted per draft snapshot (remounted after save / restore). */
function SiteForm({
  settings,
  canManage,
  tab,
  setTab,
  onSaved,
  dirty,
  setDirty,
  onPublish,
  publishing,
  onReload,
}: {
  settings: SiteSettings;
  canManage: boolean;
  tab: Tab;
  setTab: (tab: Tab) => void;
  onSaved: () => void;
  dirty: boolean;
  setDirty: (dirty: boolean) => void;
  onPublish: () => void;
  publishing: boolean;
  onReload: () => void;
}) {
  const { t } = useLocale();
  const save = useSaveSite();
  const schema = useMemo(() => createSiteSchema(t), [t]);
  // The draft this form was opened with; later writes (images, key) keep
  // the unsaved edits and only move the version.
  const [defaults] = useState(() => settingsToForm(settings));
  const [conflict, setConflict] = useState(false);

  return (
    <AppForm<SiteFormValues>
      className="space-y-4"
      schema={schema}
      defaultValues={defaults}
      onSubmit={async (values) => {
        setConflict(false);
        try {
          await save.mutateAsync(
            formToSaveInput(values, settings.version, settings.config),
          );
          onSaved();
        } catch (error) {
          if (isApiError(error) && error.code === "VERSION_CONFLICT") {
            setConflict(true);
          } else if (isApiError(error) && error.isValidation) {
            appToast.error(t("site.toast.fix_errors"));
          } else if (isApiError(error) && error.status === 409) {
            appToast.error(error.message);
          } else {
            appToast.error(t("common.error_generic"));
          }
          throw error;
        }
      }}
    >
      {(form) => (
        <>
          <DirtyReporter onChange={setDirty} />
          <SiteTabs
            form={form}
            settings={settings}
            canManage={canManage}
            tab={tab}
            setTab={setTab}
            conflict={conflict}
            onReload={() => {
              setConflict(false);
              onReload();
            }}
            dirty={dirty}
            onPublish={onPublish}
            publishing={publishing}
            onDraftReplaced={onSaved}
          />
        </>
      )}
    </AppForm>
  );
}

const WIDE_QUERY = "(min-width: 1440px)";

/** Wide enough for the studio split (editor + docked live preview). */
function useIsWide() {
  return useSyncExternalStore(
    (callback) => {
      const mql = window.matchMedia(WIDE_QUERY);
      mql.addEventListener("change", callback);
      return () => mql.removeEventListener("change", callback);
    },
    () => window.matchMedia(WIDE_QUERY).matches,
    () => false,
  );
}

/**
 * Ayarlar → Markalı site (TEC-730, studio layout TEC-783): theme,
 * sections and content on top of the ready templates, with a live preview
 * docked on wide screens. Live settings (on / off, address, language, key) apply on save;
 * the document and images are a draft until published, with a live
 * preview and a history to roll back to.
 */
export function SitePage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canManage = can(permissions.tenant.site.manage);
  const query = useSiteSettings();
  const publish = usePublishSite();
  const [tab, setTab] = useState<Tab>("general");
  const [formKey, setFormKey] = useState(0);
  const [dirty, setDirty] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [confirmPublish, setConfirmPublish] = useState(false);
  const settings = query.data;
  const wide = useIsWide();

  const reloadForm = () => {
    setDirty(false);
    setFormKey((k) => k + 1);
  };

  return (
    <EntityPage
      title={t("site.title")}
      description={t("site.description")}
      permission={permissions.tenant.site.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("site.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.section_tenant_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("site.title") },
      ]}
      actions={
        settings ? (
          <div className="flex flex-wrap items-center gap-2">
            <StatusChip
              label={t(settings.live ? "site.status.live" : "site.status.off")}
              tone={settings.live ? "success" : "default"}
            />
            {settings.has_unpublished_changes ? (
              <StatusChip label={t("site.status.unpublished")} tone="warning" />
            ) : null}
            <Button asChild variant="outline">
              <Link href={routes.tenant.settings.siteArticles.root(slug)}>
                <BookOpenText className="size-4" />
                {t("site.articles.title")}
              </Link>
            </Button>
            {!wide ? (
              <Button
                type="button"
                variant="outline"
                onClick={() => setPreviewOpen(true)}
              >
                <Eye className="size-4" />
                {t("site.preview.open")}
              </Button>
            ) : null}
            {canManage ? (
              <Button
                type="button"
                disabled={
                  dirty ||
                  publish.isPending ||
                  !settings.has_unpublished_changes
                }
                title={dirty ? t("site.publish.save_first_title") : undefined}
                onClick={() => setConfirmPublish(true)}
              >
                <Upload className="size-4" />
                {t("site.publish.publish")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    >
      {query.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : query.isError || !settings ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("site.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <div
          className={cn(
            wide &&
              "grid grid-cols-[minmax(34rem,5fr)_minmax(0,6fr)] items-start gap-6",
          )}
        >
          <Tabs
            value={tab}
            onValueChange={(value) => setTab(value as Tab)}
            className="min-w-0 space-y-4"
          >
            <SiteForm
              key={formKey}
              settings={settings}
              canManage={canManage}
              tab={tab}
              setTab={setTab}
              onSaved={reloadForm}
              dirty={dirty}
              setDirty={setDirty}
              onPublish={() => setConfirmPublish(true)}
              publishing={publish.isPending}
              onReload={() => {
                void query.refetch().then(reloadForm);
              }}
            />
          </Tabs>
          {wide ? (
            <SiteLivePreview version={settings.version} dirty={dirty} />
          ) : null}
        </div>
      )}
      {settings ? (
        <>
          <SitePreviewSheet
            open={previewOpen && !wide}
            onOpenChange={setPreviewOpen}
            version={settings.version}
            dirty={dirty}
          />
          <ConfirmDialog
            open={confirmPublish}
            title={t("site.publish.confirm_title")}
            description={t(
              settings.live
                ? "site.publish.confirm_live"
                : "site.publish.confirm_off",
            )}
            confirmLabel={t("site.publish.publish")}
            cancelLabel={t("common.cancel")}
            isPending={publish.isPending}
            onCancel={() => setConfirmPublish(false)}
            onConfirm={() =>
              publish.mutate(settings.version, {
                onSettled: () => setConfirmPublish(false),
              })
            }
          />
        </>
      ) : null}
    </EntityPage>
  );
}
