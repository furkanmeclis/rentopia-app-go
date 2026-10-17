import { ArrowUpRight, Building2, MapPin, Plane } from "lucide-react";
import Link from "next/link";

import type { Ctx } from "@/lib/server";
import type { Location } from "@/lib/types";

import { SectionHeading } from "../parts";

/** Decorative only: an airport icon for offices named like one. */
export const isAirport = (l: Location) =>
  /havaliman|airport/i.test(`${l.name} ${l.code}`);

/** An office card (live location from the public API). */
export function OfficeCard({
  ctx,
  location: l,
}: {
  ctx: Ctx;
  location: Location;
}) {
  const airport = isAirport(l);
  const Icon = airport ? Plane : Building2;
  return (
    <article className="tw-office-card">
      <div className="tw-office-art" aria-hidden>
        <Icon size={35} strokeWidth={1.3} />
        {l.city ? <span>{l.city}</span> : null}
      </div>
      <div className="tw-office-body">
        <p className="tw-mini-label">
          {l.district || ctx.t("branches.office")}
        </p>
        <h3>{l.name}</h3>
        {l.address || l.city ? (
          <p className="tw-office-address">
            <MapPin aria-hidden size={16} />
            <span>
              {[l.address, l.district, l.city].filter(Boolean).join(", ")}
            </span>
          </p>
        ) : null}
        <Link href={`/branches/${l.uuid}`} className="tw-text-button">
          {ctx.t("branches.view")}
          <ArrowUpRight aria-hidden size={16} />
          <span className="rt-sr-only">: {l.name}</span>
        </Link>
      </div>
    </article>
  );
}

/** Home branches (when a template lists the section): up to six offices. */
export function Branches({
  ctx,
  locations,
}: {
  ctx: Ctx;
  locations: Location[] | null;
}) {
  if (!locations?.length) return null;
  const { t } = ctx;
  return (
    <section className="tw-section" aria-labelledby="branches-title">
      <div className="rt-container">
        <SectionHeading
          id="branches-title"
          eyebrow={t("tw.branches.eyebrow")}
          title={t("tw.branches.title")}
          lead={t("tw.branches.lead")}
          action={
            <Link href="/branches" className="tw-text-button">
              {t("branches.all")}
              <ArrowUpRight aria-hidden size={18} />
            </Link>
          }
        />
        <ul className="tw-office-grid">
          {locations.slice(0, 6).map((l) => (
            <li key={l.uuid}>
              <OfficeCard ctx={ctx} location={l} />
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}
