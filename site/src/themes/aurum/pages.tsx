import { RotateCcw, Search, SlidersHorizontal } from "lucide-react";
import Link from "next/link";

import { EmptyState } from "@/components/site/page-heading";
import { Alert } from "@/components/ui/card";
import { loadSubgroups } from "@/lib/catalog";
import type { Ctx } from "@/lib/server";

import { segmentsOf } from "../facts";
import { AuHeading, ResultCard } from "./sections";

/**
 * /vehicles in Aurum: the studio's catalog layout. A route bar invites the
 * date search (prices need dates), the filter card lists the live segments
 * as links with their counts (no JavaScript), and the results are wide
 * rows. No price filter, sorting or fake result counts.
 */
export async function Vehicles({
  ctx,
  segment,
}: {
  ctx: Ctx;
  segment?: string;
}) {
  const { t, site } = ctx;
  const subgroups = site.booking_enabled ? await loadSubgroups() : null;
  const active =
    segment && subgroups?.some((s) => s.segment === segment)
      ? segment
      : undefined;
  const list = (subgroups ?? []).filter((s) => !active || s.segment === active);
  const segments = segmentsOf(subgroups ?? []);
  return (
    <div className="au-wrap au-catalog">
      <div className="au-route-bar">
        <div>
          <span className="au-kicker">{t("au.catalog.kicker")}</span>
          <strong>{t("au.catalog.route")}</strong>
        </div>
        {site.booking_enabled ? (
          <Link href="/search" className="rt-button rt-button--outline">
            <Search aria-hidden size={16} />
            {t("au.catalog.search")}
          </Link>
        ) : null}
      </div>
      <AuHeading
        id="catalog-title"
        level="h1"
        kicker={t("au.fleet.kicker")}
        title={t("au.catalog.title")}
        lead={t("au.catalog.lead")}
      />
      {!subgroups ? (
        <Alert>{t("search.booking_off")}</Alert>
      ) : !subgroups.length ? (
        <EmptyState title={t("fleet.empty")} />
      ) : (
        <div className="au-catalog-layout">
          <aside>
            <div className="au-card au-filter">
              <h2>
                <SlidersHorizontal aria-hidden size={18} />
                {t("au.catalog.filters")}
              </h2>
              {segments.length > 1 ? (
                <nav aria-label={t("au.catalog.class")}>
                  <p className="au-filter-legend">{t("au.catalog.class")}</p>
                  <ul>
                    {segments.map((seg) => (
                      <li key={seg}>
                        <Link
                          href={`/vehicles?segment=${encodeURIComponent(seg)}`}
                          aria-current={active === seg ? "page" : undefined}
                        >
                          <span className="au-check" aria-hidden />
                          {seg}
                          <small>
                            {subgroups.filter((s) => s.segment === seg).length}
                          </small>
                        </Link>
                      </li>
                    ))}
                  </ul>
                </nav>
              ) : null}
              {active ? (
                <Link href="/vehicles" className="au-text-link">
                  <RotateCcw aria-hidden size={15} />
                  {t("au.catalog.reset")}
                </Link>
              ) : null}
            </div>
            <p className="au-filter-note">{t("au.catalog.note")}</p>
          </aside>
          <div>
            <p className="au-results-toolbar" role="status">
              {t("fleet.count", { count: list.length })}
            </p>
            <ul className="au-results">
              {list.map((s) => (
                <li key={s.uuid}>
                  <ResultCard ctx={ctx} subgroup={s} />
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}
    </div>
  );
}
