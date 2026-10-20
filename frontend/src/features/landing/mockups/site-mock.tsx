"use client";

import { CalendarDays, Lock, MapPin } from "lucide-react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

// Sample address of a company's booking site, per panel language.
const SAMPLE_DOMAIN = {
  tr: "rezervasyon.firmaniz.com",
  en: "booking.yourcompany.com",
} as const;

// One accent per site template, to show the same site in another look.
const THEME_SWATCHES = ["#2E358C", "#0F766E", "#B45309", "#141527"];

/** Browser chrome around a page, with an address bar. */
export function BrowserFrame({
  address,
  children,
  className,
}: {
  address: string;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "bg-card overflow-hidden rounded-2xl border shadow-[0_1px_0_rgba(20,21,39,0.04),0_32px_64px_-32px_rgba(20,21,39,0.4)]",
        className,
      )}
    >
      <div className="bg-muted/60 flex items-center gap-3 border-b px-4 py-2.5">
        <div className="flex gap-1.5" aria-hidden>
          <span className="size-2.5 rounded-full bg-[#141527]/15 dark:bg-white/20" />
          <span className="size-2.5 rounded-full bg-[#141527]/15 dark:bg-white/20" />
          <span className="size-2.5 rounded-full bg-[#141527]/15 dark:bg-white/20" />
        </div>
        <div className="bg-background text-muted-foreground mx-auto flex max-w-xs min-w-0 flex-1 items-center justify-center gap-1.5 rounded-md px-3 py-1 text-xs">
          <Lock className="size-3 shrink-0" aria-hidden />
          <span className="truncate">{address}</span>
        </div>
        <span className="w-[42px]" aria-hidden />
      </div>
      {children}
    </div>
  );
}

/** A company's booking site and the widget snippet. Decorative. */
export function SiteMock({ className }: { className?: string }) {
  const { t, locale } = useLocale();
  const domain = SAMPLE_DOMAIN[locale];
  const snippet = `<div id="rentopia-booking"></div>\n<script src="https://${domain}/widget.js" async\n  data-target="#rentopia-booking" data-locale="tr"></script>`;

  return (
    <div aria-hidden className={cn("relative", className)}>
      <BrowserFrame address={domain}>
        <div className="grid gap-6 p-5 pb-12 sm:p-8 sm:pb-16">
          <div className="flex items-center justify-between gap-3">
            <span className="inline-flex items-center gap-2 font-semibold">
              <span className="size-6 rounded-full bg-[var(--site-accent,#2E358C)]" />
              {t("landing.site.mock_brand")}
            </span>
            <span className="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs">
              <span className="size-1.5 rounded-full bg-emerald-500" />
              {t("landing.site.mock_published")}
            </span>
          </div>

          <p className="max-w-sm text-2xl leading-tight font-semibold tracking-tight text-balance sm:text-3xl">
            {t("landing.site.mock_heading")}
          </p>

          <div className="grid gap-2 rounded-xl border p-2 sm:grid-cols-[1fr_1fr_auto]">
            <div className="flex items-center gap-2 rounded-lg px-3 py-2">
              <MapPin className="text-muted-foreground size-4" />
              <div className="text-xs">
                <p className="text-muted-foreground">
                  {t("landing.site.mock_pickup")}
                </p>
                <p className="font-medium">
                  {t("landing.mock.branch_airport")}
                </p>
              </div>
            </div>
            <div className="flex items-center gap-2 rounded-lg px-3 py-2">
              <CalendarDays className="text-muted-foreground size-4" />
              <div className="text-xs">
                <p className="text-muted-foreground">
                  {t("landing.site.mock_dates")}
                </p>
                <p className="font-medium">
                  {t("landing.site.mock_dates_value")}
                </p>
              </div>
            </div>
            <span className="grid place-items-center rounded-lg bg-[var(--site-accent,#2E358C)] px-4 py-2.5 text-sm font-medium text-white">
              {t("landing.site.mock_search")}
            </span>
          </div>

          <div className="flex items-center gap-3 text-xs">
            <span className="text-muted-foreground">
              {t("landing.site.mock_theme")}
            </span>
            <div className="flex gap-1.5">
              {THEME_SWATCHES.map((color, i) => (
                <span
                  key={color}
                  className={cn(
                    "ring-border size-5 rounded-full ring-1",
                    i === 0 &&
                      "ring-foreground ring-offset-card ring-2 ring-offset-2",
                  )}
                  style={{ backgroundColor: color }}
                />
              ))}
            </div>
          </div>
        </div>
      </BrowserFrame>

      <div className="relative z-10 mx-4 -mt-6 rounded-xl bg-[#141527] p-4 text-white shadow-[0_24px_48px_-24px_rgba(20,21,39,0.6)] sm:mx-10 sm:-mt-10 dark:bg-[#0b0c1a] dark:ring-1 dark:ring-white/10">
        <p className="mb-2 text-xs text-white/60">
          {t("landing.site.mock_snippet")}
        </p>
        <pre className="overflow-x-auto font-mono text-[11px] leading-relaxed text-[#C7CAF6] sm:text-xs">
          <code>{snippet}</code>
        </pre>
      </div>
    </div>
  );
}
