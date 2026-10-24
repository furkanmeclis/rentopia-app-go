"use client";

import { useState } from "react";

import { BrandMark } from "@/components/common/brand-mark";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { useSetDisplayBrandMotomarks } from "@/features/catalog/hooks/use-catalog";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

export type MotomarksDialogBrand = {
  uuid: string;
  name: string;
  logo_url?: string | null;
  motomarks?: { id?: string | null; match_source?: string } | null;
  suggestion?: { id: string; name: string; score?: number | null } | null;
};

/**
 * Link a display brand to a logo-service brand by hand, mark it as having
 * no logo, or clear the link so the next sync matches it again.
 */
export function DisplayBrandMotomarksDialog({
  brand,
  onOpenChange,
}: {
  brand: MotomarksDialogBrand | null;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const set = useSetDisplayBrandMotomarks();
  const [value, setValue] = useState("");
  const current = brand?.motomarks?.id ?? "";
  const suggestion = brand?.suggestion ?? null;
  const initialOptions: ComboboxOption[] = [];
  if (suggestion) {
    initialOptions.push({
      value: suggestion.id,
      label: suggestion.name,
      description:
        suggestion.score != null
          ? t("catalog.logos.match_score", {
              score: Math.round(suggestion.score),
            })
          : undefined,
    });
  }

  const close = (next: boolean) => {
    if (set.isPending) return;
    if (!next) setValue("");
    onOpenChange(next);
  };
  const run = async (
    mode: "link" | "skip" | "unlink",
    motomarksId?: string,
  ) => {
    if (!brand) return;
    await set.mutateAsync({ uuid: brand.uuid, mode, motomarksId });
    close(false);
  };

  return (
    <AppDialog
      open={Boolean(brand)}
      onOpenChange={close}
      title={t("catalog.logos.link_title")}
      description={
        brand
          ? t("catalog.logos.link_description", { name: brand.name })
          : undefined
      }
      footer={
        <div className="flex w-full flex-wrap items-center justify-between gap-2">
          <div className="flex gap-2">
            <Button
              type="button"
              variant="ghost"
              disabled={set.isPending}
              onClick={() => void run("skip")}
            >
              {t("catalog.logos.skip")}
            </Button>
            {current ? (
              <Button
                type="button"
                variant="ghost"
                disabled={set.isPending}
                onClick={() => void run("unlink")}
              >
                {t("catalog.logos.unlink")}
              </Button>
            ) : null}
          </div>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={set.isPending}
              onClick={() => close(false)}
            >
              {t("form.cancel")}
            </Button>
            <Button
              type="button"
              disabled={!value || set.isPending}
              onClick={() => void run("link", value)}
            >
              {t("catalog.logos.link")}
            </Button>
          </div>
        </div>
      }
    >
      {brand ? (
        <div className="space-y-4">
          <div className="flex items-center gap-3">
            <BrandMark name={brand.name} logoUrl={brand.logo_url} size="md" />
            <div className="min-w-0">
              <p className="truncate font-medium">{brand.name}</p>
              <p className="text-muted-foreground text-xs">
                {t(
                  `catalog.logos.match_source.${brand.motomarks?.match_source ?? "none"}`,
                )}
                {current ? ` · ${current}` : ""}
              </p>
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="motomarks-source">
              {t("catalog.logos.settings_title")}
            </Label>
            <AsyncCombobox
              id="motomarks-source"
              value={value || current}
              onValueChange={setValue}
              initialOptions={initialOptions}
              loadOptions={async (query) => {
                const res = await catalogService.searchBrandLogoSources(
                  query,
                  20,
                );
                return res.items.map((item) => ({
                  value: item.id,
                  label: item.name,
                  description: item.variants
                    .map((v) => t(`catalog.logos.variant.${v}`))
                    .join(" · "),
                }));
              }}
              placeholder={t("catalog.logos.source_placeholder")}
              searchPlaceholder={t("catalog.logos.source_placeholder")}
              emptyText={t("catalog.logos.source_empty")}
            />
            <p className="text-muted-foreground text-xs">
              {t("catalog.logos.source_hint")}
            </p>
          </div>
          {suggestion && !value ? (
            <Button
              type="button"
              variant="secondary"
              size="sm"
              disabled={set.isPending}
              onClick={() => void run("link", suggestion.id)}
            >
              {t("catalog.logos.link_suggestion")} · {suggestion.name}
              {suggestion.score != null
                ? ` (${Math.round(suggestion.score)})`
                : ""}
            </Button>
          ) : null}
        </div>
      ) : null}
    </AppDialog>
  );
}
