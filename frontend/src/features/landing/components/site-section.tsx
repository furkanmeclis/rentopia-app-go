"use client";

import { useLocale } from "@/providers/locale-provider";

import { SiteMock } from "../mockups/site-mock";
import { LANDING_SECTIONS } from "./landing-header";
import { Reveal } from "./motion-primitives";
import { SectionHeading } from "./section-heading";

const ITEMS = [
  { title: "landing.site.i1_title", body: "landing.site.i1_body" },
  { title: "landing.site.i2_title", body: "landing.site.i2_body" },
  { title: "landing.site.i3_title", body: "landing.site.i3_body" },
  { title: "landing.site.i4_title", body: "landing.site.i4_body" },
  { title: "landing.site.i5_title", body: "landing.site.i5_body" },
] as const;

/** The branded booking site, the custom domain and the embeddable widget. */
export function SiteSection() {
  const { t } = useLocale();

  return (
    <section
      id={LANDING_SECTIONS.site}
      aria-labelledby="landing-site-title"
      className="scroll-mt-16 border-y bg-[#F3F4FA] dark:bg-white/[0.03]"
    >
      <div className="mx-auto grid max-w-[78rem] grid-cols-1 gap-14 px-4 py-24 sm:px-6 lg:grid-cols-12 lg:gap-8 lg:px-8 lg:py-32">
        <div className="lg:col-span-5">
          <Reveal>
            <SectionHeading
              id="landing-site-title"
              title={t("landing.site.title")}
              lead={t("landing.site.body")}
            />
          </Reveal>
          <dl className="mt-10 grid">
            {ITEMS.map((item, index) => (
              <Reveal
                key={item.title}
                delay={index * 0.06}
                y={16}
                className="grid gap-1 border-t border-[#141527]/10 py-4 sm:grid-cols-[10rem_1fr] sm:gap-6 dark:border-white/10"
              >
                <dt className="font-semibold">{t(item.title)}</dt>
                <dd className="text-muted-foreground leading-relaxed">
                  {t(item.body)}
                </dd>
              </Reveal>
            ))}
          </dl>
        </div>

        <figure className="lg:sticky lg:top-28 lg:col-span-7 lg:col-start-6 lg:self-start">
          <Reveal y={56} delay={0.1}>
            <SiteMock />
          </Reveal>
          <figcaption className="sr-only">
            {t("landing.site.mock_caption")}
          </figcaption>
        </figure>
      </div>
    </section>
  );
}
