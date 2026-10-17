"use client";

import { useState } from "react";
import {
  Controller,
  get,
  useFieldArray,
  useFormContext,
} from "react-hook-form";

import { AppInput, AppTextarea, FormSection } from "@/components/forms";
import { FormFieldShell } from "@/components/forms/form-field";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  CollectionItem,
  CollectionList,
} from "@/features/site/components/site-collection";
import { LengthHint } from "@/features/site/components/site-fields";
import { MediaField } from "@/features/site/components/site-media";
import {
  ADVANTAGE_ICONS,
  ADVANTAGE_ICON_IDS,
  V2_LIMITS,
  keySegment,
} from "@/features/site/lib/site-options";
import {
  SITE_LIMITS,
  SITE_LOCALES,
  emptySlide,
  type SiteFormValues,
} from "@/features/site/schemas/site-form";
import type { SiteLocale } from "@/features/site/types";
import { useLocale } from "@/providers/locale-provider";

function listError(errors: unknown, name: string) {
  const e = get(errors, name);
  return (e?.message ?? e?.root?.message) as string | undefined;
}

function HeroSlides({
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
  const name = `config.content.${locale}.hero_slides` as const;
  const { fields, append, remove, move } = useFieldArray({ control, name });
  return (
    <FormSection
      title={t("site.slides.title")}
      description={t("site.slides.description", { max: V2_LIMITS.slides })}
    >
      <CollectionList
        count={fields.length}
        max={V2_LIMITS.slides}
        emptyTitle={t("site.slides.empty_title")}
        addLabel={t("site.slides.add")}
        onAdd={() => append(emptySlide())}
        error={listError(errors, name)}
        disabled={disabled}
      >
        {fields.map((item, index) => {
          const base = `${name}.${index}` as const;
          return (
            <CollectionItem
              key={item.id}
              title={t("site.slides.item", { n: index + 1 })}
              index={index}
              count={fields.length}
              onMove={move}
              onRemove={() => remove(index)}
              canRemove={fields.length > 1}
              disabled={disabled}
              error={listError(errors, base)}
              extra={
                <Controller
                  name={`${base}.enabled`}
                  control={control}
                  render={({ field }) => (
                    <label className="text-muted-foreground me-2 flex items-center gap-2 text-xs">
                      {t("site.slides.enabled")}
                      <Switch
                        checked={Boolean(field.value)}
                        onCheckedChange={field.onChange}
                        disabled={disabled}
                      />
                    </label>
                  )}
                />
              }
            >
              <AppInput
                name={`${base}.title`}
                label={t("site.slides.slide_title")}
                labelAction={
                  <LengthHint
                    name={`${base}.title`}
                    max={V2_LIMITS.slideTitle}
                  />
                }
                disabled={disabled}
              />
              <AppTextarea
                name={`${base}.subtitle`}
                label={t("site.slides.subtitle")}
                rows={2}
                maxLength={V2_LIMITS.slideSubtitle}
                disabled={disabled}
              />
              <div className="grid gap-4 sm:grid-cols-2">
                <AppInput
                  name={`${base}.cta_label`}
                  label={t("site.slides.cta_label")}
                  maxLength={V2_LIMITS.ctaLabel}
                  placeholder={t("site.slides.cta_label_placeholder")}
                  disabled={disabled}
                />
                <AppInput
                  name={`${base}.cta_href`}
                  label={t("site.slides.cta_href")}
                  placeholder="/arac-kirala"
                  spellCheck={false}
                  disabled={disabled}
                />
              </div>
              <MediaField
                name={`${base}.media`}
                label={t("site.slides.media")}
                description={t("site.slides.media_hint")}
                disabled={disabled}
              />
            </CollectionItem>
          );
        })}
      </CollectionList>
    </FormSection>
  );
}

function IconSelect({ name, disabled }: { name: string; disabled?: boolean }) {
  const { t } = useLocale();
  const { control } = useFormContext();
  return (
    <FormFieldShell name={name} label={t("site.advantages.icon")}>
      <Controller
        name={name}
        control={control}
        render={({ field }) => (
          <Select
            value={field.value}
            onValueChange={field.onChange}
            disabled={disabled}
          >
            <SelectTrigger id={name} className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ADVANTAGE_ICON_IDS.map((id) => {
                const Icon = ADVANTAGE_ICONS[id];
                return (
                  <SelectItem key={id} value={id}>
                    <Icon className="size-4" aria-hidden />
                    {t(`site.advantages.icons.${keySegment(id)}`)}
                  </SelectItem>
                );
              })}
            </SelectContent>
          </Select>
        )}
      />
    </FormFieldShell>
  );
}

