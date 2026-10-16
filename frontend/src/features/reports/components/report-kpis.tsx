"use client";

import type { ReactNode } from "react";

import { StatsCard } from "@/components/common/stats-card";
import { Skeleton } from "@/components/ui/skeleton";

export type Kpi = {
  key: string;
  title: string;
  value: ReactNode;
  hint?: string;
};

/** KPI tiles above a report (skeletons while loading). */
export function ReportKpis({
  items,
  loading,
  count,
}: {
  items: Kpi[];
  loading?: boolean;
  count?: number;
}) {
  const n = count ?? items.length;
  const cols =
    n >= 5
      ? "grid-cols-2 lg:grid-cols-3 xl:grid-cols-5"
      : "grid-cols-2 lg:grid-cols-4";
  if (loading) {
    return (
      <div className={`grid gap-3 ${cols}`}>
        {Array.from({ length: n }).map((_, i) => (
          <Skeleton key={i} className="h-[104px] rounded-xl" />
        ))}
      </div>
    );
  }
  return (
    <div className={`grid gap-3 ${cols}`}>
      {items.map((k) => (
        <StatsCard key={k.key} title={k.title} value={k.value} hint={k.hint} />
      ))}
    </div>
  );
}
