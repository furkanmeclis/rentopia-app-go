"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { AlertTriangle, ArrowRight, Calculator } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useMemo } from "react";
import { z } from "zod";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityDetail,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  FormSection,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  localInputFromNow,
  localInputToISO,
} from "@/features/fleet-ops/lib/datetime";
import {
  DateRuleFields,
  dateRuleBody,
  dateRuleDefaults,
  type DateRuleValues,
} from "@/features/pricing/components/date-rule-form";
import {
  CouponResultAlert,
  PreviewExtrasEditor,
  type PreviewExtraRow,
} from "@/features/pricing/components/preview-extras";
import { VersionBadge } from "@/features/pricing/components/pricing-shared";
import {
  LINE_LABEL_KEYS,
  PREPAYMENT_LABEL_KEYS,
  PREVIEW_ERROR_KEYS,
  PREVIEW_WARNING_KEYS,
  SCOPE_LABEL_KEYS,
} from "@/features/pricing/constants";
import {
  useDateRulesList,
  usePricingPreview,
  useRatePlansList,
} from "@/features/pricing/hooks/use-pricing";
import {
  useBranchOptions,
  useChannelOptions,
  useProductOptions,
} from "@/features/pricing/hooks/use-pricing-options";
import {
  bandLabel,
  isAmount,
  moneyText,
  normalizeAmount,
  ruleValueText,
  SIGNED_RE,
} from "@/features/pricing/lib/format";
import type {
  PricingDayLine,
  PricingPreviewRequest,
  PricingPreviewSide,
  RatePlanScope,
} from "@/features/pricing/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { cn } from "@/lib/utils";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Mode = "none" | "plan" | "rule" | "new_rule";

type Values = DateRuleValues & {
  rental_product_uuid: string;
  branch_uuid: string;
  return_branch_uuid: string;
  channel: string;
  start_at: string;
  end_at: string;
  delivery_fee: string;
  mode: Mode;
  rate_plan_uuid: string;
  date_rule_uuid: string;
  extras: PreviewExtraRow[];
  coupon_code: string;
};

const SAME_BRANCH = "same";

