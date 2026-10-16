"use client";

import { Info } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useBranchOptions } from "@/features/pricing/hooks/use-pricing-options";
import {
  PERIOD_LABEL_KEYS,
  PERIOD_PRESETS,
  type PeriodPreset,
} from "@/features/profitability/constants";
import { presetRange } from "@/features/profitability/lib/period";
import type { ReportMethod } from "@/features/reports/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export const ALL = "__all__";

/** Page frame of a report: title, breadcrumbs, permission gate. */
export function ReportPage({
  titleKey,
  descriptionKey,
  children,
}: {
  titleKey: string;
  descriptionKey: string;
  children: ReactNode;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <EntityPage
      title={t(titleKey)}
      description={t(descriptionKey)}
      permission={permissions.tenant.finance.reports.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reports.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("reports.title"), href: routes.tenant.reports.root(slug) },
        { label: t(titleKey) },
      ]}
    >
      <div className="space-y-6">{children}</div>
    </EntityPage>
  );
}

export type PeriodState = { preset: PeriodPreset; from: string; to: string };

/** Period (preset or custom days) and branch filter state. */
export function useReportFilters(initial: PeriodPreset = "this_month") {
  const [period, setPeriod] = useState<PeriodState>(() => ({
    preset: initial,
    ...(presetRange(initial) ?? presetRange("this_month")!),
  }));
  const [branch, setBranch] = useState(ALL);
  const params = useMemo(
    () => ({
      from: period.from || undefined,
      to: period.to || undefined,
      branch: branch === ALL ? undefined : branch,
    }),
    [period, branch],
  );
  return { period, setPeriod, branch, setBranch, params };
}

/** One row of filters above the report: period, branch, then `children`. */
export function ReportFilters({
  period,
  onPeriodChange,
  branch,
  onBranchChange,
  showPeriod = true,
  children,
}: {
  period?: PeriodState;
  onPeriodChange?: (next: PeriodState) => void;
  branch: string;
  onBranchChange: (value: string) => void;
  showPeriod?: boolean;
  children?: ReactNode;
}) {
  const { t } = useLocale();
  const branches = useBranchOptions();
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
      {showPeriod && period && onPeriodChange ? (
        <>
          <Select
            value={period.preset}
            onValueChange={(value) => {
              const preset = value as PeriodPreset;
              const range = presetRange(preset);
              onPeriodChange(
                range ? { preset, ...range } : { ...period, preset },
              );
            }}
          >
            <SelectTrigger
              className="w-full sm:w-44"
              aria-label={t("reports.filters.period")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PERIOD_PRESETS.map((p) => (
                <SelectItem key={p} value={p}>
                  {t(PERIOD_LABEL_KEYS[p])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {period.preset === "custom" ? (
            <div className="grid grid-cols-2 gap-2">
              <DatePicker
                value={period.from}
                onChange={(from) => onPeriodChange({ ...period, from })}
                placeholder={t("reports.filters.from")}
              />
              <DatePicker
                value={period.to}
                onChange={(to) => onPeriodChange({ ...period, to })}
                placeholder={t("reports.filters.to")}
              />
            </div>
          ) : null}
        </>
      ) : null}
      {branches.allowed ? (
        <Select value={branch} onValueChange={onBranchChange}>
          <SelectTrigger
            className="w-full sm:w-48"
            aria-label={t("reports.filters.branch")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>
              {t("reports.filters.all_branches")}
            </SelectItem>
            {branches.options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : null}
      {children}
    </div>
  );
}

/** A labelled select of fixed options (grouping, basis, party …). */
export function OptionSelect<T extends string>({
  value,
  options,
  labelKey,
  ariaLabel,
  onChange,
  className = "w-full sm:w-48",
}: {
  value: T;
  options: readonly T[];
  labelKey: (option: T) => string;
  ariaLabel: string;
  onChange: (value: T) => void;
  className?: string;
}) {
  const { t } = useLocale();
  return (
    <Select value={value} onValueChange={(v) => onChange(v as T)}>
      <SelectTrigger className={className} aria-label={ariaLabel}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((o) => (
          <SelectItem key={o} value={o}>
            {t(labelKey(o))}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** "Yöntem ve dönem" card: what the figures mean. */
export function MethodNotes({
  method,
  from,
  to,
  asOf,
  items = [],
  notes,
}: {
  method?: ReportMethod;
  from?: string;
  to?: string;
  asOf?: string;
  items?: { label: string; value: ReactNode }[];
  notes: string[];
}) {
  const { t, locale } = useLocale();
  if (!method) return null;
  const all = [...notes, t("reports.notes.amounts")];
  if (method.branch_scope === "branches") {
    all.push(t("reports.notes.scope_branches"));
  }
  const head: { label: string; value: ReactNode }[] = [];
  if (from && to) {
    head.push({
      label: t("reports.filters.period"),
      value: `${date(from, "dd.MM.yyyy", locale)} – ${date(to, "dd.MM.yyyy", locale)}`,
    });
  }
  if (asOf) {
    head.push({
      label: t("reports.filters.as_of"),
      value: date(asOf, "dd.MM.yyyy", locale),
    });
  }
  return (
    <EntitySectionCard title={t("reports.notes.title")}>
      <div className="space-y-3 text-sm">
        <dl className="grid gap-2 sm:grid-cols-3">
          {[...head, ...items].map((i) => (
            <div key={i.label}>
              <dt className="text-muted-foreground text-xs">{i.label}</dt>
              <dd className="font-medium">{i.value}</dd>
            </div>
          ))}
        </dl>
        <ul className="text-muted-foreground space-y-1.5">
          {all.map((n) => (
            <li key={n} className="flex gap-2">
              <Info className="mt-0.5 size-3.5 shrink-0" aria-hidden />
              <span>{n}</span>
            </li>
          ))}
        </ul>
      </div>
    </EntitySectionCard>
  );
}

/** Error state of a failed report load. */
export function ReportError({ onRetry }: { onRetry: () => void }) {
  const { t } = useLocale();
  return (
    <ErrorState
      title={t("common.error_generic")}
      description={t("reports.load_failed")}
      onRetry={onRetry}
      retryLabel={t("common.retry")}
    />
  );
}
