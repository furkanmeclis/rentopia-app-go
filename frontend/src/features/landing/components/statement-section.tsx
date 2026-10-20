"use client";

import { useEffect, useRef } from "react";
import {
  animate,
  motion,
  useInView,
  useMotionValue,
  useReducedMotion,
  useScroll,
  useTransform,
  type MotionValue,
} from "motion/react";

import { useLocale } from "@/providers/locale-provider";

import { Reveal } from "./motion-primitives";

// Product facts, each one checked against the code: the five lifecycle
// steps below, the four site templates, the three custom domains a company
// may connect, and the two complete panel languages.
const FACTS = [
  { value: 5, key: "landing.statement.fact1" },
  { value: 4, key: "landing.statement.fact2" },
  { value: 3, key: "landing.statement.fact3" },
  { value: 2, key: "landing.statement.fact4" },
] as const;

function Word({
  word,
  index,
  total,
  progress,
}: {
  word: string;
  index: number;
  total: number;
  progress: MotionValue<number>;
}) {
  const start = index / total;
  const end = Math.min(1, start + 1.5 / total);
  const opacity = useTransform(progress, [start, end], [0.14, 1]);
  return (
    <motion.span style={{ opacity }} className="inline">
      {word}{" "}
    </motion.span>
  );
}

/**
 * Counts from zero to `value` the first time it scrolls into view. The
 * server renders the real number; the client drops it to zero only to
 * count up, so the figure is right without JavaScript or with less motion.
 */
function CountUp({ value }: { value: number }) {
  const ref = useRef<HTMLSpanElement>(null);
  const inView = useInView(ref, { once: true, margin: "0px 0px -15% 0px" });
  const reduceMotion = useReducedMotion();
  const count = useMotionValue(value);
  const rounded = useTransform(count, (v) => Math.round(v));

  useEffect(() => {
    if (reduceMotion) {
      count.set(value);
      return;
    }
    if (!inView) {
      count.set(0);
      return;
    }
    const controls = animate(count, value, {
      duration: 1.1,
      ease: [0.22, 1, 0.36, 1],
    });
    return () => controls.stop();
  }, [count, inView, reduceMotion, value]);

  return (
    <motion.span ref={ref} className="tabular-nums">
      {rounded}
    </motion.span>
  );
}

/**
 * One large sentence that lights up word by word as it crosses the
 * viewport (scroll-linked, never blocks scrolling), then four product
 * facts that count up. With reduced motion every word is fully visible.
 */
export function StatementSection() {
  const { t } = useLocale();
  const reduceMotion = useReducedMotion();
  const ref = useRef<HTMLParagraphElement>(null);
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start 85%", "end 45%"],
  });
  const text = t("landing.statement.text");
  const words = text.split(" ");

  return (
    <section aria-label={t("landing.statement.facts_label")}>
      <div className="mx-auto max-w-[78rem] px-4 pt-28 pb-20 sm:px-6 sm:pt-40 lg:px-8 lg:pb-28">
        <p
          ref={ref}
          className="max-w-[22ch] text-[2.25rem] leading-[1.08] font-bold tracking-[-0.035em] text-balance sm:text-6xl lg:text-[4.75rem]"
        >
          {reduceMotion ? (
            text
          ) : (
            <>
              <span className="sr-only">{text}</span>
              <span aria-hidden>
                {words.map((word, index) => (
                  <Word
                    key={`${word}-${index}`}
                    word={word}
                    index={index}
                    total={words.length}
                    progress={scrollYProgress}
                  />
                ))}
              </span>
            </>
          )}
        </p>

        <dl className="mt-20 grid grid-cols-2 gap-x-6 gap-y-10 lg:mt-28 lg:grid-cols-4 lg:gap-8">
          {FACTS.map((fact, index) => (
            <Reveal
              key={fact.key}
              delay={index * 0.08}
              className="border-t pt-5"
            >
              <dt className="sr-only">{t(fact.key)}</dt>
              <dd>
                <span className="text-primary block text-6xl leading-none font-bold tracking-[-0.04em] sm:text-7xl">
                  <CountUp value={fact.value} />
                </span>
                <span
                  aria-hidden
                  className="text-muted-foreground mt-3 block max-w-[24ch] text-[0.9375rem] leading-snug"
                >
                  {t(fact.key)}
                </span>
              </dd>
            </Reveal>
          ))}
        </dl>
      </div>
    </section>
  );
}
