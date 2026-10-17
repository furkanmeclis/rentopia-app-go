"use client";

import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { get, useFieldArray, useFormContext } from "react-hook-form";

import { EmptyState } from "@/components/common/empty-state";
import { AppInput, AppTextarea, FormSection } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { LengthHint } from "@/features/site/components/site-fields";
import {
  SITE_LIMITS,
  SITE_LOCALES,
  type SiteFormValues,
} from "@/features/site/schemas/site-form";
import type { SiteLocale } from "@/features/site/types";
import { useLocale } from "@/providers/locale-provider";

function FaqList({
  locale,
  disabled,
}: {
  locale: SiteLocale;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext<SiteFormValues>();
  const { fields, append, remove, move } = useFieldArray({
    control,
    name: `config.content.${locale}.faq`,
  });
  const listError = get(errors, `config.content.${locale}.faq`)?.message as
    string | undefined;
  const full = fields.length >= SITE_LIMITS.faq;

  return (
    <FormSection
      title={t("site.content.faq_title")}
      description={t("site.content.faq_description", { max: SITE_LIMITS.faq })}
    >
      {fields.length === 0 ? (
        <EmptyState
          title={t("site.content.faq_empty_title")}
          description={t("site.content.faq_empty_description")}
        />
      ) : (
        <ol className="space-y-4">
          {fields.map((item, index) => {
            const base = `config.content.${locale}.faq.${index}`;
            const rowError = get(errors, base)?.message as string | undefined;
            return (
              <li
                key={item.id}
                className="bg-muted/30 space-y-3 rounded-lg border p-4"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="text-muted-foreground text-sm font-medium tabular-nums">
                    {t("site.content.faq_item", { n: index + 1 })}
                  </span>
                  {!disabled ? (
                    <div className="flex gap-1">
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        disabled={index === 0}
                        aria-label={t("site.content.faq_up")}
                        onClick={() => move(index, index - 1)}
                      >
                        <ArrowUp className="size-4" />
                      </Button>
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        disabled={index === fields.length - 1}
                        aria-label={t("site.content.faq_down")}
                        onClick={() => move(index, index + 1)}
                      >
                        <ArrowDown className="size-4" />
                      </Button>
                      <Button
                        type="button"
                        size="icon-sm"
                        variant="ghost"
                        aria-label={t("site.content.faq_remove")}
                        onClick={() => remove(index)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    </div>
                  ) : null}
                </div>
                {rowError ? (
                  <p className="text-destructive text-sm">{rowError}</p>
                ) : null}
                <AppInput
                  name={`${base}.question`}
                  label={t("site.content.question")}
                  maxLength={SITE_LIMITS.question}
                  disabled={disabled}
                />
                <AppTextarea
                  name={`${base}.answer`}
                  label={t("site.content.answer")}
                  rows={3}
                  maxLength={SITE_LIMITS.answer}
                  disabled={disabled}
                />
              </li>
            );
          })}
        </ol>
      )}
      {listError ? (
        <p className="text-destructive text-sm">{listError}</p>
      ) : null}
      {!disabled ? (
        <div>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={full}
            onClick={() => append({ question: "", answer: "" })}
          >
            <Plus className="size-4" />
            {t("site.content.faq_add")}
          </Button>
        </div>
      ) : null}
    </FormSection>
  );
}

function LocaleContent({
  locale,
  disabled,
}: {
  locale: SiteLocale;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const p = `config.content.${locale}`;
  return (
    <div className="space-y-4">
      <FormSection
        title={t("site.content.hero_title_section")}
        description={t("site.content.plain_text_hint")}
      >
        <AppInput
          name={`${p}.hero_title`}
          label={t("site.content.hero_title")}
          labelAction={
            <LengthHint name={`${p}.hero_title`} max={SITE_LIMITS.heroTitle} />
          }
          placeholder={t("site.content.hero_title_placeholder")}
          disabled={disabled}
        />
        <AppTextarea
          name={`${p}.hero_subtitle`}
          label={t("site.content.hero_subtitle")}
          rows={2}
          disabled={disabled}
        />
      </FormSection>
      <FormSection
        title={t("site.content.pages_title")}
        description={t("site.content.plain_text_hint")}
      >
        <AppTextarea
          name={`${p}.about`}
          label={t("site.content.about")}
          rows={6}
          disabled={disabled}
        />
        <AppTextarea
          name={`${p}.terms`}
          label={t("site.content.terms")}
          rows={6}
          disabled={disabled}
        />
        <AppTextarea
          name={`${p}.privacy`}
          label={t("site.content.privacy")}
          description={t("site.content.privacy_hint")}
          rows={6}
          disabled={disabled}
        />
      </FormSection>
      <FaqList locale={locale} disabled={disabled} />
    </div>
  );
}

/** İçerik: the texts of each language (plain text only, no HTML). */
export function SiteContentTab({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const [locale, setLocale] = useState<SiteLocale>("tr");
  return (
    <Tabs value={locale} onValueChange={(v) => setLocale(v as SiteLocale)}>
      <TabsList>
        {SITE_LOCALES.map((l) => (
          <TabsTrigger key={l} value={l}>
            {t(`site.locales.${l}`)}
          </TabsTrigger>
        ))}
      </TabsList>
      {SITE_LOCALES.map((l) => (
        <TabsContent
          key={l}
          value={l}
          forceMount
          className="mt-4 data-[state=inactive]:hidden"
        >
          <LocaleContent locale={l} disabled={disabled} />
        </TabsContent>
      ))}
    </Tabs>
  );
}
