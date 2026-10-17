import type { Ctx } from "@/lib/server";

import { ADVANTAGE_ICONS, SectionHeading } from "../parts";

/** The organization's own advantages (config v2); nothing when none. */
export function Advantages({ ctx }: { ctx: Ctx }) {
  const items = (ctx.content.advantages ?? []).filter(
    (a) => a && (a.title || a.text),
  );
  if (!items.length) return null;
  const { t } = ctx;
  return (
    <section className="tw-section tw-why" aria-labelledby="why-title">
      <div className="rt-container">
        <SectionHeading
          id="why-title"
          eyebrow={t("tw.why.eyebrow")}
          title={t("tw.why.title")}
          lead={t("tw.why.lead")}
        />
        <ul className="tw-why-grid">
          {items.map((a, i) => {
            const Icon = ADVANTAGE_ICONS[a.icon] ?? ADVANTAGE_ICONS.check;
            return (
              <li key={i}>
                <span className="tw-feature-icon" aria-hidden>
                  <Icon size={27} strokeWidth={1.5} />
                </span>
                <span className="tw-feature-number" aria-hidden>
                  {String(i + 1).padStart(2, "0")}
                </span>
                {a.title ? <h3>{a.title}</h3> : null}
                {a.text ? <p>{a.text}</p> : null}
              </li>
            );
          })}
        </ul>
      </div>
    </section>
  );
}
