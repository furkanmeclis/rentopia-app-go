import {
  ArrowDown,
  ArrowRight,
  ArrowUpRight,
  Asterisk,
  BadgeCheck,
  Car,
  Gauge,
  MoveUpRight,
  Plus,
  Users,
  Wallet,
} from "lucide-react";
import Link from "next/link";

import { SearchForm } from "@/components/site/search-form";
import { Paragraphs } from "@/components/site/text";
import { Alert } from "@/components/ui/card";
import { campaignCards } from "@/lib/campaigns";
import { cn } from "@/lib/cn";
import { money } from "@/lib/format";
import { localized } from "@/lib/links";
import { siteMediaUrl } from "@/lib/media";
import { mediaUrl, type Ctx } from "@/lib/server";
import type { Location, Subgroup } from "@/lib/types";

import { heroAlign } from "../registry";
import { aboutHighlights } from "../technowide/sections/about";
import { CampaignCode, validityText } from "../technowide/sections/campaigns";
import { heroSlides } from "../technowide/sections/hero";
import { hasContactPage } from "./chrome";

/** Two-digit index ("01"). */
export const pad = (n: number) => String(n).padStart(2, "0");

/** Section kicker with its number, the serif title and an optional link. */
function SectionHeading({
  id,
  index,
  eyebrow,
  title,
  link,
}: {
  id: string;
  index: string;
  eyebrow: string;
  title: string;
  link?: { href: string; label: string };
}) {
  return (
    <div className="hz-section-heading">
      <p className="hz-section-index">
        <span aria-hidden>{index}</span>
        <span>{eyebrow}</span>
      </p>
      <div className="hz-section-heading-main">
        <h2 id={id}>{title}</h2>
        {link ? (
          <Link href={link.href} className="hz-link hz-underlined">
            {link.label}
            <ArrowUpRight aria-hidden size={19} />
          </Link>
        ) : null}
      </div>
    </div>
  );
}

/**
 * The booking band: "01 / Your next chapter — Where to?" next to the flat,
 * ruled search form (a plain GET form, native controls).
 */
export function Booking({
  ctx,
  locations,
  index = "01",
}: {
  ctx: Ctx;
  locations: Location[] | null;
  index?: string;
}) {
  const { t, site } = ctx;
  return (
    <div className="hz-booking">
      <div className="hz-booking-label">
        <span>
          {index} / {t("hz.booking.kicker")}
        </span>
        <strong>{t("hz.booking.title")}</strong>
        <ArrowRight aria-hidden size={22} strokeWidth={1} />
      </div>
      {!site.booking_enabled || locations === null ? (
        <Alert>{t("search.booking_off")}</Alert>
      ) : locations.length === 0 ? (
        <Alert>{t("search.no_locations")}</Alert>
      ) : (
        <SearchForm ctx={ctx} locations={locations} id="search" />
      )}
    </div>
  );
}

/** `search` away from the hero: the booking band on its own. */
export function SearchSection({
  ctx,
  locations,
  index,
}: {
  ctx: Ctx;
  locations: Location[] | null;
  index: string;
}) {
  return (
    <section
      className="hz-section hz-search-section"
      aria-label={ctx.t("search.title")}
    >
      <div className="hz-container">
        <Booking ctx={ctx} locations={locations} index={index} />
      </div>
    </section>
  );
}

