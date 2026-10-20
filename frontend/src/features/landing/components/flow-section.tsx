"use client";

import { useRef, useState, type ReactNode } from "react";
import {
  AnimatePresence,
  motion,
  useMotionValueEvent,
  useReducedMotion,
  useScroll,
  useSpring,
} from "motion/react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { CloseoutMock, ContractMock, ReturnMock } from "../mockups/flow-mocks";
import { HandoverMock } from "../mockups/handover-mock";
import { ReservationMock } from "../mockups/reservation-mock";
import { LANDING_SECTIONS } from "./landing-header";
import { EASE_OUT, Reveal } from "./motion-primitives";
import { SectionHeading } from "./section-heading";

type Step = {
  title: string;
  body: string;
  state: string;
  points: readonly string[];
  mock: ReactNode;
};

const STEPS: Step[] = [
  {
    title: "landing.flow.step1_title",
    body: "landing.flow.step1_body",
    state: "landing.flow.step1_state",
    points: ["landing.booking.p1", "landing.booking.p2", "landing.booking.p3"],
    mock: <ReservationMock />,
  },
  {
    title: "landing.flow.step2_title",
    body: "landing.flow.step2_body",
    state: "landing.flow.step2_state",
    points: ["landing.handover.p1"],
    mock: <ContractMock />,
  },
  {
    title: "landing.flow.step3_title",
    body: "landing.flow.step3_body",
    state: "landing.flow.step3_state",
    points: ["landing.handover.p2", "landing.handover.p3"],
    mock: <HandoverMock />,
  },
  {
    title: "landing.flow.step4_title",
    body: "landing.flow.step4_body",
    state: "landing.flow.step4_state",
    points: ["landing.connect.tolls", "landing.handover.p4"],
    mock: <ReturnMock />,
  },
  {
    title: "landing.flow.step5_title",
    body: "landing.flow.step5_body",
    state: "landing.flow.step5_state",
    points: ["landing.finance.p1", "landing.connect.einvoice"],
    mock: <CloseoutMock />,
  },
];

function StateChip({ label, state }: { label: string; state: string }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full bg-[#ECEDF8] px-3 py-1 text-xs font-medium text-[#2E358C] dark:bg-[#22264A] dark:text-[#C7CAF6]">
      <span className="size-1.5 rounded-full bg-current" />
      <span className="sr-only">{label}: </span>
      {state}
    </span>
  );
}

/**
 * Scroll-linked walkthrough: the section is five screens tall and its
 * inner stage stays pinned (position: sticky) while the page scrolls
 * normally. Scroll progress picks the active step; the product frame swaps
 * to that step's screen and a bar fills along. Nothing intercepts the wheel,
 * keyboard or touch. Clicking a step scrolls to it.
 */
