import {
  ArrowRight,
  ArrowUpRight,
  Building2,
  CarFront,
  Headphones,
  MapPin,
  Plane,
  ShieldCheck,
} from "lucide-react";
import Link from "next/link";

import { SearchForm } from "@/components/site/search-form";
import { Paragraphs } from "@/components/site/text";
import { Alert } from "@/components/ui/card";
import { campaignCards, campaignDate } from "@/lib/campaigns";
import { cn } from "@/lib/cn";
import { localized } from "@/lib/links";
import { siteMediaUrl } from "@/lib/media";
import { mediaUrl, type Ctx } from "@/lib/server";
import type { Location, Subgroup } from "@/lib/types";

import { segmentsOf, vehicleFacts } from "../facts";
import { guideCards } from "../guide";
import { hasContactPage } from "../horizon/chrome";
import { fleetColumns } from "../registry";
import { aboutHighlights } from "../technowide/sections/about";
import { isAirport } from "../technowide/sections/branches";
import { CampaignCode, validityText } from "../technowide/sections/campaigns";
import { heroSlides } from "../technowide/sections/hero";
import { ADVANTAGE_ICONS } from "../technowide/parts";
import { homeFleetLimit } from "./helpers";

/** Section heading: a soft badge, the title and an optional lead. */
function Heading({
  id,
  badge,
  title,
  lead,
  left = false,
  action,
}: {
  id: string;
  badge?: string;
  title: string;
  lead?: string;
  left?: boolean;
  action?: React.ReactNode;
}) {
  return (
    <div className={cn("cl-heading", left && "cl-heading--left")}>
      <div>
        {badge ? <span className="cl-badge">{badge}</span> : null}
        <h2 id={id}>{title}</h2>
        {lead ? <p>{lead}</p> : null}
      </div>
      {action}
    </div>
  );
}

/** "Rezervasyon Motoru": the heading strip, the search form, trust notes. */
function BookingCard({
  ctx,
  locations,
}: {
  ctx: Ctx;
  locations: Location[] | null;
}) {
  const { t, site } = ctx;
  return (
    <div className="cl-booking">
      <div className="cl-booking-head">
        <span>
          <CarFront aria-hidden size={18} />
          {t("cl.search.title")}
        </span>
        <span>
          <ShieldCheck aria-hidden size={13} />
          {t("cl.search.hint")}
        </span>
      </div>
      {!site.booking_enabled || locations === null ? (
        <Alert>{t("search.booking_off")}</Alert>
      ) : locations.length === 0 ? (
        <Alert>{t("search.no_locations")}</Alert>
      ) : (
        <SearchForm ctx={ctx} locations={locations} id="search" />
      )}
      <ul className="cl-trust" aria-label={t("cl.trust.label")}>
        <li>
          <ShieldCheck aria-hidden size={14} />
          {t("cl.trust.conditions")}
        </li>
        <li>
          <Headphones aria-hidden size={14} />
          {t("cl.trust.lookup")}
        </li>
        <li>
          <CarFront aria-hidden size={14} />
          {t("cl.trust.specs")}
        </li>
      </ul>
    </div>
  );
}

/**
 * The hero band: badge (tagline, else the name), the first slide's title
 * and text and, when `search` follows the hero, the booking card. Like the
 * studio's classic it shows one message and no photo (a token gradient).
 */
export function Hero({
  ctx,
  locations,
  withSearch,
}: {
  ctx: Ctx;
  locations: Location[] | null;
  withSearch: boolean;
}) {
  const { t, site } = ctx;
  const slide = heroSlides(ctx)[0];
  const name = site.organization.name;
  const badge = localized(site.config.brand?.tagline, ctx.locale) || name;
  return (
    <section className="cl-hero" aria-label={t("tw.hero.label")}>
      <div className="cl-container">
        <span className="cl-badge">
          <MapPin aria-hidden size={12} />
          {badge}
        </span>
        <h1>{slide?.title || name}</h1>
        {slide?.subtitle ? (
          <p className="cl-hero-lead">{slide.subtitle}</p>
        ) : null}
        {slide?.cta ? (
          <a
            href={slide.cta.href}
            className="rt-button rt-button--outline cl-hero-cta"
            rel={slide.cta.external ? "noopener noreferrer" : undefined}
          >
            {slide.cta.label}
            <ArrowRight aria-hidden size={16} />
          </a>
        ) : null}
        {withSearch ? <BookingCard ctx={ctx} locations={locations} /> : null}
      </div>
    </section>
  );
}

