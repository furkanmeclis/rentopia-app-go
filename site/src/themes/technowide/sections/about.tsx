import { ArrowUpRight, Check } from "lucide-react";
import Link from "next/link";

import { Paragraphs } from "@/components/site/text";
import { siteMediaUrl } from "@/lib/media";
import type { Ctx } from "@/lib/server";

import { Eyebrow } from "../parts";

/** The about image (library media) or a neutral gradient. */
export function AboutImage({ ctx, caption }: { ctx: Ctx; caption?: boolean }) {
  const src = siteMediaUrl(ctx.site, ctx.site.config.about?.image);
  return (
    <div
      className={
        src ? "tw-about-image" : "tw-about-image tw-about-image--plain"
      }
    >
      {src ? (
        // Library media comes from the API through /media (same origin).
        // eslint-disable-next-line @next/next/no-img-element
        <img src={src} alt="" loading="lazy" />
      ) : null}
      {caption ? (
        <div className="tw-about-caption" aria-hidden>
          <ArrowUpRight size={35} />
          <span>{ctx.t("tw.about.caption")}</span>
        </div>
      ) : null}
    </div>
  );
}

export function aboutHighlights(ctx: Ctx): string[] {
  const list = ctx.site.config.about?.highlights?.[ctx.locale] ?? [];
  return list.filter((h) => typeof h === "string" && h.trim()).slice(0, 6);
}

/** Home about: story teaser, highlights and the image. */
export function About({ ctx }: { ctx: Ctx }) {
  const { t, content, site } = ctx;
  if (!content.about) return null;
  const highlights = aboutHighlights(ctx);
  return (
    <section className="tw-section tw-about" aria-labelledby="about-title">
      <div className="rt-container tw-about-grid">
        <AboutImage ctx={ctx} caption />
        <div>
          <Eyebrow>{t("tw.about.eyebrow")}</Eyebrow>
          <h2 id="about-title">
            {t("tw.about.title", { name: site.organization.name })}
          </h2>
          <Paragraphs
            text={content.about}
            limit={2}
            className="tw-about-text"
          />
          {highlights.length ? (
            <ul className="tw-checklist">
              {highlights.map((h, i) => (
                <li key={i}>
                  <Check aria-hidden size={17} />
                  {h}
                </li>
              ))}
            </ul>
          ) : null}
          <Link href="/about" className="tw-text-button">
            {t("tw.about.cta")}
            <ArrowUpRight aria-hidden size={18} />
          </Link>
        </div>
      </div>
    </section>
  );
}
