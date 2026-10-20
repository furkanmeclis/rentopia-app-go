"use client";

import { motion } from "motion/react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type BarKind = "rent" | "reserved" | "transfer" | "maintenance";

type Row = {
  plate: string;
  model: string;
  bars: { start: number; span: number; kind: BarKind }[];
};

// Sample vehicles; plates are made up.
const ROWS: Row[] = [
  {
    plate: "34 RTP 482",
    model: "Fiat Egea Cross",
    bars: [
      { start: 1, span: 4, kind: "rent" },
      { start: 5, span: 1, kind: "transfer" },
      { start: 6, span: 2, kind: "reserved" },
    ],
  },
  {
    plate: "06 RTY 118",
    model: "Renault Clio",
    bars: [
      { start: 2, span: 3, kind: "reserved" },
      { start: 6, span: 2, kind: "maintenance" },
    ],
  },
  {
    plate: "35 RNT 907",
    model: "Toyota Corolla",
    bars: [
      { start: 1, span: 2, kind: "maintenance" },
      { start: 3, span: 5, kind: "rent" },
    ],
  },
  {
    plate: "16 RPA 351",
    model: "Dacia Duster",
    bars: [
      { start: 1, span: 1, kind: "transfer" },
      { start: 2, span: 2, kind: "rent" },
      { start: 5, span: 3, kind: "reserved" },
    ],
  },
];

const BAR_CLASS: Record<BarKind, string> = {
  rent: "bg-[#2E358C] text-white dark:bg-[#8E95EC] dark:text-[#141527]",
  reserved:
    "border border-[#2E358C]/40 bg-[#ECEDF8] text-[#2E358C] dark:border-[#8E95EC]/40 dark:bg-[#22264A] dark:text-[#C7CAF6]",
  transfer:
    "bg-amber-100 text-amber-900 dark:bg-amber-400/15 dark:text-amber-200",
  maintenance:
    "bg-[repeating-linear-gradient(135deg,var(--muted)_0_6px,transparent_6px_12px)] border border-border text-muted-foreground",
};

const LABEL_KEY: Record<BarKind, string> = {
  rent: "landing.mock.on_rent",
  reserved: "landing.mock.reserved",
  transfer: "landing.mock.transfer",
  maintenance: "landing.mock.maintenance",
};

/**
 * A week of the fleet calendar: one row per vehicle, bars for bookings,
 * transfers and maintenance. Decorative; the caption describes it.
 */
export function FleetCalendarMock({
  compact = false,
  className,
}: {
  compact?: boolean;
  className?: string;
}) {
  const { t } = useLocale();
  const days = t("landing.mock.days").split(",");
  const rows = compact ? ROWS.slice(0, 3) : ROWS;

  return (
    <div
      aria-hidden
      className={cn(
        "bg-card text-card-foreground rounded-2xl border p-4 shadow-[0_1px_0_rgba(20,21,39,0.04),0_24px_48px_-24px_rgba(20,21,39,0.25)] sm:p-5",
        className,
      )}
    >
      <div className="mb-4 flex items-baseline justify-between gap-3">
        <p className="text-sm font-semibold">{t("landing.mock.week")}</p>
        <p className="text-muted-foreground text-xs">
          {t("landing.mock.branch_central")}
        </p>
      </div>

      <div className="grid grid-cols-[4.75rem_repeat(7,minmax(0,1fr))] gap-y-2 text-[11px] sm:grid-cols-[8.5rem_repeat(7,minmax(0,1fr))] sm:text-xs">
        <div />
        {days.map((day) => (
          <div
            key={day}
            className="text-muted-foreground border-l pl-1 font-medium sm:pl-1.5"
          >
            {day}
          </div>
        ))}

        {rows.map((row, rowIndex) => (
          <div key={row.plate} className="contents">
            <div className="flex min-w-0 flex-col justify-center pr-2">
              <span className="truncate font-semibold tabular-nums">
                {row.plate}
              </span>
              <span className="text-muted-foreground hidden truncate sm:block">
                {row.model}
              </span>
            </div>
            <div className="relative col-span-7 grid h-10 grid-cols-7 gap-1 sm:h-11">
              {row.bars.map((bar, barIndex) => (
                <motion.div
                  key={`${row.plate}-${bar.start}`}
                  initial={{ scaleX: 0, opacity: 0 }}
                  whileInView={{ scaleX: 1, opacity: 1 }}
                  viewport={{ once: true, margin: "0px 0px -10% 0px" }}
                  transition={{
                    duration: 0.6,
                    delay: 0.15 + rowIndex * 0.08 + barIndex * 0.06,
                    ease: [0.22, 1, 0.36, 1],
                  }}
                  className={cn(
                    "flex items-center truncate rounded-md px-1.5 font-medium sm:px-2",
                    BAR_CLASS[bar.kind],
                  )}
                  style={{
                    gridColumn: `${bar.start} / span ${bar.span}`,
                    originX: 0,
                  }}
                >
                  {bar.span > 1 ? t(LABEL_KEY[bar.kind]) : null}
                </motion.div>
              ))}
            </div>
          </div>
        ))}
      </div>

      {!compact && (
        <div className="text-muted-foreground mt-5 flex flex-wrap gap-x-4 gap-y-2 text-xs">
          {(Object.keys(BAR_CLASS) as BarKind[]).map((kind) => (
            <span key={kind} className="inline-flex items-center gap-1.5">
              <span className={cn("size-2.5 rounded-sm", BAR_CLASS[kind])} />
              {t(LABEL_KEY[kind])}
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
