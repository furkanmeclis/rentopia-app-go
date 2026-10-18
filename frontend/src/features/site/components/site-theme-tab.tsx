"use client";

import { Check, Lock } from "lucide-react";
import type { ReactNode } from "react";
import { Controller, useFormContext, useWatch } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { FormSection } from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  ChoiceCards,
  ColorField,
} from "@/features/site/components/site-fields";
import {
  BODY_FONTS,
  CORNER_STYLES,
  CORNER_SWATCH_RADIUS,
  FONT_PREVIEW_FAMILY,
  HEADING_FONTS,
  TEMPLATE_IDS,
  keySegment,
  type TemplateId,
  type ThemeDefinition,
} from "@/features/site/lib/site-options";
import {
  CARDS,
  COLOR_TOKENS,
  FOOTERS,
  HEROES,
  MODES,
  completeTemplateConfig,
  type SiteFormValues,
} from "@/features/site/schemas/site-form";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/* Small schematic previews of the ready-made variants. */
const bar = (w: string, extra = "") => (
  <span className={`bg-foreground/25 block h-1.5 rounded ${w} ${extra}`} />
);
const box = (extra = "") => (
  <span className={`bg-foreground/15 block rounded ${extra}`} />
);

const HERO_PREVIEWS: Record<(typeof HEROES)[number], ReactNode> = {
  centered: (
    <span className="flex w-full flex-col items-center justify-center gap-1.5">
      {bar("w-2/3")}
      {bar("w-1/2")}
      {box("mt-1 h-3 w-3/4")}
    </span>
  ),
  split: (
    <span className="grid w-full grid-cols-2 gap-2">
      <span className="flex flex-col justify-center gap-1.5">
        {bar("w-full")}
        {bar("w-2/3")}
        {box("h-3 w-full")}
      </span>
      {box("h-full")}
    </span>
  ),
  image: (
    <span className="bg-foreground/20 flex w-full flex-col items-center justify-center gap-1.5 rounded">
      {bar("w-2/3", "bg-background/80")}
      {box("h-3 w-3/4 bg-background/70")}
    </span>
  ),
  minimal: (
    <span className="flex w-full flex-col justify-center gap-1.5">
      {bar("w-1/2")}
      {box("h-3 w-full")}
    </span>
  ),
};

const CARD_PREVIEWS: Record<(typeof CARDS)[number], ReactNode> = {
  grid: (
    <span className="grid w-full grid-cols-3 gap-1.5">
      {box("h-full")}
      {box("h-full")}
      {box("h-full")}
    </span>
  ),
  list: (
    <span className="flex w-full flex-col gap-1.5">
      {box("h-4 w-full")}
      {box("h-4 w-full")}
      {box("h-4 w-full")}
    </span>
  ),
  compact: (
    <span className="flex w-full flex-col gap-1">
      {box("h-2.5 w-full")}
      {box("h-2.5 w-full")}
      {box("h-2.5 w-full")}
      {box("h-2.5 w-full")}
    </span>
  ),
};

const FOOTER_PREVIEWS: Record<(typeof FOOTERS)[number], ReactNode> = {
  simple: (
    <span className="flex w-full flex-col justify-end">
      <span className="flex items-center justify-between border-t pt-1.5">
        {bar("w-1/3")}
        {bar("w-1/4")}
      </span>
    </span>
  ),
  columns: (
    <span className="flex w-full flex-col justify-end">
      <span className="grid grid-cols-3 gap-1.5 border-t pt-1.5">
        <span className="space-y-1">
          {bar("w-full")}
          {bar("w-2/3")}
        </span>
        <span className="space-y-1">
          {bar("w-full")}
          {bar("w-2/3")}
        </span>
        <span className="space-y-1">
          {bar("w-full")}
          {bar("w-2/3")}
        </span>
      </span>
    </span>
  ),
  minimal: (
    <span className="flex w-full flex-col items-center justify-end">
      {bar("w-1/3")}
    </span>
  ),
};

/* Schematic covers of the registry templates (no remote images). */
const TEMPLATE_PREVIEWS: Record<TemplateId, ReactNode> = {
  "technowide-design": (
    <span className="flex size-full flex-col gap-1.5">
      <span className="bg-foreground/20 flex h-10 flex-col justify-end gap-1 rounded p-1.5">
        {bar("w-1/2", "bg-background/80")}
        {box("h-2 w-2/3 bg-background/70")}
      </span>
      <span className="grid flex-1 grid-cols-3 gap-1">
        {box("h-full")}
        {box("h-full")}
        {box("h-full")}
      </span>
    </span>
  ),
  "horizon-design": (
    <span className="flex size-full flex-col gap-1.5">
      <span className="grid h-10 grid-cols-2 gap-1.5">
        <span className="flex flex-col justify-center gap-1">
          {bar("w-full")}
          {bar("w-2/3")}
        </span>
        {box("h-full")}
      </span>
      <span className="grid flex-1 grid-cols-2 gap-1">
        {box("h-full")}
        {box("h-full")}
      </span>
    </span>
  ),
  classic: (
    <span className="flex size-full flex-col items-center gap-1.5">
      {bar("w-1/2")}
      {box("h-3 w-3/4")}
      <span className="grid w-full flex-1 grid-cols-3 gap-1">
        {box("h-full")}
        {box("h-full")}
        {box("h-full")}
      </span>
    </span>
  ),
  aurum: (
    <span className="flex size-full flex-col gap-1.5">
      <span className="bg-foreground/30 flex h-12 items-center justify-center rounded">
        {bar("w-1/3", "bg-background/80")}
      </span>
      <span className="flex flex-1 gap-1">
        {box("h-full w-1/3")}
        {box("h-full flex-1")}
      </span>
    </span>
  ),
};