/** `search` away from the hero: the booking card on its own. */
export function SearchSection({
  ctx,
  locations,
}: {
  ctx: Ctx;
  locations: Location[] | null;
}) {
  return (
    <section
      className="cl-section cl-search-section"
      aria-label={ctx.t("search.title")}
    >
      <BookingCard ctx={ctx} locations={locations} />
    </section>
  );
}

/**
 * A classic vehicle card: tinted photo well with the segment badge, the
 * model "or similar", the published conditions as four chips, a note and
 * the date-based quote line with the accent action (prices need dates).
 */
export function VehicleCard({
  ctx,
  subgroup: s,
  heading = "h3",
}: {
  ctx: Ctx;
  subgroup: Subgroup;
  heading?: "h2" | "h3";
}) {
  const { t } = ctx;
  const H = heading;
  const model = s.representative_model || s.title;
  const image = mediaUrl(s.images?.[0]);
  const facts = vehicleFacts(ctx, s);
  const group = s.title && s.title !== model ? s.title : "";
  return (
    <article className="cl-card cl-vehicle">
      <div className="cl-vehicle-image">
        {s.segment ? <span className="cl-chip">{s.segment}</span> : null}
        {image ? (
          // Product images come from the API through /media (same origin).
          // eslint-disable-next-line @next/next/no-img-element
          <img src={image} alt={model} loading="lazy" />
        ) : (
          <CarFront aria-hidden size={64} strokeWidth={1.1} />
        )}
      </div>
      <div className="cl-vehicle-body">
        <H>{model}</H>
        <p className="cl-vehicle-similar">
          {t("vehicle.similar")}
          {group ? ` · ${group}` : ` · ${t("cl.fleet.group")}`}
        </p>
        {facts.length ? (
          <dl className="cl-specs">
            {facts.map(({ icon: Icon, label, value }) => (
              <div key={label} title={label}>
                <dt>
                  <Icon aria-hidden size={14} />
                  <span className="rt-sr-only">{label}</span>
                </dt>
                <dd>{value}</dd>
              </div>
            ))}
          </dl>
        ) : null}
        <p className="cl-vehicle-note">
          <ShieldCheck aria-hidden size={14} />
          {t("cl.fleet.note")}
        </p>
        <div className="cl-vehicle-price">
          <span>{t("cl.fleet.quote")}</span>
          <small>{t("cl.fleet.quote_hint")}</small>
        </div>
        <Link href="/search" className="rt-button cl-cta cl-vehicle-cta">
          {t("cl.fleet.choose")}
          <span className="rt-sr-only">: {model}</span>
          <ArrowRight aria-hidden size={16} />
        </Link>
      </div>
    </article>
  );
}

/** Segment chips (links to /vehicles: filtering works without JavaScript). */
export function SegmentChips({
  ctx,
  subgroups,
  active,
}: {
  ctx: Ctx;
  subgroups: Subgroup[];
  active?: string;
}) {
  const segments = segmentsOf(subgroups);
  if (segments.length < 2) return null;
  return (
    <nav className="cl-chips" aria-label={ctx.t("fleet.segments")}>
      <Link href="/vehicles" aria-current={!active ? "page" : undefined}>
        {ctx.t("cl.fleet.all_count", { count: subgroups.length })}
      </Link>
      {segments.map((seg) => (
        <Link
          key={seg}
          href={`/vehicles?segment=${encodeURIComponent(seg)}`}
          aria-current={active === seg ? "page" : undefined}
        >
          {seg}
        </Link>
      ))}
    </nav>
  );
}

/** The vehicle grid in the organization's `fleetColumns` (2 or 3). */
export function VehicleGrid({
  ctx,
  subgroups,
  heading,
}: {
  ctx: Ctx;
  subgroups: Subgroup[];
  heading?: "h2" | "h3";
}) {
  const columns = fleetColumns(ctx.site.settings);
  return (
    <ul className={`cl-vehicle-grid cl-vehicle-grid--${columns}`}>
      {subgroups.map((s) => (
        <li key={s.uuid}>
          <VehicleCard ctx={ctx} subgroup={s} heading={heading} />
        </li>
      ))}
    </ul>
  );
}