function DaysTable({ days }: { days: PricingDayLine[] }) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<PricingDayLine>({
          accessorKey: "index",
          labelKey: "pricing.preview.columns.day",
          enableSorting: false,
        }),
        createColumn<PricingDayLine>({
          accessorKey: "date",
          labelKey: "pricing.preview.columns.date",
          enableSorting: false,
          cell: ({ row }) => date(row.original.date, "EEE d MMM", locale),
        }),
        createColumn<PricingDayLine>({
          accessorKey: "base_price",
          labelKey: "pricing.preview.columns.base_price",
          enableSorting: false,
          cell: ({ row }) => moneyText(row.original.base_price, locale),
        }),
        createColumn<PricingDayLine>({
          id: "rule",
          accessorFn: (row) => row.rule?.name ?? "",
          labelKey: "pricing.preview.columns.rule",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.rule ? (
              <span>
                {row.original.rule.name}{" "}
                <span className="text-muted-foreground text-xs">
                  v{row.original.rule.version} ·{" "}
                  {ruleValueText(
                    row.original.rule.kind,
                    row.original.rule.kind === "percent"
                      ? (row.original.rule.percent ?? "0")
                      : (row.original.rule.amount ?? "0"),
                    locale,
                  )}
                </span>
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<PricingDayLine>({
          accessorKey: "unit_price",
          labelKey: "pricing.preview.columns.unit_price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-medium tabular-nums">
              {moneyText(row.original.unit_price, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<PricingDayLine, unknown>[],
    [locale],
  );
  return (
    <EntityTable
      columns={columns}
      data={days}
      getRowId={(row) => String(row.index)}
      emptyTitle={t("pricing.preview.no_days")}
      features={{
        persistKey: "tenant-pricing-preview-days-v1",
        rowSelection: false,
        globalFilter: false,
        columnFilters: false,
        facetedFilters: false,
        sorting: false,
        viewMode: false,
        pagination: false,
      }}
      initialState={{ pagination: { pageIndex: 0, pageSize: 400 } }}
    />
  );
}

function SideCard({
  title,
  side,
  highlight,
}: {
  title: string;
  side: PricingPreviewSide;
  highlight?: boolean;
}) {
  const { t, locale } = useLocale();
  if (side.error || !side.breakdown) {
    const key = side.error ? PREVIEW_ERROR_KEYS[side.error.code] : undefined;
    return (
      <EntitySectionCard title={title}>
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{t("pricing.preview.no_price")}</AlertTitle>
          <AlertDescription>
            {key ? t(key) : side.error?.message}
          </AlertDescription>
        </Alert>
      </EntitySectionCard>
    );
  }
  const b = side.breakdown;
  return (
    <EntitySectionCard
      title={title}
      className={cn(highlight && "ring-primary/40 ring-2")}
    >
      <div className="space-y-4">
        {side.coupon ? <CouponResultAlert coupon={side.coupon} /> : null}
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <span
            className="font-display text-3xl font-semibold tabular-nums"
            data-testid="preview-total"
          >
            {moneyText(b.total, locale)}
          </span>
          <span className="text-muted-foreground text-sm">
            {t("pricing.preview.days_band", {
              days: b.billing_days,
              band: bandLabel(t, b.band.min_days, b.band.max_days),
            })}
          </span>
        </div>
        <EntityDetail
          sections={[
            {
              id: "summary",
              fields: [
                {
                  key: "plan",
                  label: t("pricing.preview.plan"),
                  value: (
                    <span className="flex flex-wrap items-center gap-1">
                      {b.plan.name ||
                        t(
                          SCOPE_LABEL_KEYS[b.plan.scope as RatePlanScope] ??
                            "pricing.scope.common",
                        )}
                      <VersionBadge version={b.plan.version} />
                    </span>
                  ),
                },
                {
                  key: "daily",
                  label: t("pricing.preview.band_price"),
                  value: moneyText(b.band.daily_price, locale),
                },
                {
                  key: "subtotal",
                  label: t("pricing.preview.rental_subtotal"),
                  value: moneyText(b.rental_subtotal, locale),
                },
                ...(b.discount
                  ? [
                      {
                        key: "discount",
                        label: b.discount.code
                          ? t("pricing.preview.discount_code", {
                              code: b.discount.code,
                            })
                          : t("pricing.preview.discount"),
                        value: (
                          <span className="flex flex-wrap items-center gap-1">
                            {`−${moneyText(b.discount.amount, locale)}`}
                            {b.discount.capped ? (
                              <StatusChip
                                label={t("pricing.preview.capped")}
                                tone="warning"
                              />
                            ) : null}
                          </span>
                        ),
                      },
                    ]
                  : []),
                ...b.lines
                  .filter((l) => l.kind !== "rental")
                  .map((l, i) => ({
                    key: `line-${i}`,
                    label:
                      l.kind === "extra" && l.name
                        ? t("pricing.preview.extra_line", {
                            name: l.name,
                            quantity: l.quantity,
                          })
                        : t(
                            LINE_LABEL_KEYS[l.kind] ??
                              "pricing.preview.lines.extra",
                          ),
                    value: (
                      <span className="flex flex-wrap items-center gap-1">
                        {moneyText(l.gross, locale)}
                        {l.capped ? (
                          <StatusChip
                            label={t("pricing.preview.capped")}
                            tone="warning"
                          />
                        ) : null}
                      </span>
                    ),
                  })),
                {
                  key: "base",
                  label: t("pricing.preview.base_total"),
                  value: moneyText(b.base_total, locale),
                },
                {
                  key: "vat",
                  label: b.vat_rate
                    ? t("pricing.preview.vat_rate", {
                        rate: String(Number(b.vat_rate)),
                      })
                    : t("pricing.preview.vat"),
                  value: moneyText(b.vat_total, locale),
                },
                {
                  key: "prepayment",
                  label: t(
                    PREPAYMENT_LABEL_KEYS[b.prepayment_rule] ??
                      "pricing.preview.prepayment.none",
                  ),
                  value: moneyText(b.prepayment, locale),
                },
              ],
            },
          ]}
        />
        {b.rules.length ? (
          <div className="flex flex-wrap gap-1">
            {b.rules.map((r) => (
              <StatusChip
                key={`${r.id}-${r.version}`}
                label={`${r.name} v${r.version}`}
                tone="warning"
              />
            ))}
          </div>
        ) : null}
        <DaysTable days={b.days} />
      </div>
    </EntitySectionCard>
  );
}

/**
 * Price preview ("Fiyat önizleme"): one rental priced with the active plans
 * and rules, and with a draft plan / rule (saved or typed here) — old vs new
 * before anything goes live.
 */
export function PricePreviewPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const search = useSearchParams();
  const products = useProductOptions();
  const branches = useBranchOptions();
  const channels = useChannelOptions();
  const preview = usePricingPreview();
  const draftPlans = useRatePlansList({
    limit: 100,
    offset: 0,
    has_draft: "true",
    sort: "rental_product",
  });
  const draftRules = useDateRulesList({
    limit: 100,
    offset: 0,
    has_draft: "true",
    sort: "start_date",
  });
  const initialPlan = search.get("plan") ?? "";
  const initialRule = search.get("rule") ?? "";

  const schema = useMemo(
    () =>
      z
        .object({
          rental_product_uuid: z
            .string()
            .min(1, t("pricing.validation.product")),
          branch_uuid: z.string().min(1, t("pricing.validation.branch")),
          return_branch_uuid: z.string(),
          channel: z.string().min(1, t("pricing.validation.channel")),
          start_at: z.string().min(1, t("pricing.validation.start_date")),
          end_at: z.string().min(1, t("pricing.validation.end_date")),
          delivery_fee: z
            .string()
            .refine(
              (v) => v.trim() === "" || isAmount(v),
              t("pricing.validation.price"),
            ),
          extras: z.array(
            z.object({
              extra_uuid: z.string().min(1, t("pricing.validation.extra")),
              quantity: z
                .string()
                .trim()
                .refine(
                  (v) => /^\d{1,3}$/.test(v) && Number(v) >= 1,
                  t("pricing.validation.extra_quantity"),
                ),
            }),
          ),
          coupon_code: z.string().max(64),
          mode: z.enum(["none", "plan", "rule", "new_rule"]),
          rate_plan_uuid: z.string(),
          date_rule_uuid: z.string(),
          start_date: z.string(),
          end_date: z.string(),
          value: z.string(),
        })
        .passthrough()
        .refine((v) => v.end_at > v.start_at, {
          path: ["end_at"],
          message: t("pricing.validation.end_after_start"),
        })
        .refine((v) => v.mode !== "plan" || v.rate_plan_uuid !== "", {
          path: ["rate_plan_uuid"],
          message: t("pricing.validation.plan"),
        })
        .refine((v) => v.mode !== "new_rule" || v.start_date !== "", {
          path: ["start_date"],
          message: t("pricing.validation.start_date"),
        })
        .refine((v) => v.mode !== "new_rule" || v.end_date > v.start_date, {
          path: ["end_date"],
          message: t("pricing.validation.end_after_start"),
        })
        .refine(
          (v) =>
            v.mode !== "new_rule" ||
            (SIGNED_RE.test(normalizeAmount(v.value)) &&
              Number(normalizeAmount(v.value)) !== 0),
          { path: ["value"], message: t("pricing.validation.value") },
        )
        .refine((v) => v.mode !== "rule" || v.date_rule_uuid !== "", {
          path: ["date_rule_uuid"],
          message: t("pricing.validation.rule"),
        }),
    [t],
  ) as unknown as z.ZodType<Values>;

  const result = preview.data;
  const difference = result?.difference ? Number(result.difference) : null;

  return (
    <EntityPage
      title={t("pricing.preview.title")}
      description={t("pricing.preview.description")}
      permission={permissions.tenant.pricing.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("pricing.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("layout.section_pricing") },
        { label: t("pricing.preview.title") },
      ]}
    >
      <div className="grid gap-6 xl:grid-cols-[minmax(22rem,28rem)_1fr]">
        <AppForm<Values>
          schema={schema}
          className="space-y-4"
          defaultValues={{
            ...dateRuleDefaults(null),
            name: "",
            rental_product_uuid: search.get("product") ?? "",
            branch_uuid: "",
            return_branch_uuid: SAME_BRANCH,
            channel: "web",
            start_at: localInputFromNow(24),
            end_at: localInputFromNow(24 * 9),
            delivery_fee: "",
            mode: initialPlan ? "plan" : initialRule ? "rule" : "none",
            rate_plan_uuid: initialPlan,
            date_rule_uuid: initialRule,
            extras: [],
            coupon_code: "",
          }}
          onSubmit={async (values) => {
            const body: PricingPreviewRequest = {
              rental_product_uuid: values.rental_product_uuid,
              branch_uuid: values.branch_uuid,
              return_branch_uuid:
                values.return_branch_uuid === SAME_BRANCH
                  ? null
                  : values.return_branch_uuid,
              channel: values.channel,
              start_at: localInputToISO(values.start_at) ?? values.start_at,
              end_at: localInputToISO(values.end_at) ?? values.end_at,
              delivery_fee: values.delivery_fee.trim()
                ? normalizeAmount(values.delivery_fee)
                : null,
            };
            if (values.extras.length) {
              body.extras = values.extras.map((e) => ({
                extra_uuid: e.extra_uuid,
                quantity: Number(e.quantity),
              }));
            }
            if (values.coupon_code.trim()) {
              body.coupon_code = values.coupon_code.trim();
            }
            if (values.mode === "plan")
              body.rate_plan_uuid = values.rate_plan_uuid;
            if (values.mode === "rule")
              body.date_rule_uuid = values.date_rule_uuid;
            if (values.mode === "new_rule") {
              const { name, ...rest } = dateRuleBody(values);
              body.date_rule_draft = {
                ...rest,
                name: name || t("pricing.preview.draft_rule_name"),
              };
            }
            await preview.mutateAsync(body);
          }}
        >
          {(form) => {
            const mode = form.watch("mode");
            const product = form.watch("rental_product_uuid");
            const planOptions = (draftPlans.data?.items ?? [])
              .filter((p) => !product || p.rental_product.uuid === product)
              .map((p) => ({
                value: p.uuid,
                label: `${p.rental_product.code} · ${t(SCOPE_LABEL_KEYS[p.scope])}${p.name ? ` · ${p.name}` : ""}`,
              }));
            const ruleOptions = (draftRules.data?.items ?? []).map((r) => ({
              value: r.uuid,
              label: r.name,
            }));
            return (
              <>
                <FormSection title={t("pricing.preview.section_rental")}>
                  <AppSelect
                    name="rental_product_uuid"
                    label={t("pricing.fields.rental_product")}
                    placeholder={t("pricing.fields.product_placeholder")}
                    options={products.options}
                  />
                  <div className="grid gap-4 sm:grid-cols-2">
                    <AppSelect
                      name="branch_uuid"
                      label={t("pricing.preview.pickup_branch")}
                      placeholder={t("pricing.fields.branch_placeholder")}
                      options={branches.options}
                    />
                    <AppSelect
                      name="return_branch_uuid"
                      label={t("pricing.preview.return_branch")}
                      options={[
                        {
                          value: SAME_BRANCH,
                          label: t("pricing.preview.same_branch"),
                        },
                        ...branches.options,
                      ]}
                    />
                  </div>
                  <AppSelect
                    name="channel"
                    label={t("pricing.fields.channel")}
                    options={channels}
                  />
                  <div className="grid gap-4 sm:grid-cols-2">
                    <AppDatePicker
                      name="start_at"
                      withTime
                      label={t("pricing.preview.start_at")}
                    />
                    <AppDatePicker
                      name="end_at"
                      withTime
                      label={t("pricing.preview.end_at")}
                    />
                  </div>
                  <AppInput
                    name="delivery_fee"
                    inputMode="decimal"
                    label={t("pricing.preview.delivery_fee")}
                    placeholder="0,00"
                  />
                </FormSection>
                <FormSection title={t("pricing.preview.section_extras")}>
                  <PreviewExtrasEditor />
                  <AppInput
                    name="coupon_code"
                    label={t("pricing.preview.coupon_code")}
                    description={t("pricing.preview.coupon_code_hint")}
                    placeholder="YAZ20"
                  />
                </FormSection>
                <FormSection title={t("pricing.preview.section_change")}>
                  <AppSelect
                    name="mode"
                    label={t("pricing.preview.compare_with")}
                    options={[
                      { value: "none", label: t("pricing.preview.mode.none") },
                      { value: "plan", label: t("pricing.preview.mode.plan") },
                      { value: "rule", label: t("pricing.preview.mode.rule") },
                      {
                        value: "new_rule",
                        label: t("pricing.preview.mode.new_rule"),
                      },
                    ]}
                  />
                  {mode === "plan" ? (
                    <AppSelect
                      name="rate_plan_uuid"
                      label={t("pricing.preview.draft_plan")}
                      placeholder={t("pricing.preview.draft_plan_placeholder")}
                      options={planOptions}
                    />
                  ) : null}
                  {mode === "rule" ? (
                    <AppSelect
                      name="date_rule_uuid"
                      label={t("pricing.preview.draft_rule")}
                      placeholder={t("pricing.preview.draft_rule_placeholder")}
                      options={ruleOptions}
                    />
                  ) : null}
                </FormSection>
                {mode === "new_rule" ? <DateRuleFields /> : null}
                <Button
                  type="submit"
                  className="w-full"
                  disabled={preview.isPending}
                >
                  <Calculator className="size-4" />
                  {t("pricing.preview.run")}
                </Button>
              </>
            );
          }}
        </AppForm>

        <div className="space-y-4">
          {!result ? (
            <EmptyState
              className="py-16"
              title={t("pricing.preview.empty_title")}
              description={t("pricing.preview.empty_description")}
            />
          ) : (
            <>
              {result.warnings.filter((w) => w !== "no_changes").length ? (
                <Alert>
                  <AlertTriangle className="size-4" />
                  <AlertTitle>{t("pricing.preview.warnings_title")}</AlertTitle>
                  <AlertDescription>
                    <ul className="list-disc pl-4">
                      {result.warnings
                        .filter((w) => w !== "no_changes")
                        .map((w) => (
                          <li key={w}>
                            {PREVIEW_WARNING_KEYS[w]
                              ? t(PREVIEW_WARNING_KEYS[w])
                              : w}
                          </li>
                        ))}
                    </ul>
                  </AlertDescription>
                </Alert>
              ) : null}
              {difference != null && !result.warnings.includes("no_changes") ? (
                <div
                  className="bg-muted/40 flex flex-wrap items-center gap-3 rounded-xl border px-4 py-3 text-sm"
                  data-testid="preview-difference"
                >
                  <span className="tabular-nums">
                    {moneyText(result.current.breakdown?.total, locale)}
                  </span>
                  <ArrowRight className="size-4" />
                  <span className="font-semibold tabular-nums">
                    {moneyText(result.proposed.breakdown?.total, locale)}
                  </span>
                  <StatusChip
                    label={`${difference > 0 ? "+" : ""}${moneyText(result.difference, locale)}`}
                    tone={
                      difference > 0
                        ? "warning"
                        : difference < 0
                          ? "success"
                          : "default"
                    }
                  />
                </div>
              ) : null}
              <div className="grid gap-4 2xl:grid-cols-2">
                <SideCard
                  title={t("pricing.preview.current")}
                  side={result.current}
                />
                {result.warnings.includes("no_changes") ? null : (
                  <SideCard
                    title={t("pricing.preview.proposed")}
                    side={result.proposed}
                    highlight
                  />
                )}
              </div>
            </>
          )}
        </div>
      </div>
    </EntityPage>
  );
}
