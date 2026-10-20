"use client";

import type { ReactNode } from "react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { FinanceMock } from "../mockups/finance-mock";
import { FleetCalendarMock } from "../mockups/fleet-calendar-mock";
import { HoverLift, Reveal } from "./motion-primitives";

type Row = {
  id: string;
  title: string;
  body: string;
  points: readonly string[];
  caption: string;
  mock: ReactNode;
};

const ROWS: Row[] = [
  {
    id: "fleet",
    title: "landing.fleet.title",
    body: "landing.fleet.body",
    points: [
      "landing.fleet.p1",
      "landing.fleet.p2",
      "landing.fleet.p3",
      "landing.fleet.p4",
    ],
    caption: "landing.fleet.mock_caption",
    mock: <FleetCalendarMock className="w-full" />,
  },
  {
    id: "finance",
    title: "landing.finance.title",
    body: "landing.finance.body",
    points: [
      "landing.finance.p1",
      "landing.finance.p2",
      "landing.finance.p3",
      "landing.finance.p4",
    ],
    caption: "landing.finance.mock_caption",
    mock: <FinanceMock className="max-w-xl" />,
  },
];

/** Text beside a working-UI mockup, alternating sides from lg; both reveal on scroll. */
export function FeatureRows() {
  const { t } = useLocale();

  return (
    <div className="mx-auto grid max-w-[78rem] grid-cols-1 gap-24 px-4 pb-24 sm:px-6 lg:gap-36 lg:px-8 lg:pb-36">
      {ROWS.map((row, index) => {
        const flipped = index % 2 === 1;
        const titleId = `landing-${row.id}-title`;
        return (
          <section
            key={row.id}
            aria-labelledby={titleId}
            className="grid grid-cols-1 items-center gap-10 lg:grid-cols-12 lg:gap-8"
          >
            <Reveal
              className={cn(
                "lg:col-span-5",
                flipped ? "lg:order-2 lg:col-start-8" : "lg:order-1",
              )}
            >
              <h2
                id={titleId}
                className="text-3xl leading-[1.1] font-bold tracking-[-0.03em] text-balance sm:text-4xl"
              >
                {t(row.title)}
              </h2>
              <p className="text-muted-foreground mt-5 max-w-[56ch] text-lg leading-relaxed text-pretty">
                {t(row.body)}
              </p>
              <ul className="mt-8 grid text-[0.9375rem]">
                {row.points.map((point) => (
                  <li key={point} className="border-t py-3 last:border-b">
                    {t(point)}
                  </li>
                ))}
              </ul>
            </Reveal>

            <Reveal
              delay={0.1}
              y={48}
              className={cn(
                "lg:col-span-7",
                flipped ? "lg:order-1 lg:col-start-1" : "lg:order-2",
              )}
            >
              <HoverLift>
                <figure className="grid place-items-center rounded-[2rem] border bg-[#F3F4FA] px-4 py-10 sm:px-10 sm:py-14 dark:bg-white/[0.03]">
                  {row.mock}
                  <figcaption className="sr-only">{t(row.caption)}</figcaption>
                </figure>
              </HoverLift>
            </Reveal>
          </section>
        );
      })}
    </div>
  );
}