/**
 * Horizon hero: the serif headline beside a tall photo with a caption
 * strip and a tilted stamp; `heroAlign` centers the copy column. Several
 * slides form a scroll-snap track; the caption's arrow is a plain anchor
 * to the next slide, so it works without JavaScript. Slides without media
 * show the cover image, else a token gradient.
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
  const align = heroAlign(site.settings);
  const slides = heroSlides(ctx);
  const many = slides.length > 1;
  const name = site.organization.name;
  const kicker = localized(site.config.brand?.tagline, ctx.locale) || name;
  const fallback = site.booking_enabled ? "/vehicles" : "/branches";
  return (
    <section
      className={cn("hz-hero", align === "center" && "hz-hero--center")}
      aria-label={t("tw.hero.label")}
      aria-roledescription={many ? t("tw.hero.carousel") : undefined}
    >
      <div className="hz-container">
        <div
          className={cn("hz-hero-track", many && "hz-hero-track--many")}
          tabIndex={many ? 0 : undefined}
        >
          {slides.map((s, i) => {
            const Title = i === 0 ? "h1" : "h2";
            const href = s.cta?.href ?? fallback;
            return (
              <div
                key={i}
                id={many ? `hero-slide-${i + 1}` : undefined}
                className="hz-hero-slide"
                role={many ? "group" : undefined}
                aria-roledescription={many ? t("tw.hero.slide") : undefined}
                aria-label={
                  many
                    ? t("tw.hero.position", { n: i + 1, total: slides.length })
                    : undefined
                }
              >
                <div className="hz-hero-grid">
                  <div className="hz-hero-copy">
                    <span className="hz-kicker">
                      <i aria-hidden />
                      {kicker}
                    </span>
                    {s.title ? (
                      <Title className="hz-hero-title">{s.title}</Title>
                    ) : i === 0 ? (
                      <h1 className="rt-sr-only">{name}</h1>
                    ) : null}
                    <div className="hz-hero-bottom">
                      {s.subtitle ? <p>{s.subtitle}</p> : <span />}
                      <a
                        href={href}
                        className="hz-round hz-round-action"
                        rel={
                          s.cta?.external ? "noopener noreferrer" : undefined
                        }
                      >
                        <ArrowUpRight aria-hidden />
                        <span>{s.cta?.label || t("hz.hero.action")}</span>
                      </a>
                    </div>
                    <div className="hz-hero-footnote" aria-hidden>
                      <span>{t("hz.hero.footnote")}</span>
                      <ArrowDown size={15} />
                    </div>
                  </div>
                  <div
                    className={cn(
                      "hz-hero-visual",
                      !s.image && "hz-hero-visual--plain",
                    )}
                  >
                    {s.image ? (
                      // Library media and the cover come from the API through /media.
                      // eslint-disable-next-line @next/next/no-img-element
                      <img src={s.image} alt="" />
                    ) : null}
                    <div className="hz-photo-caption">
                      <span aria-hidden>
                        {pad(i + 1)} / {pad(slides.length)}
                      </span>
                      <span aria-hidden>{name}</span>
                      {many ? (
                        <a
                          href={`#hero-slide-${((i + 1) % slides.length) + 1}`}
                          aria-label={t("hz.hero.next")}
                        >
                          <ArrowRight aria-hidden size={16} />
                        </a>
                      ) : null}
                    </div>
                    <span className="hz-image-stamp" aria-hidden>
                      <Asterisk size={30} />
                      {t("hz.hero.stamp")}
                    </span>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
        {withSearch ? <Booking ctx={ctx} locations={locations} /> : null}
      </div>
    </section>
  );
}

/** A vehicle group's published conditions as short facts. */
function conditions(ctx: Ctx, s: Subgroup) {
  const { t, locale } = ctx;
  const c = s.conditions;
  if (!c) return [];
  return [
    {
      icon: Users,
      label: t("vehicle.age"),
      value: c.min_driver_age ? `${c.min_driver_age}+` : "—",
    },
    {
      icon: BadgeCheck,
      label: t("vehicle.licence"),
      value: c.min_license_years
        ? t("vehicle.years", { years: c.min_license_years })
        : "—",
    },
    {
      icon: Gauge,
      label: t("vehicle.km"),
      value: c.included_km_per_day
        ? t("vehicle.km_day", { km: c.included_km_per_day })
        : t("vehicle.km_unlimited"),
    },
    {
      icon: Wallet,
      label: t("vehicle.deposit"),
      value:
        Number(c.deposit_amount) > 0
          ? money(c.deposit_amount, "TRY", locale)
          : "—",
    },
  ];
}

