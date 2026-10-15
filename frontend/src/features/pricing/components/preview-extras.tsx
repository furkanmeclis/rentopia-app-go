"use client";

import { BadgeCheck, Plus, TicketX, Trash2 } from "lucide-react";
import { useMemo } from "react";
import { useFieldArray, useFormContext } from "react-hook-form";

import { AppInput, AppSelect } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { COUPON_REASON_KEYS } from "@/features/campaigns/constants";
import { CHARGE_LABEL_KEYS } from "@/features/extras/constants";
import { useExtrasList } from "@/features/extras/hooks/use-extras";
import { moneyText } from "@/features/pricing/lib/format";
import type { components } from "@/generated/api";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type CouponResult = components["schemas"]["PricingCouponResult"];

/** One selected extra of the preview form (strings keep inputs controlled). */
export type PreviewExtraRow = { extra_uuid: string; quantity: string };

/**
 * Extras of the preview ("Ek hizmetler"): published extras of the catalog
 * with a quantity; the API prices them per rental product. Hidden without
 * `tenant.extras.read`.
 */
export function PreviewExtrasEditor({ name = "extras" }: { name?: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.extras.read);
  const { control } = useFormContext();
  const { fields, append, remove } = useFieldArray({ control, name });
  const query = useExtrasList(
    { limit: 100, offset: 0, status: "published", sort: "sort_order" },
    allowed,
  );
  const options = useMemo(
    () =>
      (query.data?.items ?? []).map((e) => ({
        value: e.uuid,
        label: `${e.name} · ${t(CHARGE_LABEL_KEYS[e.charge_type])}`,
      })),
    [query.data, t],
  );
  if (!allowed) return null;
  return (
    <div className="space-y-3">
      {fields.map((field, index) => (
        <div
          key={field.id}
          className="border-border grid grid-cols-[1fr_6rem_auto] items-start gap-2 rounded-lg border p-2"
        >
          <AppSelect
            name={`${name}.${index}.extra_uuid`}
            label={index === 0 ? t("pricing.preview.extra") : undefined}
            placeholder={t("pricing.preview.extra_placeholder")}
            options={options}
          />
          <AppInput
            name={`${name}.${index}.quantity`}
            inputMode="numeric"
            label={
              index === 0 ? t("pricing.preview.extra_quantity") : undefined
            }
            aria-label={t("pricing.preview.extra_quantity")}
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
            aria-label={t("pricing.preview.remove_extra")}
            onClick={() => remove(index)}
          >
            <Trash2 className="size-4" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={options.length === 0}
        onClick={() => append({ extra_uuid: "", quantity: "1" })}
      >
        <Plus className="size-4" />
        {t("pricing.preview.add_extra")}
      </Button>
    </div>
  );
}

/** Applied / rejected coupon of one preview side, with the reason. */
export function CouponResultAlert({ coupon }: { coupon: CouponResult }) {
  const { t, locale } = useLocale();
  if (coupon.status === "applied") {
    return (
      <Alert>
        <BadgeCheck className="size-4" />
        <AlertTitle>
          {coupon.campaign_type === "repeat_customer"
            ? t("pricing.preview.coupon_auto", { name: coupon.campaign_name })
            : t("pricing.preview.coupon_applied", {
                code: coupon.code,
                name: coupon.campaign_name,
              })}
        </AlertTitle>
      </Alert>
    );
  }
  const key = COUPON_REASON_KEYS[coupon.reason];
  const params: Record<string, string> = { ...(coupon.params ?? {}) };
  for (const name of ["start_date", "end_date"]) {
    if (params[name]) params[name] = date(params[name], "d MMM yyyy", locale);
  }
  for (const name of ["min_subtotal", "subtotal"]) {
    if (params[name]) params[name] = moneyText(params[name], locale);
  }
  return (
    <Alert variant="destructive">
      <TicketX className="size-4" />
      <AlertTitle>
        {t("pricing.preview.coupon_rejected", { code: coupon.code })}
      </AlertTitle>
      <AlertDescription>
        {key ? t(key, params) : coupon.reason}
      </AlertDescription>
    </Alert>
  );
}
