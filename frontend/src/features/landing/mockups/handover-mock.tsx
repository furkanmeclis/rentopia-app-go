"use client";

import { CloudOff, FileCheck2 } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const FUEL_STEPS = 8;
const FUEL_LEVEL = 6;

/** Top view of a car with two marked damage points. */
function CarDiagram() {
  return (
    <svg viewBox="0 0 120 200" className="h-40 w-auto" role="presentation">
      <rect
        x="18"
        y="8"
        width="84"
        height="184"
        rx="34"
        className="fill-muted stroke-border"
        strokeWidth="2"
      />
      <rect
        x="30"
        y="48"
        width="60"
        height="34"
        rx="10"
        className="fill-background stroke-border"
        strokeWidth="2"
      />
      <rect
        x="30"
        y="128"
        width="60"
        height="26"
        rx="9"
        className="fill-background stroke-border"
        strokeWidth="2"
      />
      <line
        x1="60"
        y1="86"
        x2="60"
        y2="124"
        className="stroke-border"
        strokeWidth="2"
      />
      <circle cx="96" cy="66" r="7" className="fill-destructive/20" />
      <circle cx="96" cy="66" r="3.5" className="fill-destructive" />
      <circle cx="34" cy="168" r="7" className="fill-destructive/20" />
      <circle cx="34" cy="168" r="3.5" className="fill-destructive" />
    </svg>
  );
}

/** Handover form on a phone, mid-way through a drop in connection. */
export function HandoverMock({ className }: { className?: string }) {
  const { t } = useLocale();

  return (
    <div
      aria-hidden
      className={cn(
        "bg-card text-card-foreground mx-auto w-[18rem] rounded-[2.5rem] border-[6px] border-[#141527] p-4 shadow-[0_32px_64px_-32px_rgba(20,21,39,0.45)] sm:w-[19.5rem] dark:border-[#2a2c48]",
        className,
      )}
    >
      <div className="mx-auto mb-4 h-1.5 w-16 rounded-full bg-[#141527]/15 dark:bg-white/15" />

      <div className="flex items-center gap-2 rounded-lg bg-amber-100 px-3 py-2 text-xs font-medium text-amber-900 dark:bg-amber-400/15 dark:text-amber-200">
        <CloudOff className="size-3.5" />
        {t("landing.handover.mock_offline")}
      </div>

      <div className="mt-4 flex items-baseline justify-between">
        <p className="font-semibold">{t("landing.handover.mock_title")}</p>
        <p className="text-muted-foreground text-xs tabular-nums">34 RTP 482</p>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-2 text-xs">
        <div className="rounded-lg border px-3 py-2">
          <p className="text-muted-foreground">
            {t("landing.handover.mock_km")}
          </p>
          <p className="text-base font-semibold tabular-nums">48.210</p>
        </div>
        <div className="rounded-lg border px-3 py-2">
          <p className="text-muted-foreground">
            {t("landing.handover.mock_fuel")}
          </p>
          <div className="mt-1.5 flex gap-0.5">
            {Array.from({ length: FUEL_STEPS }, (_, i) => (
              <span
                key={i}
                className={cn(
                  "h-3 flex-1 rounded-[2px]",
                  i < FUEL_LEVEL ? "bg-primary" : "bg-muted",
                )}
              />
            ))}
          </div>
        </div>
      </div>

      <div className="mt-3 flex items-center gap-4 rounded-lg border px-3 py-3">
        <CarDiagram />
        <div className="text-xs">
          <p className="text-muted-foreground">
            {t("landing.handover.mock_damage")}
          </p>
          <p className="mt-0.5 font-semibold">
            {t("landing.handover.mock_damage_value")}
          </p>
          <div className="mt-3 grid grid-cols-3 gap-1">
            {Array.from({ length: 6 }, (_, i) => (
              <span
                key={i}
                className="bg-muted aspect-square rounded-[4px] bg-[linear-gradient(160deg,transparent_55%,rgba(46,53,140,0.18))]"
              />
            ))}
          </div>
        </div>
      </div>

      <div className="mt-3 flex items-center justify-between gap-2 rounded-lg border px-3 py-2.5 text-xs">
        <span className="inline-flex items-center gap-1.5 font-medium">
          <FileCheck2 className="text-muted-foreground size-4" />
          {t("landing.handover.mock_contract")}
        </span>
        <Badge variant="success">
          {t("landing.handover.mock_contract_state")}
        </Badge>
      </div>

      <div className="bg-primary text-primary-foreground mt-4 rounded-lg py-2.5 text-center text-sm font-medium">
        {t("landing.handover.mock_submit")}
      </div>
    </div>
  );
}