/** Option previews of the typed theme settings (`heroLayout`, …). */
const SETTING_PREVIEWS: Record<string, Record<string, ReactNode>> = {
  heroLayout: { image: HERO_PREVIEWS.image, split: HERO_PREVIEWS.split },
  // Horizon: the serif headline beside the photo, left-aligned or centered.
  heroAlign: {
    left: (
      <span className="grid w-full grid-cols-2 gap-2">
        <span className="flex flex-col justify-center gap-1.5">
          {bar("w-full")}
          {bar("w-2/3")}
          {bar("w-1/3", "mt-1")}
        </span>
        {box("h-full")}
      </span>
    ),
    center: (
      <span className="grid w-full grid-cols-2 gap-2">
        <span className="flex flex-col items-center justify-center gap-1.5">
          {bar("w-full")}
          {bar("w-2/3")}
          {bar("w-1/3", "mt-1")}
        </span>
        {box("h-full")}
      </span>
    ),
  },
};

function TemplateCards({
  registry,
  disabled,
}: {
  registry: ThemeDefinition[];
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const { control, getValues, setValue } = useFormContext<SiteFormValues>();
  const active = useWatch({ control, name: "config.template" });
  const ids = TEMPLATE_IDS.filter(
    (id) => !registry.length || registry.some((d) => d.id === id),
  );
  const choose = (id: TemplateId) => {
    const def = registry.find((d) => d.id === id);
    if (!def?.available || id === active) return;
    // Every template keeps its own sections and settings (stored per id).
    const templates = getValues("config.templates");
    if (!templates[id]) {
      setValue(
        `config.templates.${id}`,
        completeTemplateConfig(def, undefined),
        { shouldDirty: true },
      );
    }
    setValue("config.template", id, { shouldDirty: true });
  };
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {ids.map((id) => {
        const def = registry.find((d) => d.id === id);
        const available = Boolean(def?.available);
        const selected = active === id;
        const k = keySegment(id);
        return (
          <button
            key={id}
            type="button"
            disabled={disabled || !available}
            aria-pressed={selected}
            onClick={() => choose(id)}
            className={cn(
              "bg-card hover:bg-accent/40 flex flex-col gap-3 rounded-lg border p-3 text-start transition-colors",
              "focus-visible:ring-ring outline-none focus-visible:ring-2",
              selected && "border-primary ring-primary/30 ring-2",
              !available && "hover:bg-card cursor-not-allowed opacity-60",
            )}
          >
            <span
              aria-hidden
              className="bg-muted/50 flex h-28 overflow-hidden rounded-md border p-2"
            >
              {TEMPLATE_PREVIEWS[id]}
            </span>
            <span className="flex items-start justify-between gap-2">
              <span className="grid gap-0.5">
                <span className="text-sm font-medium">
                  {t(`site.templates.${k}.name`)}
                </span>
                <span className="text-muted-foreground text-xs">
                  {t(`site.templates.${k}.description`)}
                </span>
              </span>
              {selected ? (
                <Badge>
                  <Check className="size-3" />
                  {t("site.templates.selected")}
                </Badge>
              ) : !available ? (
                <Badge variant="secondary">
                  <Lock className="size-3" />
                  {t("site.templates.soon")}
                </Badge>
              ) : null}
            </span>
          </button>
        );
      })}
    </div>
  );
}