/** Home fleet: the first four live vehicle groups as numbered rows. */
export function Fleet({
  ctx,
  subgroups,
  index,
}: {
  ctx: Ctx;
  subgroups: Subgroup[] | null;
  index: string;
}) {
  if (!subgroups?.length) return null;
  const { t } = ctx;
  return (
    <section className="hz-section hz-fleet" aria-labelledby="fleet-title">
      <div className="hz-container">
        <SectionHeading
          id="fleet-title"
          index={index}
          eyebrow={t("hz.fleet.eyebrow")}
          title={t("hz.fleet.title")}
          link={{ href: "/vehicles", label: t("hz.fleet.all") }}
        />
        <ol className="hz-fleet-list">
          {subgroups.slice(0, 4).map((s, i) => {
            const model = s.representative_model || s.title;
            const image = mediaUrl(s.images?.[0]);
            const sub = [
              ...new Set(
                [s.title !== model ? s.title : "", s.segment].filter(Boolean),
              ),
            ].join(" · ");
            const facts = conditions(ctx, s);
            return (
              <li key={s.uuid} className="hz-vehicle">
                <span className="hz-vehicle-index" aria-hidden>
                  {pad(i + 1)}
                </span>
                <div className="hz-vehicle-image">
                  {image ? (
                    // Product images come from the API through /media.
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={image} alt={model} loading="lazy" />
                  ) : (
                    <Car aria-hidden size={56} strokeWidth={1} />
                  )}
                </div>
                <div className="hz-vehicle-info">
                  {sub ? <span className="hz-kicker">{sub}</span> : null}
                  <h3>{model}</h3>
                  {facts.length ? (
                    <ul
                      className="hz-vehicle-specs"
                      aria-label={t("hz.fleet.conditions")}
                    >
                      {facts.map(({ icon: Icon, label, value }) => (
                        <li key={label} title={label}>
                          <Icon aria-hidden />
                          <span className="rt-sr-only">{label}: </span>
                          {value}
                        </li>
                      ))}
                    </ul>
                  ) : null}
                  <p>{t("vehicle.similar")}</p>
                </div>
                <Link href="/search" className="hz-vehicle-action">
                  <span>
                    {t("fleet.rent")}
                    <span className="rt-sr-only">: {model}</span>
                  </span>
                  <ArrowUpRight aria-hidden strokeWidth={1} />
                </Link>
              </li>
            );
          })}
        </ol>
      </div>
    </section>
  );
}

/** Home campaigns: an asymmetric collage of up to three live campaigns. */
export function Campaigns({ ctx, index }: { ctx: Ctx; index: string }) {
  const { t } = ctx;
  const all = campaignCards(ctx.site, ctx.locale);
  if (!all.length) return null;
  const cards = all.slice(0, 3);
  return (
    <section
      className="hz-section hz-escapes"
      aria-labelledby="campaigns-title"
    >
      <div className="hz-container">
        <SectionHeading
          id="campaigns-title"
          index={index}
          eyebrow={t("hz.campaigns.eyebrow")}
          title={t("hz.campaigns.title")}
          link={{ href: "/campaigns", label: t("tw.campaigns.all") }}
        />
        <div
          className={cn(
            "hz-escape-grid",
            cards.length === 1 && "hz-escape-grid--single",
          )}
        >
          {cards.map((card, i) => {
            const title = card.title || t("tw.campaigns.default_title");
            const href = `/campaigns/${card.uuid}`;
            return (
              <article
                key={card.uuid}
                className={cn("hz-escape", i === 0 && "hz-escape--lead")}
              >
                {/* The image repeats the caption link: hidden from AT and tab order. */}
                <Link
                  href={href}
                  className={cn(
                    "hz-image-link",
                    !card.image && "hz-image-link--plain",
                  )}
                  tabIndex={-1}
                  aria-hidden
                >
                  {card.image ? (
                    // Library media comes from the API through /media.
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={card.image} alt="" loading="lazy" />
                  ) : null}
                  <span>
                    <MoveUpRight size={28} strokeWidth={1.2} />
                  </span>
                </Link>
                <div className="hz-story-caption">
                  <span className="hz-kicker">{validityText(ctx, card)}</span>
                  <h3>{title}</h3>
                  {card.summary ? <p>{card.summary}</p> : null}
                  {card.code ? (
                    <CampaignCode ctx={ctx} code={card.code} />
                  ) : null}
                  <Link href={href} className="hz-link hz-underlined">
                    <span>
                      {card.cta || t("tw.campaigns.cta")}
                      <span className="rt-sr-only">: {title}</span>
                    </span>
                    <ArrowUpRight aria-hidden size={18} />
                  </Link>
                </div>
              </article>
            );
          })}
        </div>
      </div>
    </section>
  );
}

