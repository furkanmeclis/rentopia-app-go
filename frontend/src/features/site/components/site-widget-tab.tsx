"use client";

import { Copy, Plus, Trash2 } from "lucide-react";
import Link from "next/link";
import { get, useFieldArray, useFormContext } from "react-hook-form";

import { AppInput, AppSelect, FormSection } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  useSiteCatalog,
  useSiteSettings,
} from "@/features/site/hooks/use-site";
import {
  SITE_LIMITS,
  WIDGET_NO_BRANCH,
  type SiteFormValues,
} from "@/features/site/schemas/site-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Widget (TEC-732): the embed snippet, the domains that may frame it and
 * its initial search values (part of the draft: publish to apply). Keys
 * stay on Ayarlar → Public API.
 */
export function SiteWidgetTab({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const catalog = useSiteCatalog();
  const {
    control,
    formState: { errors },
  } = useFormContext<SiteFormValues>();
  const { fields, append, remove } = useFieldArray({
    control,
    name: "config.widget.allowed_domains",
  });
  const settings = useSiteSettings().data;
  const origin = settings?.url ? new URL(settings.url).origin : "";
  const key = settings?.api_key?.publishable_key ?? "";
  const snippet = origin
    ? `<div id="rentopia-booking"></div>\n<script src="${origin}/widget.js" async data-target="#rentopia-booking"${key ? ` data-key="${key}"` : ""} data-locale="tr"></script>`
    : "";

  return (
    <div className="space-y-4">
      <FormSection
        title={t("site.widget.embed_title")}
        description={t("site.widget.embed_description")}
      >
        {snippet ? (
          <div className="space-y-2">
            <pre className="bg-muted overflow-x-auto rounded-md p-3 text-xs break-all whitespace-pre-wrap">
              {snippet}
            </pre>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() =>
                  void navigator.clipboard
                    ?.writeText(snippet)
                    .then(() => appToast.success(t("site.widget.copied")))
                }
              >
                <Copy className="size-4" />
                {t("site.widget.copy_snippet")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="link"
                className="px-0"
                asChild
              >
                <Link href={routes.tenant.settings.publicApi(slug)}>
                  {t("site.widget.keys_link")}
                </Link>
              </Button>
            </div>
            <p className="text-muted-foreground text-xs">
              {t("site.widget.embed_hint")}
            </p>
          </div>
        ) : (
          <Alert>
            <AlertTitle>{t("site.widget.not_ready_title")}</AlertTitle>
            <AlertDescription>
              {t("site.widget.not_ready_description")}
            </AlertDescription>
          </Alert>
        )}
      </FormSection>
      <FormSection
        title={t("site.widget.domains_title")}
        description={t("site.widget.domains_description", {
          max: SITE_LIMITS.widgetDomains,
        })}
      >
        {fields.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("site.widget.domains_empty")}
          </p>
        ) : (
          <ul className="space-y-2">
            {fields.map((item, index) => {
              const serverError = get(
                errors,
                `config.widget.allowed_domains.${index}`,
              )?.message as string | undefined;
              return (
                <li key={item.id} className="flex items-start gap-2">
                  <div className="flex-1">
                    <AppInput
                      name={`config.widget.allowed_domains.${index}.value`}
                      aria-label={t("site.widget.domain")}
                      placeholder="https://www.example.com"
                      spellCheck={false}
                      autoComplete="off"
                      disabled={disabled}
                    />
                    {serverError ? (
                      <p className="text-destructive mt-1 text-sm">
                        {serverError}
                      </p>
                    ) : null}
                  </div>
                  {!disabled ? (
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      aria-label={t("site.widget.domain_remove")}
                      onClick={() => remove(index)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  ) : null}
                </li>
              );
            })}
          </ul>
        )}
        {!disabled ? (
          <div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={fields.length >= SITE_LIMITS.widgetDomains}
              onClick={() => append({ value: "" })}
            >
              <Plus className="size-4" />
              {t("site.widget.domain_add")}
            </Button>
          </div>
        ) : null}
      </FormSection>
      <FormSection
        title={t("site.widget.filters_title")}
        description={t("site.widget.filters_description")}
        columns={2}
      >
        <AppSelect
          name="config.widget.initial_filters.pickup_branch_uuid"
          label={t("site.widget.pickup_branch")}
          placeholder={t("site.widget.pickup_branch_none")}
          options={[
            {
              value: WIDGET_NO_BRANCH,
              label: t("site.widget.pickup_branch_none"),
            },
            ...(catalog.data?.branches ?? []).map((b) => ({
              value: b.uuid,
              label: b.city ? `${b.name} · ${b.city}` : b.name,
            })),
          ]}
          disabled={disabled}
        />
        <AppInput
          name="config.widget.initial_filters.rental_days"
          label={t("site.widget.rental_days")}
          description={t("site.widget.rental_days_hint")}
          inputMode="numeric"
          maxLength={3}
          disabled={disabled}
        />
      </FormSection>
    </div>
  );
}
