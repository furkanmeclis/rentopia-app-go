import { ArrowRight, ArrowUpRight } from "lucide-react";
import Link from "next/link";

import { VehicleCard } from "@/components/site/vehicle-card";
import type { Ctx } from "@/lib/server";
import type { Subgroup } from "@/lib/types";

import { SectionHeading } from "../parts";

/** Segment chips (links: filtering works without JavaScript). */
export function SegmentTabs({
  ctx,
  subgroups,
  active,
}: {
  ctx: Ctx;
  subgroups: Subgroup[];
  active?: string;
}) {
  const segments = [
    ...new Set(subgroups.map((s) => s.segment).filter(Boolean)),
  ];
  if (segments.length < 2) return null;
  return (
    <nav className="tw-fleet-tabs" aria-label={ctx.t("fleet.segments")}>
      <Link href="/vehicles" aria-current={!active ? "page" : undefined}>
        {ctx.t("fleet.all")}
      </Link>
      {segments.map((s) => (
        <Link
          key={s}
          href={`/vehicles?segment=${encodeURIComponent(s)}`}
          aria-current={active === s ? "page" : undefined}
        >
          {s}
        </Link>
      ))}
    </nav>
  );
}

/** A fleet card's actions: prices need dates, so both lead to the search. */
export function FleetActions({
  ctx,
  onFleetPage = false,
}: {
  ctx: Ctx;
  onFleetPage?: boolean;
}) {
  return (
    <>
      {onFleetPage ? (
        <span />
      ) : (
        <Link href="/vehicles" className="tw-text-button">
          {ctx.t("fleet.details")}
          <ArrowUpRight aria-hidden size={15} />
        </Link>
      )}
      <Link
        href="/search"
        className="rt-button rt-button--primary rt-button--sm"
      >
        {ctx.t("fleet.rent")}
        <ArrowRight aria-hidden size={15} />
      </Link>
    </>
  );
}

/** Home fleet: the storefront's live vehicle groups (up to six). */
export function Fleet({
  ctx,
  subgroups,
}: {
  ctx: Ctx;
  subgroups: Subgroup[] | null;
}) {
  if (!subgroups?.length) return null;
  const { t } = ctx;
  return (
    <section className="tw-section" aria-labelledby="fleet-title">
      <div className="rt-container">
        <SectionHeading
          id="fleet-title"
          eyebrow={t("tw.fleet.eyebrow")}
          title={t("tw.fleet.title")}
          lead={t("tw.fleet.lead")}
          action={
            <Link href="/vehicles" className="tw-text-button">
              {t("fleet.all_vehicles")}
              <ArrowUpRight aria-hidden size={18} />
            </Link>
          }
        />
        <SegmentTabs ctx={ctx} subgroups={subgroups} />
        <ul className="tw-vehicle-grid">
          {subgroups.slice(0, 6).map((s) => (
            <li key={s.uuid}>
              <VehicleCard ctx={ctx} subgroup={s}>
                <FleetActions ctx={ctx} />
              </VehicleCard>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}
