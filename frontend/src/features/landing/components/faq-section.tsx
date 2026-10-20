"use client";

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { useLocale } from "@/providers/locale-provider";

import { LANDING_SECTIONS } from "./landing-header";
import { Reveal } from "./motion-primitives";
import { SectionHeading } from "./section-heading";

const QUESTIONS = [
  { q: "landing.faq.q1", a: "landing.faq.a1" },
  { q: "landing.faq.q2", a: "landing.faq.a2" },
  { q: "landing.faq.q3", a: "landing.faq.a3" },
  { q: "landing.faq.q4", a: "landing.faq.a4" },
  { q: "landing.faq.q5", a: "landing.faq.a5" },
  { q: "landing.faq.q6", a: "landing.faq.a6" },
  { q: "landing.faq.q7", a: "landing.faq.a7" },
] as const;

export function FaqSection() {
  const { t } = useLocale();

  return (
    <section
      id={LANDING_SECTIONS.faq}
      aria-labelledby="landing-faq-title"
      className="scroll-mt-16 border-t"
    >
      <div className="mx-auto grid max-w-[78rem] grid-cols-1 gap-10 px-4 py-24 sm:px-6 lg:grid-cols-12 lg:gap-8 lg:px-8 lg:py-32">
        <div className="lg:sticky lg:top-28 lg:col-span-4 lg:self-start">
          <Reveal>
            <SectionHeading
              id="landing-faq-title"
              title={t("landing.faq.title")}
            />
          </Reveal>
        </div>
        <Reveal delay={0.1} className="lg:col-span-7 lg:col-start-6">
          <Accordion type="single" collapsible>
            {QUESTIONS.map((item) => (
              <AccordionItem key={item.q} value={item.q}>
                <AccordionTrigger className="py-5 text-base font-semibold sm:text-lg">
                  {t(item.q)}
                </AccordionTrigger>
                <AccordionContent className="text-muted-foreground max-w-[64ch] text-base leading-relaxed">
                  {t(item.a)}
                </AccordionContent>
              </AccordionItem>
            ))}
          </Accordion>
        </Reveal>
      </div>
    </section>
  );
}