function PinnedFlow() {
  const { t } = useLocale();
  const ref = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState(0);
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start start", "end end"],
  });
  const bar = useSpring(scrollYProgress, { stiffness: 200, damping: 40 });

  useMotionValueEvent(scrollYProgress, "change", (v) => {
    setActive(
      Math.min(STEPS.length - 1, Math.max(0, Math.floor(v * STEPS.length))),
    );
  });

  const goTo = (index: number) => {
    const el = ref.current;
    if (!el) return;
    const top = el.getBoundingClientRect().top + window.scrollY;
    const travel = el.offsetHeight - window.innerHeight;
    window.scrollTo({
      top: top + travel * ((index + 0.5) / STEPS.length),
      behavior: "smooth",
    });
  };

  const step = STEPS[active]!;

  return (
    <div
      ref={ref}
      className="relative"
      style={{ height: `${STEPS.length * 90 + 40}svh` }}
    >
      <div className="sticky top-0 flex h-svh items-center overflow-hidden">
        <div className="mx-auto grid w-full max-w-[78rem] grid-cols-1 gap-6 px-4 pt-20 pb-6 sm:px-6 lg:grid-cols-12 lg:gap-8 lg:px-8 lg:pt-16">
          {/* Steps */}
          <div className="order-2 lg:order-1 lg:col-span-5 lg:self-center">
            <div className="text-muted-foreground mb-4 flex items-center gap-3 text-sm font-medium tabular-nums">
              <span aria-live="polite">
                {t("landing.flow.progress", {
                  n: active + 1,
                  total: STEPS.length,
                })}
              </span>
              <span className="bg-border relative h-1 flex-1 overflow-hidden rounded-full">
                <motion.span
                  className="bg-primary absolute inset-0 origin-left rounded-full"
                  style={{ scaleX: bar }}
                />
              </span>
            </div>

            <ol className="grid gap-1">
              {STEPS.map((s, index) => {
                const isActive = index === active;
                return (
                  <li key={s.title}>
                    <button
                      type="button"
                      onClick={() => goTo(index)}
                      aria-current={isActive ? "step" : undefined}
                      className="focus-visible:ring-ring/50 group relative flex w-full items-start gap-4 rounded-2xl px-3 py-2.5 text-left outline-none focus-visible:ring-[3px] lg:py-3"
                    >
                      {isActive ? (
                        <motion.span
                          layoutId="landing-flow-active"
                          className="bg-muted/70 absolute inset-0 rounded-2xl dark:bg-white/[0.05]"
                          transition={{
                            type: "spring",
                            stiffness: 380,
                            damping: 34,
                          }}
                        />
                      ) : null}
                      <span
                        className={cn(
                          "relative grid size-8 shrink-0 place-items-center rounded-full text-sm font-semibold tabular-nums transition-colors",
                          isActive
                            ? "bg-primary text-primary-foreground"
                            : "ring-border text-muted-foreground ring-1",
                        )}
                      >
                        {index + 1}
                      </span>
                      <span className="relative min-w-0 flex-1">
                        <span
                          className={cn(
                            "block text-lg font-semibold tracking-tight transition-colors lg:text-xl",
                            isActive
                              ? "text-foreground"
                              : "text-muted-foreground group-hover:text-foreground",
                          )}
                        >
                          {t(s.title)}
                        </span>
                        <AnimatePresence initial={false}>
                          {isActive ? (
                            <motion.span
                              key="detail"
                              initial={{ height: 0, opacity: 0 }}
                              animate={{ height: "auto", opacity: 1 }}
                              exit={{ height: 0, opacity: 0 }}
                              transition={{ duration: 0.4, ease: EASE_OUT }}
                              className="block overflow-hidden"
                            >
                              <span className="text-muted-foreground block pt-1.5 text-[0.9375rem] leading-relaxed">
                                {t(s.body)}
                              </span>
                              <span className="hidden flex-wrap gap-1.5 pt-3 sm:flex">
                                {s.points.map((p) => (
                                  <span
                                    key={p}
                                    className="bg-card rounded-full border px-2.5 py-1 text-xs"
                                  >
                                    {t(p)}
                                  </span>
                                ))}
                              </span>
                            </motion.span>
                          ) : null}
                        </AnimatePresence>
                      </span>
                    </button>
                  </li>
                );
              })}
            </ol>
          </div>

          {/* Stage */}
          <div className="order-1 lg:order-2 lg:col-span-7 lg:col-start-6">
            <div className="relative grid h-[42svh] place-items-center overflow-hidden rounded-[2rem] border bg-[#F3F4FA] sm:h-[46svh] lg:h-[min(78svh,44rem)] dark:bg-white/[0.03]">
              <div
                aria-hidden
                className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_70%_20%,rgba(90,98,196,0.14),transparent_55%)]"
              />
              <div className="absolute top-4 left-5">
                <StateChip
                  label={t("landing.flow.state_label")}
                  state={t(step.state)}
                />
              </div>
              <AnimatePresence mode="popLayout" initial={false}>
                <motion.div
                  key={active}
                  initial={{
                    opacity: 0,
                    y: 48,
                    scale: 0.96,
                    filter: "blur(6px)",
                  }}
                  animate={{ opacity: 1, y: 0, scale: 1, filter: "blur(0px)" }}
                  exit={{
                    opacity: 0,
                    y: -48,
                    scale: 0.96,
                    filter: "blur(6px)",
                  }}
                  transition={{ duration: 0.55, ease: EASE_OUT }}
                  className="flex w-full origin-center scale-[0.56] justify-center px-6 sm:scale-[0.78] lg:scale-[0.9] xl:scale-100"
                >
                  {step.mock}
                </motion.div>
              </AnimatePresence>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

/** Reduced motion: the same five steps as a plain, fully visible list. */
function StaticFlow() {
  const { t } = useLocale();
  return (
    <div className="mx-auto max-w-[78rem] px-4 pb-20 sm:px-6 lg:px-8 lg:pb-28">
      <ol className="relative mt-4 grid gap-10 lg:grid-cols-5 lg:gap-6">
        <span
          aria-hidden
          className="bg-border absolute top-2 bottom-2 left-[1.125rem] w-px lg:top-[1.125rem] lg:right-0 lg:bottom-auto lg:left-0 lg:h-px lg:w-auto"
        />
        {STEPS.map((s, index) => (
          <li
            key={s.title}
            className="relative grid grid-cols-[2.25rem_1fr] content-start gap-x-5 lg:grid-cols-1 lg:gap-y-6"
          >
            <span
              aria-hidden
              className="bg-background text-foreground ring-border relative grid size-9 place-items-center rounded-full text-sm font-semibold tabular-nums ring-1"
            >
              {index + 1}
            </span>
            <div className="lg:pr-4">
              <h3 className="text-lg font-semibold tracking-tight">
                {t(s.title)}
              </h3>
              <p className="text-muted-foreground mt-2 text-[0.9375rem] leading-relaxed">
                {t(s.body)}
              </p>
              <p className="mt-4">
                <StateChip
                  label={t("landing.flow.state_label")}
                  state={t(s.state)}
                />
              </p>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}

/** The rental lifecycle: a pinned walkthrough, or a list for reduced motion. */
export function FlowSection() {
  const { t } = useLocale();
  const reduceMotion = useReducedMotion();

  return (
    <section
      id={LANDING_SECTIONS.product}
      aria-labelledby="landing-flow-title"
      className="scroll-mt-20"
    >
      <div className="mx-auto max-w-[78rem] px-4 pt-16 sm:px-6 lg:px-8">
        <Reveal>
          <SectionHeading
            id="landing-flow-title"
            title={t("landing.flow.title")}
            lead={t("landing.flow.lead")}
          />
        </Reveal>
      </div>
      {reduceMotion ? <StaticFlow /> : <PinnedFlow />}
    </section>
  );
}
