"use client";

import { useRef } from "react";
import {
  motion,
  useAnimationFrame,
  useMotionValue,
  useReducedMotion,
  useTransform,
} from "motion/react";

import { useLocale } from "@/providers/locale-provider";

import { Reveal } from "./motion-primitives";
import { SectionHeading } from "./section-heading";

const GROUPS = [
  {
    title: "landing.connect.group_payments",
    items: ["landing.connect.iyzico"],
  },
  {
    title: "landing.connect.group_messages",
    items: [
      "landing.connect.whatsapp",
      "landing.connect.sms",
      "landing.connect.email",
    ],
  },
  {
    title: "landing.connect.group_channels",
    items: ["landing.connect.yolcu360", "landing.connect.obilet"],
  },
  {
    title: "landing.connect.group_official",
    items: [
      "landing.connect.kabis",
      "landing.connect.tolls",
      "landing.connect.einvoice",
    ],
  },
  {
    title: "landing.connect.group_developers",
    items: ["landing.connect.api", "landing.connect.webhooks"],
  },
] as const;

type Chip = { group: string; item: string };

const CHIPS: Chip[] = GROUPS.flatMap((g) =>
  g.items.map((item) => ({ group: g.title, item })),
);
const ROW_A = CHIPS.filter((_, i) => i % 2 === 0);
const ROW_B = CHIPS.filter((_, i) => i % 2 === 1);

function ChipView({ chip }: { chip: Chip }) {
  const { t } = useLocale();
  return (
    <span className="bg-card hover:border-primary/40 inline-flex shrink-0 items-center gap-2.5 rounded-full border py-2 pr-4 pl-2 text-sm font-medium whitespace-nowrap shadow-[0_1px_0_rgba(20,21,39,0.04)] transition-colors">
      <span className="bg-muted text-muted-foreground rounded-full px-2.5 py-0.5 text-xs">
        {t(chip.group)}
      </span>
      {t(chip.item)}
    </span>
  );
}

/**
 * One endless row of chips. Speed is in px per second; the row is drawn
 * twice so the loop is seamless, and it pauses while the pointer is over it.
 */
function MarqueeRow({ chips, speed }: { chips: Chip[]; speed: number }) {
  const offset = useMotionValue(0);
  const paused = useRef(false);
  const track = useRef<HTMLDivElement>(null);
  const x = useTransform(offset, (v) => `${v}px`);

  useAnimationFrame((_, delta) => {
    const el = track.current;
    if (!el || paused.current) return;
    const half = el.scrollWidth / 2;
    if (!half) return;
    let next = offset.get() - (speed * delta) / 1000;
    if (speed > 0 && next <= -half) next += half;
    if (speed < 0 && next >= 0) next -= half;
    offset.set(next);
  });

  return (
    <div
      className="overflow-hidden [mask-image:linear-gradient(90deg,transparent,black_8%,black_92%,transparent)]"
      onPointerEnter={() => (paused.current = true)}
      onPointerLeave={() => (paused.current = false)}
    >
      <motion.div ref={track} style={{ x }} className="flex w-max gap-3 py-1.5">
        {/* Two copies of a doubled set: wide screens never see the seam. */}
        {[...chips, ...chips, ...chips, ...chips].map((chip, i) => (
          <ChipView key={`${chip.item}-${i}`} chip={chip} />
        ))}
      </motion.div>
    </div>
  );
}

/** Connections, as two slow marquee rows (a wrapped list for reduced motion). */
export function ConnectSection() {
  const { t } = useLocale();
  const reduceMotion = useReducedMotion();

  return (
    <section aria-labelledby="landing-connect-title" className="border-t">
      <div className="mx-auto max-w-[78rem] px-4 pt-24 sm:px-6 lg:px-8 lg:pt-32">
        <Reveal className="grid grid-cols-1 gap-6 lg:grid-cols-12 lg:gap-8">
          <SectionHeading
            id="landing-connect-title"
            title={t("landing.connect.title")}
            className="lg:col-span-6"
          />
          <p className="text-muted-foreground max-w-[56ch] text-lg leading-relaxed text-pretty lg:col-span-5 lg:col-start-8 lg:self-end">
            {t("landing.connect.body")}
          </p>
        </Reveal>
      </div>

      {/* Accessible version of the chips; the marquee below is decorative. */}
      <ul className="sr-only" aria-label={t("landing.connect.marquee_label")}>
        {CHIPS.map((chip) => (
          <li key={chip.item}>
            {t(chip.group)}: {t(chip.item)}
          </li>
        ))}
      </ul>

      <Reveal y={16} className="mt-14 lg:mt-16" aria-hidden>
        {reduceMotion ? (
          <div className="mx-auto flex max-w-[78rem] flex-wrap gap-3 px-4 sm:px-6 lg:px-8">
            {CHIPS.map((chip) => (
              <ChipView key={chip.item} chip={chip} />
            ))}
          </div>
        ) : (
          <div className="grid gap-3">
            <MarqueeRow chips={ROW_A} speed={36} />
            <MarqueeRow chips={ROW_B} speed={-30} />
          </div>
        )}
      </Reveal>

      <div className="mx-auto max-w-[78rem] px-4 pt-10 pb-24 sm:px-6 lg:px-8 lg:pb-32">
        <p className="text-muted-foreground max-w-[72ch] text-sm leading-relaxed">
          {t("landing.connect.note")}
        </p>
      </div>
    </section>
  );
}
