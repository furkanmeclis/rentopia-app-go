"use client";

import { Plus, X } from "lucide-react";
import { useCallback, useRef, useState } from "react";

import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { fleetService } from "@/features/fleet/services/fleet.service";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

/** A picked platform catalog model or type with its display label. */
export type CatalogPick = { uuid: string; label: string };

type Selection = {
  brand: ComboboxOption | null;
  model: ComboboxOption | null;
  type: ComboboxOption | null;
};

const EMPTY: Selection = { brand: null, model: null, type: null };

function remember(
  known: Map<string, ComboboxOption>,
  options: ComboboxOption[],
): ComboboxOption[] {
  for (const option of options) known.set(option.value, option);
  return options;
}

/**
 * Brand → model → type cascade over the read-only tenant catalog endpoints
 * (`/v1/tenant/catalog/*`). The caller decides what a pick means (add to a
 * list, set a single reference).
 */
export function CatalogCascade({
  allowModel = true,
  allowType = true,
  onPickModel,
  onPickType,
  modelActionLabel,
  typeActionLabel,
  disabled,
}: {
  allowModel?: boolean;
  allowType?: boolean;
  onPickModel?: (pick: CatalogPick) => void;
  onPickType?: (pick: CatalogPick) => void;
  modelActionLabel?: string;
  typeActionLabel?: string;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [sel, setSel] = useState<Selection>(EMPTY);
  // Options seen in any search, to resolve the label of a picked value.
  const known = useRef(new Map<string, ComboboxOption>());
  const lookup = (value: string): ComboboxOption =>
    known.current.get(value) ?? { value, label: value };

  const loadBrands = useCallback(
    async (q: string) => {
      const page = await fleetService.catalogBrands(slug, q);
      return remember(
        known.current,
        page.items.map((b) => ({ value: b.uuid, label: b.display_name })),
      );
    },
    [slug],
  );
  const brandUuid = sel.brand?.value ?? "";
  const loadModels = useCallback(
    async (q: string) => {
      if (!brandUuid) return [];
      const page = await fleetService.catalogModels(slug, brandUuid, q);
      return remember(
        known.current,
        page.items.map((m) => ({ value: m.uuid, label: m.name })),
      );
    },
    [brandUuid, slug],
  );
  const modelUuid = sel.model?.value ?? "";
  const loadTypes = useCallback(
    async (q: string) => {
      if (!modelUuid) return [];
      const page = await fleetService.catalogTypes(slug, modelUuid, q);
      return remember(
        known.current,
        page.items.map((ty) => ({
          value: ty.uuid,
          label: ty.name,
          description: ty.years.length
            ? `${Math.min(...ty.years)}–${Math.max(...ty.years)}`
            : undefined,
        })),
      );
    },
    [modelUuid, slug],
  );

  const brandLabel = sel.brand?.label ?? "";
  const modelLabel = sel.model ? `${brandLabel} ${sel.model.label}`.trim() : "";
  const typeLabel = sel.type
    ? sel.type.label.toLowerCase().startsWith(brandLabel.toLowerCase() + " ")
      ? sel.type.label
      : `${brandLabel} ${sel.type.label}`.trim()
    : "";

  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <div className="grid gap-1.5">
        <Label>{t("fleet.catalog.brand")}</Label>
        <AsyncCombobox
          value={brandUuid}
          initialOptions={sel.brand ? [sel.brand] : undefined}
          loadOptions={loadBrands}
          placeholder={t("fleet.catalog.brand_placeholder")}
          disabled={disabled}
          onValueChange={(value) => {
            if (!value) {
              setSel(EMPTY);
              return;
            }
            const brand = lookup(value);
            setSel({ brand, model: null, type: null });
          }}
        />
      </div>
      <div className="grid gap-1.5">
        <Label>{t("fleet.catalog.model")}</Label>
        <AsyncCombobox
          key={brandUuid || "no-brand"}
          value={modelUuid}
          initialOptions={sel.model ? [sel.model] : undefined}
          loadOptions={loadModels}
          placeholder={t("fleet.catalog.model_placeholder")}
          disabled={disabled || !brandUuid}
          onValueChange={(value) => {
            if (!value) {
              setSel((prev) => ({ ...prev, model: null, type: null }));
              return;
            }
            const model = lookup(value);
            setSel((prev) => ({ ...prev, model, type: null }));
          }}
        />
        {allowModel && sel.model ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="justify-start"
            disabled={disabled}
            onClick={() =>
              onPickModel?.({ uuid: sel.model!.value, label: modelLabel })
            }
          >
            <Plus className="size-4" />
            {modelActionLabel ?? t("fleet.catalog.pick_model")}
          </Button>
        ) : null}
      </div>
      {allowType ? (
        <div className="grid gap-1.5">
          <Label>{t("fleet.catalog.type")}</Label>
          <AsyncCombobox
            key={modelUuid || "no-model"}
            value={sel.type?.value ?? ""}
            initialOptions={sel.type ? [sel.type] : undefined}
            loadOptions={loadTypes}
            placeholder={t("fleet.catalog.type_placeholder")}
            disabled={disabled || !modelUuid}
            onValueChange={(value) => {
              if (!value) {
                setSel((prev) => ({ ...prev, type: null }));
                return;
              }
              const type = lookup(value);
              setSel((prev) => ({ ...prev, type }));
            }}
          />
          {sel.type ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="justify-start"
              disabled={disabled}
              onClick={() =>
                onPickType?.({ uuid: sel.type!.value, label: typeLabel })
              }
            >
              <Plus className="size-4" />
              {typeActionLabel ?? t("fleet.catalog.pick_type")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

/** Removable chips of picked catalog rows. */
export function CatalogChips({
  items,
  onRemove,
  emptyText,
  disabled,
}: {
  items: CatalogPick[];
  onRemove: (uuid: string) => void;
  emptyText: string;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  if (!items.length) {
    return <p className="text-muted-foreground text-sm">{emptyText}</p>;
  }
  return (
    <div className="flex flex-wrap gap-2">
      {items.map((item) => (
        <Badge key={item.uuid} variant="secondary" className="gap-1 pe-1">
          {item.label}
          <button
            type="button"
            className="hover:bg-muted-foreground/20 rounded-sm p-0.5"
            aria-label={t("fleet.catalog.remove", { label: item.label })}
            disabled={disabled}
            onClick={() => onRemove(item.uuid)}
          >
            <X className="size-3" />
          </button>
        </Badge>
      ))}
    </div>
  );
}
