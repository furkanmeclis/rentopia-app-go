"use client";

import { get, useFieldArray, useFormContext } from "react-hook-form";

import { AppInput, AppSelect, FormSection } from "@/components/forms";
import {
  CollectionItem,
  CollectionList,
} from "@/features/site/components/site-collection";
import {
  MENU_KINDS,
  SOCIAL_NETWORKS,
  SOCIAL_PLACEHOLDERS,
  V2_LIMITS,
} from "@/features/site/lib/site-options";
import {
  SITE_LOCALES,
  type SiteFormValues,
} from "@/features/site/schemas/site-form";
import { useLocale } from "@/providers/locale-provider";

type MenuName = "header" | "footer";

function MenuEditor({
  menu,
  disabled,
}: {
  menu: MenuName;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext<SiteFormValues>();
  const name = `config.menus.${menu}` as const;
  const { fields, append, remove, move } = useFieldArray({ control, name });
  const error = get(errors, name);
  return (
    <FormSection
      title={t(`site.menus.${menu}_title`)}
      description={t(`site.menus.${menu}_description`, {
        max: V2_LIMITS.menuItems,
      })}
    >
      <CollectionList
        count={fields.length}
        max={V2_LIMITS.menuItems}
        emptyTitle={t("site.menus.empty_title")}
        emptyDescription={t("site.menus.empty_description")}
        addLabel={t("site.menus.add")}
        onAdd={() =>
          append({ label: { tr: "", en: "" }, href: "", kind: "link" })
        }
        error={(error?.message ?? error?.root?.message) as string | undefined}
        disabled={disabled}
      >
        {fields.map((item, index) => {
          const base = `${name}.${index}` as const;
          return (
            <CollectionItem
              key={item.id}
              title={t("site.menus.item", { n: index + 1 })}
              index={index}
              count={fields.length}
              onMove={move}
              onRemove={() => remove(index)}
              disabled={disabled}
            >
              <div className="grid gap-4 sm:grid-cols-2">
                {SITE_LOCALES.map((loc) => (
                  <AppInput
                    key={loc}
                    name={`${base}.label.${loc}`}
                    label={t("site.menus.label", {
                      locale: t(`site.locales.${loc}`),
                    })}
                    maxLength={V2_LIMITS.menuLabel}
                    disabled={disabled}
                  />
                ))}
              </div>
              <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_minmax(0,12rem)]">
                <AppInput
                  name={`${base}.href`}
                  label={t("site.menus.href")}
                  description={t("site.menus.href_hint")}
                  placeholder="/hakkimizda"
                  spellCheck={false}
                  disabled={disabled}
                />
                <AppSelect
                  name={`${base}.kind`}
                  label={t("site.menus.kind")}
                  options={MENU_KINDS.map((k) => ({
                    value: k,
                    label: t(`site.menus.kinds.${k}`),
                  }))}
                  disabled={disabled}
                />
              </div>
            </CollectionItem>
          );
        })}
      </CollectionList>
    </FormSection>
  );
}

/**
 * Menü ve marka: the brand tagline per language, header / footer menus
 * (labels in both languages) and one HTTPS link per social network.
 */
export function SiteBrandTab({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
      <FormSection
        title={t("site.brand.title")}
        description={t("site.brand.description")}
        columns={2}
      >
        {SITE_LOCALES.map((loc) => (
          <AppInput
            key={loc}
            name={`config.brand.tagline.${loc}`}
            label={t("site.brand.tagline", {
              locale: t(`site.locales.${loc}`),
            })}
            maxLength={V2_LIMITS.tagline}
            disabled={disabled}
          />
        ))}
      </FormSection>
      <MenuEditor menu="header" disabled={disabled} />
      <MenuEditor menu="footer" disabled={disabled} />
      <FormSection
        title={t("site.socials.title")}
        description={t("site.socials.description")}
        columns={2}
      >
        {SOCIAL_NETWORKS.map((network) => (
          <AppInput
            key={network}
            name={`config.brand.socials.${network}`}
            type="url"
            label={t(`site.socials.networks.${network}`)}
            placeholder={SOCIAL_PLACEHOLDERS[network]}
            spellCheck={false}
            autoComplete="off"
            disabled={disabled}
          />
        ))}
      </FormSection>
    </div>
  );
}