function TemplateSettings({
  registry,
  disabled,
}: {
  registry: ThemeDefinition[];
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const active = useWatch<SiteFormValues, "config.template">({
    name: "config.template",
  });
  const def = registry.find((d) => d.id === active);
  if (!def?.settings.length) return null;
  return (
    <FormSection
      title={t("site.theme_settings.title")}
      description={t("site.theme_settings.description", {
        theme: t(`site.templates.${keySegment(active)}.name`),
      })}
    >
      {def.settings.map((setting) => {
        const k = keySegment(setting.key);
        return (
          <ChoiceCards
            key={`${active}-${setting.key}`}
            name={`config.templates.${active}.settings.${setting.key}`}
            label={t(`site.theme_settings.${k}.label`)}
            columns={setting.values.length >= 3 ? 3 : 2}
            options={setting.values.map((value) => ({
              value,
              label: t(`site.theme_settings.${k}.${keySegment(value)}`),
              preview: SETTING_PREVIEWS[setting.key]?.[value],
            }))}
            disabled={disabled}
          />
        );
      })}
    </FormSection>
  );
}

/** Font select whose items render in their own family (fallback stacks). */
function FontSelect({
  name,
  label,
  description,
  fonts,
  disabled,
}: {
  name: string;
  label: string;
  description?: string;
  fonts: readonly (typeof HEADING_FONTS)[number][];
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const { control } = useFormContext();
  return (
    <FormFieldShell name={name} label={label} description={description}>
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
              {fonts.map((f) => (
                <SelectItem
                  key={f}
                  value={f}
                  style={{ fontFamily: FONT_PREVIEW_FAMILY[f] }}
                >
                  {t(`site.theme.fonts.${f}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      />
    </FormFieldShell>
  );
}

/**
 * Tema: the registry templates (Technowide selectable; the others are
 * shown as "Yakında"), the active template's typed settings, appearance
 * (body / heading font, eight corner styles, mode), light / dark color
 * tokens and the layout variants of the current site. Draft until
 * published.
 */
export function SiteThemeTab({
  registry,
  disabled,
}: {
  registry: ThemeDefinition[];
  disabled?: boolean;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
      <FormSection
        title={t("site.templates.title")}
        description={t("site.templates.description")}
      >
        <TemplateCards registry={registry} disabled={disabled} />
      </FormSection>
      <TemplateSettings registry={registry} disabled={disabled} />
      <FormSection title={t("site.theme.type_title")} columns={2}>
        <FontSelect
          name="config.theme.font"
          label={t("site.theme.font")}
          fonts={BODY_FONTS}
          disabled={disabled}
        />
        <FontSelect
          name="config.theme.heading_font"
          label={t("site.theme.heading_font")}
          description={t("site.theme.heading_font_hint")}
          fonts={HEADING_FONTS}
          disabled={disabled}
        />
        <div className="sm:col-span-2">
          <ChoiceCards
            name="config.theme.radius"
            label={t("site.theme.radius")}
            options={CORNER_STYLES.map((r) => ({
              value: r,
              label: t(`site.theme.radii.${r}`),
              preview: (
                <span className="flex w-full items-center justify-center gap-2">
                  <span
                    className="border-foreground/40 bg-foreground/10 size-9 border"
                    style={{ borderRadius: CORNER_SWATCH_RADIUS[r] }}
                  />
                  <span
                    className="bg-foreground/30 h-4 w-12"
                    style={{ borderRadius: CORNER_SWATCH_RADIUS[r] }}
                  />
                </span>
              ),
            }))}
            disabled={disabled}
          />
        </div>
        <div className="sm:col-span-2">
          <ChoiceCards
            name="config.theme.mode"
            label={t("site.theme.mode")}
            columns={3}
            options={MODES.map((m) => ({
              value: m,
              label: t(`site.theme.modes.${m}`),
              description: t(`site.theme.modes_hint.${m}`),
            }))}
            disabled={disabled}
          />
        </div>
      </FormSection>
      <FormSection
        title={t("site.theme.colors_title")}
        description={t("site.theme.colors_description")}
        columns={2}
      >
        {COLOR_TOKENS.map((token) => (
          <ColorField
            key={token}
            name={`config.theme.colors.${token}`}
            label={t(`site.theme.tokens.${token}`)}
            disabled={disabled}
          />
        ))}
      </FormSection>
      <FormSection
        title={t("site.theme.dark_colors_title")}
        description={t("site.theme.dark_colors_description")}
        columns={2}
      >
        {COLOR_TOKENS.map((token) => (
          <ColorField
            key={token}
            name={`config.theme.dark_colors.${token}`}
            label={t(`site.theme.tokens.${token}`)}
            disabled={disabled}
          />
        ))}
      </FormSection>
      <FormSection
        title={t("site.theme.layout_title")}
        description={t("site.theme.layout_description")}
      >
        <ChoiceCards
          name="config.layout.hero"
          label={t("site.theme.hero")}
          options={HEROES.map((v) => ({
            value: v,
            label: t(`site.theme.heroes.${v}`),
            preview: HERO_PREVIEWS[v],
          }))}
          disabled={disabled}
        />
        <ChoiceCards
          name="config.layout.cards"
          label={t("site.theme.cards")}
          columns={3}
          options={CARDS.map((v) => ({
            value: v,
            label: t(`site.theme.card_variants.${v}`),
            preview: CARD_PREVIEWS[v],
          }))}
          disabled={disabled}
        />
        <ChoiceCards
          name="config.layout.footer"
          label={t("site.theme.footer")}
          columns={3}
          options={FOOTERS.map((v) => ({
            value: v,
            label: t(`site.theme.footers.${v}`),
            preview: FOOTER_PREVIEWS[v],
          }))}
          disabled={disabled}
        />
      </FormSection>
    </div>
  );
}
