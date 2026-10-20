"use client";

import { useLocale } from "@/providers/locale-provider";

import { PermissionsMock } from "../mockups/permissions-mock";
import { LANDING_SECTIONS } from "./landing-header";
import { HoverLift, Reveal } from "./motion-primitives";
import { SectionHeading } from "./section-heading";

const ITEMS = [
  { title: "landing.control.i1_title", body: "landing.control.i1_body" },
  { title: "landing.control.i2_title", body: "landing.control.i2_body" },
  { title: "landing.control.i3_title", body: "landing.control.i3_body" },
  { title: "landing.control.i4_title", body: "landing.control.i4_body" },
] as const;

/** Branches, roles and permissions, sign-in and data separation. */
export function ControlSection() {
  const { t } = useLocale();

  return (
    <section
      id={LANDING_SECTIONS.control}
      aria-labelledby="landing-control-title"
      className="scroll-mt-16"
    >
      <div className="mx-auto max-w-[78rem] px-4 py-24 sm:px-6 lg:px-8 lg:py-32">
        <Reveal>
          <SectionHeading
            id="landing-control-title"
            title={t("landing.control.title")}
            lead={t("landing.control.body")}
          />
        </Reveal>

        <div className="mt-14 grid grid-cols-1 items-start gap-12 lg:grid-cols-12 lg:gap-8">
          <figure className="lg:col-span-6">
            <Reveal y={48}>
              <HoverLift>
                <PermissionsMock />
              </HoverLift>
            </Reveal>
            <figcaption className="sr-only">
              {t("landing.control.mock_caption")}
            </figcaption>
          </figure>

          <dl className="grid gap-x-8 gap-y-10 sm:grid-cols-2 lg:col-span-5 lg:col-start-8">
            {ITEMS.map((item, index) => (
              <Reveal
                key={item.title}
                delay={0.1 + index * 0.08}
                y={20}
                className="border-t pt-4"
              >
                <dt className="font-semibold">{t(item.title)}</dt>
                <dd className="text-muted-foreground mt-2 leading-relaxed">
                  {t(item.body)}
                </dd>
              </Reveal>
            ))}
          </dl>
        </div>
      </div>
    </section>
  );
}
