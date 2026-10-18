import { ArrowRight, Mail } from "lucide-react";

import { submitLead } from "@/app/lead-actions";
import {
  LeadConsents,
  LeadHidden,
  LeadStatus,
  leadErrorText,
  leadValues,
} from "@/components/site/lead-form";
import { LEAD_LIMITS, leadAnchor, type LeadResult } from "@/lib/leads";
import type { Ctx } from "@/lib/server";

/**
 * The newsletter band (studio "club" band): e-mail, KVKK and marketing
 * consent, a hidden bot trap, posted without JavaScript to the lead action.
 * Rendered only while the published `leads.newsletter` flag is on.
 */
export async function Newsletter({
  ctx,
  result,
}: {
  ctx: Ctx;
  result: LeadResult | null;
}) {
  const { t } = ctx;
  const values = await leadValues("newsletter", result);
  const error = leadErrorText(ctx, "email", result?.errors.email);
  return (
    <section className="tw-newsletter" aria-labelledby="newsletter-title">
      <div className="rt-container tw-newsletter-inner">
        <div>
          <span className="tw-mini-label">{t("tw.newsletter.eyebrow")}</span>
          <h2 id="newsletter-title">{t("tw.newsletter.title")}</h2>
          <p>{t("tw.newsletter.text")}</p>
        </div>
        <form
          id={leadAnchor("newsletter")}
          action={submitLead}
          className="tw-newsletter-form"
          aria-labelledby="newsletter-title"
        >
          <LeadStatus
            ctx={ctx}
            kind="newsletter"
            result={result}
            className="tw-newsletter-feedback"
          />
          <LeadHidden ctx={ctx} kind="newsletter" />
          <label htmlFor="newsletter-email" className="rt-sr-only">
            {t("lead.field.email")}
          </label>
          <div className="tw-newsletter-input">
            <Mail aria-hidden size={21} />
            <input
              id="newsletter-email"
              type="email"
              name="email"
              required
              maxLength={LEAD_LIMITS.email}
              autoComplete="email"
              spellCheck={false}
              placeholder={t("tw.newsletter.placeholder")}
              defaultValue={values.email ?? ""}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? "newsletter-email-error" : undefined}
            />
            <button type="submit">
              {t("lead.submit.newsletter")}
              <ArrowRight aria-hidden size={18} />
            </button>
          </div>
          {error ? (
            <p
              id="newsletter-email-error"
              className="tw-newsletter-error"
              role="alert"
            >
              {error}
            </p>
          ) : null}
          <LeadConsents
            ctx={ctx}
            kind="newsletter"
            result={result}
            values={values}
            className="tw-newsletter-consents"
          />
        </form>
      </div>
    </section>
  );
}
