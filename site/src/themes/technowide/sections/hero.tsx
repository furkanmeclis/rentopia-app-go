import { ArrowUpRight } from "lucide-react";
import type { CSSProperties } from "react";

import { SearchForm } from "@/components/site/search-form";
import { Alert } from "@/components/ui/card";
import { cn } from "@/lib/cn";
import { isExternal, safeHref } from "@/lib/links";
import { siteMediaUrl } from "@/lib/media";
import { mediaUrl, type Ctx } from "@/lib/server";
import type { HeroSlide, Location } from "@/lib/types";

type Slide = {
  title: string;
  subtitle: string;
  cta: { href: string; label: string; external: boolean } | null;
  image: string | null;
};

/**
 * Enabled slides of the visitor's locale (1–8 in config v2), falling back
 * to the legacy hero text and then the template's default copy. The cover
 * image stands in for slides without library media.
 */
export function heroSlides(ctx: Ctx): Slide[] {
  const { content, site, t } = ctx;
  const cover = mediaUrl(site.assets.cover);
  const source: HeroSlide[] = (content.hero_slides ?? []).filter(
    (s) => s && s.enabled,
  );
  const slides = source.map((s): Slide => {
    const href = safeHref(s.cta_href);
    return {
      title: s.title,
      subtitle: s.subtitle,
      cta:
        href && s.cta_label
          ? { href, label: s.cta_label, external: isExternal(href) }
          : null,
      image: siteMediaUrl(site, s.media) ?? cover,
    };
  });
  const usable = slides.filter((s) => s.title || s.subtitle);
  if (usable.length) return usable.slice(0, 8);
  return [
    {
      title: content.hero_title || t("hero.title"),
      subtitle: content.hero_subtitle || t("hero.subtitle"),
      cta: null,
      image: cover,
    },
  ];
}

const bg = (image: string | null): CSSProperties | undefined =>
  image
    ? ({
        "--tw-slide-image": `url("${image.replace(/"/g, "")}")`,
      } as CSSProperties)
    : undefined;

/**
 * Technowide hero: the story (slides) and, when `search` follows the hero,
 * the booking card. `heroLayout=image` draws full-bleed photos behind both;
 * `split` puts the story in a rounded photo card next to the search. More
 * slides form a horizontal scroll-snap track with anchor dots: every slide
 * is in the HTML and readable without JavaScript.
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
  const layout = site.settings?.heroLayout === "split" ? "split" : "image";
  const slides = heroSlides(ctx);
  const many = slides.length > 1;

  const search = !withSearch ? null : !site.booking_enabled ||
    locations === null ? (
    <div className="rt-search tw-hero-search">
      <Alert>{t("search.booking_off")}</Alert>
    </div>
  ) : locations.length === 0 ? (
    <div className="rt-search tw-hero-search">
      <Alert>{t("search.no_locations")}</Alert>
    </div>
  ) : (
    <div className="tw-hero-search">
      <SearchForm
        ctx={ctx}
        locations={locations}
        id="search"
        title={t("tw.search.title")}
        tagline={t("tw.search.tagline")}
      />
    </div>
  );

  return (
    <section
      className={cn(
        "tw-hero",
        `tw-hero--${layout}`,
        withSearch && "tw-hero--with-search",
      )}
      aria-roledescription={many ? t("tw.hero.carousel") : undefined}
      aria-label={t("tw.hero.label")}
    >
      <div className="tw-hero-track" tabIndex={many ? 0 : undefined}>
        {slides.map((s, i) => {
          const Title = i === 0 ? "h1" : "h2";
          return (
            <div
              key={i}
              id={many ? `hero-slide-${i + 1}` : undefined}
              className={cn(
                "tw-hero-slide",
                !s.image && "tw-hero-slide--plain",
              )}
              style={bg(s.image)}
              role={many ? "group" : undefined}
              aria-roledescription={many ? t("tw.hero.slide") : undefined}
              aria-label={
                many
                  ? t("tw.hero.position", { n: i + 1, total: slides.length })
                  : undefined
              }
            >
              <div className="rt-container tw-hero-grid">
                <div className="tw-hero-story">
                  <div className="tw-hero-copy">
                    <span className="tw-hero-eyebrow">
                      <i aria-hidden />
                      {site.organization.name}
                    </span>
                    {s.title ? (
                      <Title>{s.title}</Title>
                    ) : i === 0 ? (
                      <h1 className="rt-sr-only">{site.organization.name}</h1>
                    ) : null}
                    {s.subtitle ? <p>{s.subtitle}</p> : null}
                    {s.cta ? (
                      <a
                        href={s.cta.href}
                        className="rt-button rt-button--primary tw-hero-cta"
                        rel={s.cta.external ? "noopener noreferrer" : undefined}
                      >
                        {s.cta.label}
                        <ArrowUpRight aria-hidden size={18} />
                      </a>
                    ) : null}
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </div>
      {many ? (
        <div className="rt-container tw-hero-controls-wrap">
          <nav className="tw-hero-controls" aria-label={t("tw.hero.slides")}>
            {slides.map((_, i) => (
              <a
                key={i}
                href={`#hero-slide-${i + 1}`}
                aria-label={t("tw.hero.position", {
                  n: i + 1,
                  total: slides.length,
                })}
              />
            ))}
          </nav>
        </div>
      ) : null}
      {search ? (
        <div className="tw-hero-overlay">
          <div className="rt-container tw-hero-grid">
            <div className="tw-hero-spacer" aria-hidden />
            {search}
          </div>
        </div>
      ) : null}
    </section>
  );
}
