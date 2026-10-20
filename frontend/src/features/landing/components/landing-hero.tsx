"use client";

import { Fragment, useRef } from "react";
import Link from "next/link";
import { Check } from "lucide-react";
import {
  motion,
  useReducedMotion,
  useScroll,
  useSpring,
  useTransform,
} from "motion/react";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

import { LANDING_LOGIN_HREF, demoMailto } from "../config";
import { HeroVideo } from "./hero-video";
import { HeroShader } from "./landing-shader";
import { EASE_OUT } from "./motion-primitives";

const FACTS = [
  "landing.hero.fact_language",
  "landing.hero.fact_branches",
  "landing.hero.fact_site",
] as const;

const WORD_STAGGER = 0.07;

/**
 * Ink band with the brand mesh. The headline arrives word by word, then the
 * lead and buttons; the product frame rises last, tilted back, and lies flat
 * as the visitor scrolls it into view while the copy drifts away slower.
 */
export function LandingHero() {
  const { t } = useLocale();
  const reduceMotion = useReducedMotion();
  const ref = useRef<HTMLElement>(null);
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start start", "end start"],
  });
  const progress = useSpring(scrollYProgress, {
    stiffness: 120,
    damping: 30,
    mass: 0.3,
  });

  const copyY = useTransform(progress, [0, 1], [0, -120]);
  const copyOpacity = useTransform(progress, [0, 0.55], [1, 0]);
  const frameRotate = useTransform(progress, [0, 0.45], [16, 0]);
  const frameScale = useTransform(progress, [0, 0.45], [0.92, 1]);
  const frameY = useTransform(progress, [0, 1], [0, -60]);

  const words = t("landing.hero.title").split(" ");
  const afterHeadline = words.length * WORD_STAGGER + 0.15;

  return (
    <section
      ref={ref}
      aria-labelledby="landing-hero-title"
      className="relative"
    >
      <div className="dark relative overflow-hidden bg-[#141527] text-white">
        <HeroShader className="absolute inset-0" />
        <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(180deg,rgba(20,21,39,0.55)_0%,rgba(20,21,39,0.15)_45%,rgba(20,21,39,0.75)_100%)]" />

        <motion.div
          style={reduceMotion ? undefined : { y: copyY, opacity: copyOpacity }}
          className="relative mx-auto max-w-[78rem] px-4 pt-36 pb-44 sm:px-6 sm:pt-44 sm:pb-64 lg:px-8 lg:pt-48 lg:pb-80"
        >
          <h1
            id="landing-hero-title"
            className="max-w-[15ch] text-[2.75rem] leading-[1.02] font-bold tracking-[-0.035em] text-balance sm:text-6xl lg:text-[5.5rem]"
          >
            {words.map((word, index) => (
              <Fragment key={`${word}-${index}`}>
                {index > 0 ? " " : null}
                <span className="inline-block overflow-hidden pb-[0.08em] align-bottom">
                  <motion.span
                    className="inline-block"
                    initial={{ y: "105%", opacity: 0 }}
                    animate={{ y: 0, opacity: 1 }}
                    transition={{
                      duration: 0.8,
                      delay: 0.15 + index * WORD_STAGGER,
                      ease: EASE_OUT,
                    }}
                  >
                    {word}
                  </motion.span>
                </span>
              </Fragment>
            ))}
          </h1>

          <div className="mt-10 grid gap-10 lg:mt-14 lg:grid-cols-12 lg:gap-8">
            <motion.div
              className="lg:col-span-6"
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.7, delay: afterHeadline }}
            >
              <p className="max-w-[52ch] text-lg leading-relaxed text-pretty text-white/75 sm:text-xl">
                {t("landing.hero.lead")}
              </p>
              <div className="mt-8 flex flex-wrap gap-3">
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
                  <Link href={LANDING_LOGIN_HREF}>
                    {t("landing.cta.login")}
                  </Link>
                </Button>
              </div>
            </motion.div>

            <ul
              aria-label={t("landing.hero.facts_label")}
              className="grid content-end gap-3 text-sm text-white/80 sm:text-base lg:col-span-5 lg:col-start-8"
            >
              {FACTS.map((key, index) => (
                <motion.li
                  key={key}
                  initial={{ opacity: 0, x: 24 }}
                  animate={{ opacity: 1, x: 0 }}
                  transition={{
                    duration: 0.6,
                    delay: afterHeadline + 0.1 + index * 0.08,
                  }}
                  className="flex items-center gap-3 border-t border-white/15 pt-3"
                >
                  <Check className="size-4 shrink-0 text-[#8E95EC]" />
                  {t(key)}
                </motion.li>
              ))}
            </ul>
          </div>
        </motion.div>
      </div>

      <div className="relative mx-auto -mt-32 max-w-[78rem] px-4 [perspective:1600px] sm:-mt-52 sm:px-6 lg:-mt-64 lg:px-8">
        <motion.div
          initial={{ opacity: 0, y: 80 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ duration: 1, delay: afterHeadline + 0.2 }}
        >
          <motion.div
            style={
              reduceMotion
                ? undefined
                : {
                    rotateX: frameRotate,
                    scale: frameScale,
                    y: frameY,
                    transformOrigin: "50% 0%",
                  }
            }
          >
            <HeroVideo />
          </motion.div>
        </motion.div>
      </div>
    </section>
  );
}
