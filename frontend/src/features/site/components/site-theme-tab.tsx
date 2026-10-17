"use client";

import type { ReactNode } from "react";

import { AppSelect, FormSection } from "@/components/forms";
import {
  ChoiceCards,
  ColorField,
} from "@/features/site/components/site-fields";
import {
  CARDS,
  COLOR_TOKENS,
  FONTS,
  FOOTERS,
  HEROES,
  MODES,
  RADII,
} from "@/features/site/schemas/site-form";
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

/**
 * Tema: color tokens (light and dark), font, corner radius, mode and the
 * ready-made layout variants. Draft until published.
 */
export function SiteThemeTab({ disabled }: { disabled?: boolean }) {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
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
      <FormSection title={t("site.theme.type_title")} columns={2}>
        <AppSelect
          name="config.theme.font"
          label={t("site.theme.font")}
          options={FONTS.map((f) => ({
            value: f,
            label: t(`site.theme.fonts.${f}`),
          }))}
          disabled={disabled}
        />
        <AppSelect
          name="config.theme.radius"
          label={t("site.theme.radius")}
          options={RADII.map((r) => ({
            value: r,
            label: t(`site.theme.radii.${r}`),
          }))}
          disabled={disabled}
        />
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
