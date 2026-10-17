import { ArrowUpRight, ChevronDown } from "lucide-react";
import Link from "next/link";

import { Paragraphs } from "@/components/site/text";
import type { Ctx } from "@/lib/server";

import { Eyebrow } from "../parts";

/** Questions as native disclosure widgets (no JavaScript). */
export function FaqList({
  items,
  className,
}: {
  items: { question: string; answer: string }[];
  className?: string;
}) {
  return (
    <div className={className ?? "tw-faq-items"}>
      {items.map((f, i) => (
        <details key={i} className="tw-faq-item" open={i === 0}>
          <summary>
            <span className="tw-faq-number" aria-hidden>
              {String(i + 1).padStart(2, "0")}
            </span>
            <span className="tw-faq-question">{f.question}</span>
            <ChevronDown aria-hidden size={18} />
          </summary>
          <Paragraphs text={f.answer} className="tw-faq-answer" />
        </details>
      ))}
    </div>
  );
}

/** Home FAQ: the first six questions and a link to all. */
export function Faq({ ctx }: { ctx: Ctx }) {
  const { t, content } = ctx;
  if (!content.faq.length) return null;
  return (
    <section className="tw-section" aria-labelledby="faq-title">
      <div className="rt-container tw-faq-grid">
        <div>
          <Eyebrow>{t("tw.faq.eyebrow")}</Eyebrow>
          <h2 id="faq-title">{t("tw.faq.title")}</h2>
          <p className="tw-body-copy">{t("tw.faq.lead")}</p>
          <Link href="/faq" className="tw-text-button">
            {t("tw.faq.all")}
            <ArrowUpRight aria-hidden size={18} />
          </Link>
        </div>
        <FaqList items={content.faq.slice(0, 6)} />
      </div>
    </section>
  );
}