/** Home fleet: chips, then two rows of live vehicle groups. */
export function Fleet({
  ctx,
  subgroups,
}: {
  ctx: Ctx;
  subgroups: Subgroup[] | null;
}) {
  if (!subgroups?.length) return null;
  const { t } = ctx;
  const limit = homeFleetLimit(fleetColumns(ctx.site.settings));
  return (
    <section className="cl-section cl-fleet" aria-labelledby="fleet-title">
      <h2 id="fleet-title" className="rt-sr-only">
        {t("fleet.title")}
      </h2>
      <div className="cl-fleet-toolbar">
        <SegmentChips ctx={ctx} subgroups={subgroups} />
        <Link href="/vehicles" className="cl-text-link">
          {t("cl.fleet.more")}
          <ArrowRight aria-hidden size={15} />
        </Link>
      </div>
      <VehicleGrid ctx={ctx} subgroups={subgroups.slice(0, limit)} />
    </section>
  );
}

/** The organization's advantages as four-up cards with an icon tile. */
export function Advantages({ ctx }: { ctx: Ctx }) {
  const items = (ctx.content.advantages ?? []).filter(
    (a) => a && (a.title || a.text),
  );
  if (!items.length) return null;
  const { t } = ctx;
  return (
    <section
      className="cl-section cl-advantages"
      aria-labelledby="advantages-title"
    >
      <Heading
        id="advantages-title"
        title={t("cl.why.title")}
        lead={t("cl.why.lead")}
      />
      <ul className="cl-benefit-grid">
        {items.map((a, i) => {
          const Icon = ADVANTAGE_ICONS[a.icon] ?? ADVANTAGE_ICONS.check;
          return (
            <li key={i} className="cl-card">
              <span className="cl-icon" aria-hidden>
                <Icon size={20} />
              </span>
              {a.title ? <h3>{a.title}</h3> : null}
              {a.text ? <p>{a.text}</p> : null}
            </li>
          );
        })}
      </ul>
    </section>
  );
}

