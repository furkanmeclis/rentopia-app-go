"use client";

import { useMemo, useState, type ReactNode } from "react";

import { DatePicker } from "@/components/ui/date-picker";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  ALLOCATIONS,
  ALLOCATION_LABEL_KEYS,
  BASES,
  BASIS_LABEL_KEYS,
  PERIOD_LABEL_KEYS,
  PERIOD_PRESETS,
  type PeriodPreset,
} from "@/features/profitability/constants";
import { presetRange } from "@/features/profitability/lib/period";
import type {
  ProfitabilityAllocation,
  ProfitabilityBasis,
  ProfitabilityParams,
} from "@/features/profitability/types";
import { useLocale } from "@/providers/locale-provider";

export type ProfitabilityFilterState = {
  preset: PeriodPreset;
  from: string;
  to: string;
  basis: ProfitabilityBasis;
  allocation: ProfitabilityAllocation;
};

/** Filter state shared by the fleet report and the vehicle tab. */
export function useProfitabilityFilters(initial: PeriodPreset) {
  const [state, setState] = useState<ProfitabilityFilterState>(() => {
    const range = presetRange(initial) ?? presetRange("this_month")!;
    return {
      preset: initial,
      ...range,
      basis: "accrual",
      allocation: "rental_days",
    };
  });
  const params = useMemo<ProfitabilityParams>(
    () => ({
      from: state.from || undefined,
      to: state.to || undefined,
      basis: state.basis,
      allocation: state.allocation,
    }),
    [state],
  );
  return { state, setState, params };
}

/**
 * Period (preset or custom days), basis (accrual / cash) and overhead
 * allocation method; `children` adds page-specific filters on the same row.
 */
export function ProfitabilityFilters({
  state,
  onChange,
  children,
}: {
  state: ProfitabilityFilterState;
  onChange: (next: ProfitabilityFilterState) => void;
  children?: ReactNode;
}) {
  const { t } = useLocale();
  const set = (patch: Partial<ProfitabilityFilterState>) =>
    onChange({ ...state, ...patch });

  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:flex-wrap sm:items-center">
      <Select
        value={state.preset}
        onValueChange={(value) => {
          const preset = value as PeriodPreset;
          const range = presetRange(preset);
          set(range ? { preset, ...range } : { preset });
        }}
      >
        <SelectTrigger
          className="w-full sm:w-44"
          aria-label={t("profitability.filters.period")}
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
      {state.preset === "custom" ? (
        <div className="grid grid-cols-2 gap-2">
          <DatePicker
            value={state.from}
            onChange={(from) => set({ from })}
            placeholder={t("profitability.filters.from")}
          />
          <DatePicker
            value={state.to}
            onChange={(to) => set({ to })}
            placeholder={t("profitability.filters.to")}
          />
        </div>
      ) : null}
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        value={state.basis}
        onValueChange={(value) => {
          if (value) set({ basis: value as ProfitabilityBasis });
        }}
        aria-label={t("profitability.filters.basis")}
        className="w-full sm:w-auto"
      >
        {BASES.map((b) => (
          <ToggleGroupItem key={b} value={b} className="flex-1 sm:flex-none">
            {t(BASIS_LABEL_KEYS[b])}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
      <Select
        value={state.allocation}
        onValueChange={(value) =>
          set({ allocation: value as ProfitabilityAllocation })
        }
      >
        <SelectTrigger
          className="w-full sm:w-52"
          aria-label={t("profitability.filters.allocation")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {ALLOCATIONS.map((a) => (
            <SelectItem key={a} value={a}>
              {t(ALLOCATION_LABEL_KEYS[a])}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      {children}
    </div>
  );
}
