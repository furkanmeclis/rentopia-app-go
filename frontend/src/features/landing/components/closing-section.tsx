"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

import { LANDING_LOGIN_HREF, demoMailto } from "../config";
import { ClosingShader } from "./landing-shader";
import { Reveal } from "./motion-primitives";

/** Closing call to action on the grain shader band; always dark. */
export function ClosingSection() {
  const { t } = useLocale();

  return (
    <section
      aria-labelledby="landing-closing-title"
      className="dark relative overflow-hidden bg-[#141527] text-white"
    >
      <ClosingShader className="absolute inset-0" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(90deg,rgba(20,21,39,0.85)_0%,rgba(20,21,39,0.35)_70%)]" />
      <Reveal className="relative mx-auto max-w-[78rem] px-4 py-24 sm:px-6 lg:px-8 lg:py-36">
        <h2
          id="landing-closing-title"
          className="max-w-[18ch] text-4xl leading-[1.05] font-bold tracking-[-0.035em] text-balance sm:text-6xl"
        >
          {t("landing.closing.title")}
        </h2>
        <p className="mt-6 max-w-[52ch] text-lg leading-relaxed text-white/75">
          {t("landing.closing.body")}
        </p>
        <div className="mt-10 flex flex-wrap gap-3">
          <Button
            asChild
            size="lg"
            className="h-12 rounded-full px-6 text-base transition-transform hover:-translate-y-0.5"
          >
            <a href={demoMailto(t("landing.cta.demo_subject"))}>
              {t("landing.cta.demo")}
            </a>
          </Button>
          <Button
            asChild
            size="lg"
            variant="outline"
            className="h-12 rounded-full border-white/25 bg-transparent px-6 text-base text-white transition-transform hover:-translate-y-0.5 hover:bg-white/10 hover:text-white dark:bg-transparent dark:hover:bg-white/10"
          >
            <Link href={LANDING_LOGIN_HREF}>{t("landing.cta.login")}</Link>
          </Button>
        </div>
      </Reveal>
    </section>
  );
}
