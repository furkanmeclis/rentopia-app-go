"use client";

import { Plus, Trash2 } from "lucide-react";
import Link from "next/link";
import { get, useFieldArray, useFormContext } from "react-hook-form";

import { AppInput, AppSelect, FormSection } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useSiteCatalog } from "@/features/site/hooks/use-site";
import {
  SITE_LIMITS,
  WIDGET_NO_BRANCH,
  type SiteFormValues,
} from "@/features/site/schemas/site-form";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * Widget (TEC-732 serves it): the domains that may embed it and its
 * initial search values are stored with the draft now. Keys stay on
 * Ayarlar → Public API.
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

  return (
    <div className="space-y-4">
      <Alert>
        <AlertTitle>{t("site.widget.soon_title")}</AlertTitle>
        <AlertDescription>
          <p>{t("site.widget.soon_description")}</p>
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
        </AlertDescription>
      </Alert>
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