/**
 * The dark story band: serif title, the organization's about text, its
 * image (or a token gradient) and the highlights as numbered promises.
 * `full` (the about page) shows the whole text and no "more" link.
 */
export function About({ ctx, full = false }: { ctx: Ctx; full?: boolean }) {
  const { t, content, site } = ctx;
  if (!content.about) return null;
  const highlights = aboutHighlights(ctx);
  const src = siteMediaUrl(site, site.config.about?.image);
  const tagline = localized(site.config.brand?.tagline, ctx.locale);
  return (
    <section className="hz-about" aria-labelledby="about-title">
      <div className="hz-container">
        <div className="hz-about-top">
          <span className="hz-kicker">{t("hz.about.kicker")}</span>
          <Asterisk aria-hidden size={52} strokeWidth={1} />
        </div>
        <div className="hz-about-grid">
          <div>
            <h2 id="about-title">{t("hz.about.title")}</h2>
            <Paragraphs
              text={content.about}
              limit={full ? undefined : 2}
              className="hz-about-text"
            />
            {full ? null : (
              <Link href="/about" className="hz-link hz-about-link">
                {t("hz.about.cta")}
                <ArrowUpRight aria-hidden size={21} />
              </Link>
            )}
          </div>
          <figure className={cn("hz-about-figure", !src && "hz-plain")}>
            {src ? (
              // Library media comes from the API through /media.
              // eslint-disable-next-line @next/next/no-img-element
              <img src={src} alt="" loading="lazy" />
            ) : (
              <span aria-hidden />
            )}
            <figcaption>
              {site.organization.name}
              {tagline ? ` — ${tagline}` : ""}
            </figcaption>
          </figure>
        </div>
        {highlights.length ? (
          <ul className="hz-promises" aria-label={t("hz.about.highlights")}>
            {highlights.map((h, i) => (
              <li key={i}>
                <span aria-hidden>{pad(i + 1)}</span>
                <p>{h}</p>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    </section>
  );
}

/**
 * Questions beside a serif title, as native <details> rows (no
 * JavaScript). The home shows six with a link to all.
 */
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
    <section className="hz-section hz-faq-section" aria-labelledby="faq-title">
      <div className="hz-container hz-faq">
        <div className="hz-faq-intro">
          <span className="hz-kicker">{t("hz.faq.kicker")}</span>
          <h2 id="faq-title">{t("hz.faq.title")}</h2>
          <div className="hz-faq-links">
            {more ? (
              <Link href="/faq" className="hz-link hz-underlined">
                {t("tw.faq.all")}
                <ArrowUpRight aria-hidden size={18} />
              </Link>
            ) : null}
            {hasContactPage(ctx) ? (
              <Link href="/contact" className="hz-link hz-underlined">
                {t("hz.faq.contact")}
                <ArrowUpRight aria-hidden size={18} />
              </Link>
            ) : null}
          </div>
        </div>
        <div className="hz-faq-items">
          {items.map((f, i) => (
            <details key={i} className="hz-faq-item">
              <summary>
                <span className="hz-faq-number" aria-hidden>
                  {pad(i + 1)}
                </span>
                <span className="hz-faq-question">{f.question}</span>
                <Plus aria-hidden size={16} strokeWidth={1.2} />
              </summary>
              <Paragraphs text={f.answer} className="hz-faq-answer" />
            </details>
          ))}
        </div>
      </div>
    </section>
  );
}