function Advantages({
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
  const name = `config.content.${locale}.advantages` as const;
  const { fields, append, remove, move } = useFieldArray({ control, name });
  return (
    <FormSection
      title={t("site.advantages.title")}
      description={t("site.advantages.description", {
        max: V2_LIMITS.advantages,
      })}
    >
      <CollectionList
        count={fields.length}
        max={V2_LIMITS.advantages}
        emptyTitle={t("site.advantages.empty_title")}
        emptyDescription={t("site.advantages.empty_description")}
        addLabel={t("site.advantages.add")}
        onAdd={() => append({ icon: "shield-check", title: "", text: "" })}
        error={listError(errors, name)}
        disabled={disabled}
      >
        {fields.map((item, index) => {
          const base = `${name}.${index}` as const;
          return (
            <CollectionItem
              key={item.id}
              title={t("site.advantages.item", { n: index + 1 })}
              index={index}
              count={fields.length}
              onMove={move}
              onRemove={() => remove(index)}
              disabled={disabled}
              error={listError(errors, base)}
            >
              <div className="grid gap-4 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)]">
                <IconSelect name={`${base}.icon`} disabled={disabled} />
                <AppInput
                  name={`${base}.title`}
                  label={t("site.advantages.item_title")}
                  maxLength={V2_LIMITS.advantageTitle}
                  disabled={disabled}
                />
              </div>
              <AppTextarea
                name={`${base}.text`}
                label={t("site.advantages.text")}
                rows={2}
                maxLength={V2_LIMITS.advantageText}
                disabled={disabled}
              />
            </CollectionItem>
          );
        })}
      </CollectionList>
    </FormSection>
  );
}

function Highlights({
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
  const name = `config.about.highlights.${locale}` as const;
  const { fields, append, remove, move } = useFieldArray({ control, name });
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium">{t("site.about.highlights")}</p>
      <p className="text-muted-foreground text-xs">
        {t("site.about.highlights_hint", { max: V2_LIMITS.highlights })}
      </p>
      <CollectionList
        count={fields.length}
        max={V2_LIMITS.highlights}
        emptyTitle={t("site.about.highlights_empty")}
        addLabel={t("site.about.highlight_add")}
        onAdd={() => append({ value: "" })}
        error={listError(errors, name)}
        disabled={disabled}
      >
        {fields.map((item, index) => (
          <CollectionItem
            key={item.id}
            title={t("site.about.highlight_item", { n: index + 1 })}
            index={index}
            count={fields.length}
            onMove={move}
            onRemove={() => remove(index)}
            disabled={disabled}
          >
            <AppInput
              name={`${name}.${index}.value`}
              label={t("site.about.highlight")}
              maxLength={V2_LIMITS.highlight}
              disabled={disabled}
            />
          </CollectionItem>
        ))}
      </CollectionList>
    </div>
  );
}

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
  const name = `config.content.${locale}.faq` as const;
  const { fields, append, remove, move } = useFieldArray({ control, name });
  return (
    <FormSection
      title={t("site.content.faq_title")}
      description={t("site.content.faq_description", { max: SITE_LIMITS.faq })}
    >
      <CollectionList
        count={fields.length}
        max={SITE_LIMITS.faq}
        emptyTitle={t("site.content.faq_empty_title")}
        emptyDescription={t("site.content.faq_empty_description")}
        addLabel={t("site.content.faq_add")}
        onAdd={() => append({ question: "", answer: "" })}
        error={listError(errors, name)}
        disabled={disabled}
      >
        {fields.map((item, index) => {
          const base = `${name}.${index}` as const;
          return (
            <CollectionItem
              key={item.id}
              title={t("site.content.faq_item", { n: index + 1 })}
              index={index}
              count={fields.length}
              onMove={move}
              onRemove={() => remove(index)}
              disabled={disabled}
              error={listError(errors, base)}
            >
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
            </CollectionItem>
          );
        })}
      </CollectionList>
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
      <HeroSlides locale={locale} disabled={disabled} />
      <FormSection
        title={t("site.content.hero_title_section")}
        description={t("site.content.hero_legacy_hint")}
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
      <Advantages locale={locale} disabled={disabled} />
      <FormSection
        title={t("site.about.title")}
        description={t("site.content.plain_text_hint")}
      >
        <AppTextarea
          name={`${p}.about`}
          label={t("site.content.about")}
          rows={6}
          disabled={disabled}
        />
        <Highlights locale={locale} disabled={disabled} />
      </FormSection>
      <FaqList locale={locale} disabled={disabled} />
      <FormSection
        title={t("site.content.legal_title")}
        description={t("site.content.plain_text_hint")}
      >
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
    </div>
  );
}

/** Marks a locale tab whose fields have errors. */
function LocaleTabDot({ locale }: { locale: SiteLocale }) {
  const { t } = useLocale();
  const {
    formState: { errors },
  } = useFormContext<SiteFormValues>();
  const has =
    Boolean(get(errors, `config.content.${locale}`)) ||
    Boolean(get(errors, `config.about.highlights.${locale}`));
  return has ? (
    <span
      className="bg-destructive size-1.5 rounded-full"
      aria-label={t("site.tab_has_errors")}
    />
  ) : null;
}

/**
 * İçerik: the shared about image, then per language the hero slides,
 * the current site's hero text, advantages, about story and highlights,
 * FAQ and the legal pages (plain text only, no HTML).
 */
export function SiteContentTab({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  const [locale, setLocale] = useState<SiteLocale>("tr");
  return (
    <div className="space-y-4">
      <FormSection
        title={t("site.about.image_title")}
        description={t("site.about.image_description")}
      >
        <MediaField
          name="config.about.image"
          label={t("site.about.image")}
          disabled={disabled}
        />
      </FormSection>
      <Tabs value={locale} onValueChange={(v) => setLocale(v as SiteLocale)}>
        <TabsList>
          {SITE_LOCALES.map((l) => (
            <TabsTrigger key={l} value={l}>
              {t(`site.locales.${l}`)}
              <LocaleTabDot locale={l} />
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
    </div>
  );
}
