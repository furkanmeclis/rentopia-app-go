"use client";

import { CarFront, MapPin } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatTry } from "../lib/format";

const LINES = [
  { key: "landing.booking.mock_line_rent", amount: 6400 },
  { key: "landing.booking.mock_line_oneway", amount: 750 },
  { key: "landing.booking.mock_line_extra", amount: 400 },
  { key: "landing.booking.mock_line_campaign", amount: -640 },
] as const;

/** A booking awaiting approval, with an itemised price. Decorative. */
export function ReservationMock({ className }: { className?: string }) {
  const { t, locale } = useLocale();
  const total = LINES.reduce((sum, line) => sum + line.amount, 0);

  return (
    <div
      aria-hidden
      className={cn(
        "bg-card text-card-foreground w-full max-w-md rounded-2xl border p-5 shadow-[0_1px_0_rgba(20,21,39,0.04),0_24px_48px_-24px_rgba(20,21,39,0.25)] sm:p-6",
        className,
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <span className="text-muted-foreground text-sm font-medium tabular-nums">
          {t("landing.booking.mock_ref")}
        </span>
        <Badge variant="success">{t("landing.booking.mock_status")}</Badge>
      </div>

      <div className="mt-4 flex items-start gap-3">
        <span className="bg-muted text-foreground grid size-10 shrink-0 place-items-center rounded-xl">
          <CarFront className="size-5" />
        </span>
        <div className="min-w-0">
          <p className="font-semibold">{t("landing.booking.mock_vehicle")}</p>
          <p className="text-muted-foreground text-sm">
            {t("landing.booking.mock_group")}
          </p>
        </div>
      </div>

      <dl className="mt-5 grid gap-3 text-sm">
        {(
          [
            [
              "landing.booking.mock_pickup",
              "landing.booking.mock_pickup_value",
            ],
            [
              "landing.booking.mock_return",
              "landing.booking.mock_return_value",
            ],
          ] as const
        ).map(([label, value]) => (
          <div key={label} className="flex items-start gap-2.5">
            <MapPin className="text-muted-foreground mt-0.5 size-4 shrink-0" />
            <div>
              <dt className="text-muted-foreground text-xs">{t(label)}</dt>
              <dd className="font-medium">{t(value)}</dd>
            </div>
          </div>
        ))}
      </dl>

      <Separator className="my-5" />

      <ul className="grid gap-2 text-sm">
        {LINES.map((line) => (
          <li key={line.key} className="flex justify-between gap-4">
            <span className="text-muted-foreground">{t(line.key)}</span>
            <span
              className={cn(
                "tabular-nums",
                line.amount < 0 && "text-emerald-700 dark:text-emerald-300",
              )}
            >
              {formatTry(locale, line.amount)}
            </span>
          </li>
        ))}
      </ul>

      <div className="mt-4 flex items-end justify-between gap-4 border-t pt-4">
        <div>
          <p className="text-muted-foreground text-xs">
            {t("landing.booking.mock_total")}
          </p>
          <p className="text-2xl font-semibold tracking-tight tabular-nums">
            {formatTry(locale, total)}
          </p>
          <p className="text-muted-foreground text-xs tabular-nums">
            {t("landing.booking.mock_deposit")} {formatTry(locale, 5000)}
          </p>
        </div>
        <span className="bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm font-medium">
          {t("landing.booking.mock_approve")}
        </span>
      </div>
    </div>
  );
}