/** Live offices as cards (airport / city icon, city, details link). */
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
    <section className="cl-section" aria-labelledby="branches-title">
      <Heading
        id="branches-title"
        badge={t("cl.branches.badge")}
        title={t("cl.branches.title")}
        left
        action={
          <Link href="/branches" className="cl-text-link">
            {t("branches.all")}
            <ArrowRight aria-hidden size={15} />
          </Link>
        }
      />
      <ul className="cl-branch-grid">
        {locations.slice(0, 6).map((l) => {
          const Icon = isAirport(l) ? Plane : Building2;
          return (
            <li key={l.uuid} className="cl-card">
              <span className="cl-icon" aria-hidden>
                <Icon size={20} />
              </span>
              <h3>{l.name}</h3>
              {l.city || l.district ? (
                <p>
                  <MapPin aria-hidden size={13} />
                  {[l.district, l.city].filter(Boolean).join(", ")}
                </p>
              ) : null}
              <Link
                href={`/branches/${l.uuid}`}
                className="rt-button rt-button--outline rt-button--sm"
              >
                {t("cl.branches.details")}
                <span className="rt-sr-only">: {l.name}</span>
                <ArrowUpRight aria-hidden size={14} />
              </Link>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

/**
 * Split about: badge, title, the story (two paragraphs on the home page,
 * all of it on /about), highlights and the image card (library image or a
 * token gradient) with the name and tagline.
 */
export function About({ ctx, full = false }: { ctx: Ctx; full?: boolean }) {
  const { t, content, site } = ctx;
  if (!content.about) return null;
  const highlights = aboutHighlights(ctx);
  const src = siteMediaUrl(site, site.config.about?.image);
  const tagline = localized(site.config.brand?.tagline, ctx.locale);
  return (
    <section className="cl-section cl-about" aria-labelledby="about-title">
      <div>
        <span className="cl-badge">{t("cl.about.badge")}</span>
        <h2 id="about-title">
          {t("cl.about.title", { name: site.organization.name })}
        </h2>
        <Paragraphs
          text={content.about}
          limit={full ? undefined : 2}
          className="cl-about-text"
        />
        {highlights.length ? (
          <ul className="cl-highlights" aria-label={t("cl.about.highlights")}>
            {highlights.map((h, i) => (
              <li key={i}>
                <ShieldCheck aria-hidden size={18} />
                {h}
              </li>
            ))}
          </ul>
        ) : null}
        {full ? null : (
          <Link href="/about" className="rt-button rt-button--outline">
            {t("cl.about.cta")}
            <ArrowRight aria-hidden size={16} />
          </Link>
        )}
      </div>
      <figure className="cl-card cl-about-image">
        {src ? (
          // Library media comes from the API through /media (same origin).
          // eslint-disable-next-line @next/next/no-img-element
          <img src={src} alt="" loading="lazy" />
        ) : (
          <span className="cl-plain" aria-hidden />
        )}
        <figcaption>
          <strong>{site.organization.name}</strong>
          {tagline ? <span>{tagline}</span> : null}
        </figcaption>
      </figure>
    </section>
  );
}

/** Questions as bordered native <details> rows (no JavaScript). */
export function Faq({
  ctx,
  items,
  more = false,
}: {
  ctx: Ctx;
  items: { question: string; answer: string }[];
  more?: boolean;
}) {
  const { t } = ctx;
  if (!items.length) return null;
  return (
    <section className="cl-section cl-faq" aria-labelledby="faq-title">
      <Heading
        id="faq-title"
        badge={t("cl.faq.badge")}
        title={t("cl.faq.title")}
        lead={t("cl.faq.lead")}
      />
      <div className="cl-accordion">
        {items.map((f, i) => (
          <details key={i} className="cl-accordion-item">
            <summary>
              <span>{f.question}</span>
            </summary>
            <Paragraphs text={f.answer} className="cl-accordion-answer" />
          </details>
        ))}
      </div>
      {more || hasContactPage(ctx) ? (
        <div className="cl-faq-links">
          {more ? (
            <Link href="/faq" className="cl-text-link">
              {t("tw.faq.all")}
              <ArrowRight aria-hidden size={15} />
            </Link>
          ) : null}
          {hasContactPage(ctx) ? (
            <Link href="/contact" className="cl-text-link">
              {t("nav.contact")}
              <ArrowRight aria-hidden size={15} />
            </Link>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

/** Story cards: live campaigns (first six) with validity and code. */
export function Campaigns({ ctx }: { ctx: Ctx }) {
  const { t } = ctx;
  const cards = campaignCards(ctx.site, ctx.locale).slice(0, 6);
  if (!cards.length) return null;
  return (
    <section className="cl-section" aria-labelledby="campaigns-title">
      <Heading
        id="campaigns-title"
        badge={t("cl.campaigns.badge")}
        title={t("cl.campaigns.title")}
        left
        action={
          <Link href="/campaigns" className="cl-text-link">
            {t("tw.campaigns.all")}
            <ArrowRight aria-hidden size={15} />
          </Link>
        }
      />
      <ul className="cl-story-grid">
        {cards.map((card) => {
          const title = card.title || t("tw.campaigns.default_title");
          return (
            <li key={card.uuid} className="cl-card cl-story">
              <div className={cn("cl-story-image", !card.image && "cl-plain")}>
                {card.image ? (
                  // Library media comes from the API through /media.
                  // eslint-disable-next-line @next/next/no-img-element
                  <img src={card.image} alt="" loading="lazy" />
                ) : null}
              </div>
              <div className="cl-story-body">
                <span className="cl-badge">{validityText(ctx, card)}</span>
                <h3>{title}</h3>
                {card.summary ? <p>{card.summary}</p> : null}
                {card.code ? <CampaignCode ctx={ctx} code={card.code} /> : null}
                <Link
                  href={`/campaigns/${card.uuid}`}
                  className="rt-button rt-button--outline rt-button--sm"
                >
                  {card.cta || t("tw.campaigns.cta")}
                  <span className="rt-sr-only">: {title}</span>
                  <ArrowRight aria-hidden size={15} />
                </Link>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

/** Guide cards from resolve `articles` (see ../guide.ts). */
export function Guide({ ctx }: { ctx: Ctx }) {
  const { t } = ctx;
  const cards = guideCards(ctx.site, ctx.locale);
  if (!cards.length) return null;
  return (
    <section className="cl-section" aria-labelledby="guide-title">
      <Heading id="guide-title" title={t("cl.guide.title")} left />
      <ul className="cl-story-grid">
        {cards.map((a) => (
          <li key={a.href} className="cl-card cl-story">
            <div className={cn("cl-story-image", !a.image && "cl-plain")}>
              {a.image ? (
                // Library media comes from the API through /media.
                // eslint-disable-next-line @next/next/no-img-element
                <img src={a.image} alt="" loading="lazy" />
              ) : null}
            </div>
            <div className="cl-story-body">
              {a.date ? (
                <span className="cl-badge">
                  {campaignDate(a.date, ctx.locale)}
                </span>
              ) : null}
              <h3>{a.title}</h3>
              {a.summary ? <p>{a.summary}</p> : null}
              <Link
                href={a.href}
                className="rt-button rt-button--outline rt-button--sm"
              >
                {t("cl.guide.read")}
                <span className="rt-sr-only">: {a.title}</span>
                <ArrowRight aria-hidden size={15} />
              </Link>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
