"use client";

import { Plus, Trash2, WandSparkles } from "lucide-react";
import { get, useFieldArray, useFormContext } from "react-hook-form";
import { z } from "zod";

import { EmptyState } from "@/components/common/empty-state";
import { AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { DEFAULT_BANDS } from "@/features/pricing/constants";
import { isAmount, normalizeAmount } from "@/features/pricing/lib/format";
import type { RateBand, RateBandInput } from "@/features/pricing/types";
import { useLocale } from "@/providers/locale-provider";

/** One band row of a form: strings so the inputs stay controlled. */
export type BandRow = {
  min_days: string;
  max_days: string;
  daily_price: string;
};

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** zod schema of the band rows (ranges, prices, overlaps). */
export function bandRowsSchema(t: Translate) {
  const int = /^\d{1,4}$/;
  return z
    .array(
      z
        .object({
          min_days: z
            .string()
            .trim()
            .refine(
              (v) => int.test(v) && Number(v) >= 1,
              t("pricing.validation.min_days"),
            ),
          max_days: z
            .string()
            .trim()
            .refine(
              (v) => v === "" || int.test(v),
              t("pricing.validation.max_days"),
            ),
          daily_price: z
            .string()
            .trim()
            .refine(isAmount, t("pricing.validation.price")),
        })
        .refine(
          (row) =>
            row.max_days === "" || Number(row.max_days) >= Number(row.min_days),
          {
            path: ["max_days"],
            message: t("pricing.validation.max_before_min"),
          },
        ),
    )
    .superRefine((rows, ctx) => {
      const sorted = rows
        .map((row, index) => ({
          index,
          min: Number(row.min_days),
          max: row.max_days === "" ? Infinity : Number(row.max_days),
        }))
        .filter((r) => Number.isFinite(r.min))
        .sort((a, b) => a.min - b.min);
      for (let i = 1; i < sorted.length; i++) {
        if (sorted[i - 1].max >= sorted[i].min) {
          ctx.addIssue({
            code: "custom",
            path: [sorted[i].index, "min_days"],
            message: t("pricing.validation.overlap"),
          });
        }
      }
    });
}

export function bandsToRows(bands: RateBand[] | undefined): BandRow[] {
  return (bands ?? []).map((b) => ({
    min_days: String(b.min_days),
    max_days: b.max_days == null ? "" : String(b.max_days),
    daily_price: b.daily_price,
  }));
}

export function rowsToBands(rows: BandRow[]): RateBandInput[] {
  return rows.map((row) => ({
    min_days: Number(row.min_days),
    max_days: row.max_days.trim() === "" ? null : Number(row.max_days),
    daily_price: normalizeAmount(row.daily_price),
  }));
}

/**
 * Duration bands of a draft (`bands` field array): "min – max days" (max
 * empty = open-ended) and the VAT-inclusive daily price. The whole rental
 * length picks one band; every day uses its price.
 */
export function BandsEditor({ name = "bands" }: { name?: string }) {
  const { t } = useLocale();
  const {
    control,
    getValues,
    formState: { errors },
  } = useFormContext();
  const { fields, append, remove, replace } = useFieldArray({ control, name });
  const arrayError = get(errors, name)?.message as string | undefined;

  return (
    <div className="space-y-3">
      {fields.length === 0 ? (
        <EmptyState
          className="py-6"
          title={t("pricing.bands.empty")}
          description={t("pricing.bands.empty_hint")}
        />
      ) : null}
      {fields.map((field, index) => {
        const base = `${name}.${index}`;
        return (
          <div
            key={field.id}
            className="border-border grid grid-cols-[1fr_1fr_1.4fr_auto] items-start gap-2 rounded-lg border p-2"
          >
            <AppInput
              name={`${base}.min_days`}
              inputMode="numeric"
              label={index === 0 ? t("pricing.fields.min_days") : undefined}
              aria-label={t("pricing.fields.min_days")}
            />
            <AppInput
              name={`${base}.max_days`}
              inputMode="numeric"
              placeholder="∞"
              label={index === 0 ? t("pricing.fields.max_days") : undefined}
              aria-label={t("pricing.fields.max_days")}
            />
            <AppInput
              name={`${base}.daily_price`}
              inputMode="decimal"
              placeholder="0,00"
              label={index === 0 ? t("pricing.fields.daily_price") : undefined}
              aria-label={t("pricing.fields.daily_price")}
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className={
                index === 0
                  ? "text-destructive hover:text-destructive mt-6 size-9"
                  : "text-destructive hover:text-destructive size-9"
              }
              aria-label={t("pricing.bands.remove")}
              onClick={() => remove(index)}
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        );
      })}
      {arrayError ? (
        <p className="text-destructive text-sm">{arrayError}</p>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => {
            const rows = (getValues(name) as BandRow[] | undefined) ?? [];
            const last = rows[rows.length - 1];
            const nextMin =
              last && last.max_days !== "" ? Number(last.max_days) + 1 : 1;
            append({
              min_days: String(nextMin),
              max_days: "",
              daily_price: "",
            });
          }}
        >
          <Plus className="size-4" />
          {t("pricing.bands.add")}
        </Button>
        {fields.length === 0 ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() =>
              replace(
                DEFAULT_BANDS.map((b) => ({
                  min_days: String(b.min_days),
                  max_days: b.max_days == null ? "" : String(b.max_days),
                  daily_price: "",
                })),
              )
            }
          >
            <WandSparkles className="size-4" />
            {t("pricing.bands.defaults")}
          </Button>
        ) : null}
      </div>
    </div>
  );
}
