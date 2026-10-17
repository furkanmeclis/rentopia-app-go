"use client";

import { AppInput, AppTextarea, FormSection } from "@/components/forms";
import { LengthHint } from "@/features/site/components/site-fields";
import { SITE_LIMITS, SITE_LOCALES } from "@/features/site/schemas/site-form";
import { useLocale } from "@/providers/locale-provider";

/** SEO & İletişim & Analitik: meta per language, contact, GA4. */
export function SiteSeoTab({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
      <FormSection
        title={t("site.seo.title")}
        description={t("site.seo.description")}
        columns={2}
      >
        {SITE_LOCALES.map((l) => (
          <div key={l} className="space-y-4">
            <p className="text-sm font-medium">{t(`site.locales.${l}`)}</p>
            <AppInput
              name={`config.seo.${l}.title`}
              label={t("site.seo.meta_title")}
              labelAction={
                <LengthHint
                  name={`config.seo.${l}.title`}
                  max={SITE_LIMITS.seoTitle}
                />
              }
              disabled={disabled}
            />
            <AppTextarea
              name={`config.seo.${l}.description`}
              label={t("site.seo.meta_description")}
              description={t("site.seo.meta_description_hint", {
                max: SITE_LIMITS.seoDescription,
              })}
              rows={3}
              disabled={disabled}
            />
            <LengthHint
              name={`config.seo.${l}.description`}
              max={SITE_LIMITS.seoDescription}
            />
          </div>
        ))}
      </FormSection>
      <FormSection
        title={t("site.contact.title")}
        description={t("site.contact.description")}
        columns={2}
      >
        <AppInput
          name="config.contact.phone"
          type="tel"
          label={t("site.contact.phone")}
          placeholder="+90 212 000 00 00"
          disabled={disabled}
        />
        <AppInput
          name="config.contact.whatsapp"
          type="tel"
          label={t("site.contact.whatsapp")}
          placeholder="+90 555 000 00 00"
          disabled={disabled}
        />
        <AppInput
          name="config.contact.email"
          type="email"
          label={t("site.contact.email")}
          disabled={disabled}
        />
        <AppInput
          name="config.contact.address"
          label={t("site.contact.address")}
          maxLength={SITE_LIMITS.contact}
          disabled={disabled}
        />
      </FormSection>
      <FormSection
        title={t("site.analytics.title")}
        description={t("site.analytics.description")}
      >
        <AppInput
          name="config.analytics.ga4_measurement_id"
          label={t("site.analytics.ga4")}
          placeholder="G-XXXXXXXXXX"
          spellCheck={false}
          autoComplete="off"
          disabled={disabled}
        />
      </FormSection>
    </div>
  );
}
