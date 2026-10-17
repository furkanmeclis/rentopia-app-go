import { ArrowRight } from "lucide-react";
import { cookies } from "next/headers";

import { submitLead } from "@/app/lead-actions";
import { Alert } from "@/components/ui/card";
import { Checkbox, Field, Input, Textarea } from "@/components/ui/form";
import {
  FLEET_SIZE_MAX,
  LEAD_FLASH_COOKIE,
  LEAD_FORM_FIELDS,
  LEAD_LIMITS,
  decodeLeadFlash,
  leadAnchor,
  type LeadErrorCode,
  type LeadField,
  type LeadKind,
  type LeadResult,
  type LeadValues,
} from "@/lib/leads";
import type { Ctx } from "@/lib/server";
import type { MessageKey } from "@/messages/tr";

/**
 * Site lead forms (theme-neutral markup, `rt-*` classes): a plain POST to
 * the `submitLead` server action, so they work without JavaScript. Native
 * validation (required, type=email, maxlength) catches most mistakes in
 * the browser; the action checks again and the API decides. Outcomes come
 * back through the redirect query (`lib/leads.ts`).
 */

/** The message of a field error. */
export function leadErrorText(
  ctx: Ctx,
  field: LeadField,
  code: LeadErrorCode | undefined,
): string | undefined {
  if (!code) return undefined;
  const { t } = ctx;
  if (code === "too_long") return t("lead.error.too_long");
  if (code === "required")
    return field.startsWith("consent_")
      ? t("lead.error.consent")
      : t("lead.error.required");
  if (field === "email") return t("lead.error.email");
  if (field === "phone") return t("lead.error.phone");
  if (field === "fleet_size") return t("lead.error.fleet_size");
  return t("lead.error.invalid");
}

/** The success / failure banner of a submission (null when none). */
export function LeadStatus({
  ctx,
  kind,
  result,
  className,
}: {
  ctx: Ctx;
  kind: LeadKind;
  result: LeadResult | null;
  className?: string;
}) {
  if (!result) return null;
  const sent = result.status === "sent";
  return (
    <Alert tone={sent ? "success" : "error"} className={className}>
      {sent
        ? ctx.t(`lead.sent.${kind}` as MessageKey)
        : ctx.t(`lead.status.${result.status}` as MessageKey)}
    </Alert>
  );
}

/**
 * What the visitor typed, after a failed submission (an httpOnly cookie
 * scoped to the form's page; empty once the API accepted it).
 */
export async function leadValues(
  kind: LeadKind,
  result: LeadResult | null,
): Promise<LeadValues> {
  if (!result || result.status === "sent") return {};
  return decodeLeadFlash((await cookies()).get(LEAD_FLASH_COOKIE)?.value, kind);
}

/** Hidden fields: the form kind and the bot trap (left empty by people). */
export function LeadHidden({ ctx, kind }: { ctx: Ctx; kind: LeadKind }) {
  return (
    <>
      <input type="hidden" name="kind" value={kind} />
      <div className="rt-hp" aria-hidden>
        <label>
          {ctx.t("lead.optional")}
          <input
            type="text"
            name="website"
            tabIndex={-1}
            autoComplete="off"
            defaultValue=""
          />
        </label>
      </div>
    </>
  );
}

/** Consent checkboxes: KVKK always, marketing (required for the bulletin). */
export function LeadConsents({
  ctx,
  kind,
  result,
  values,
  className,
}: {
  ctx: Ctx;
  kind: LeadKind;
  result: LeadResult | null;
  values: LeadValues;
  className?: string;
}) {
  const { t } = ctx;
  const required = LEAD_FORM_FIELDS[kind].required;
  const box = (field: "consent_kvkk" | "consent_marketing") => {
    const id = `${kind}-${field}`;
    const error = leadErrorText(ctx, field, result?.errors[field]);
    const isRequired = required.includes(field);
    return (
      <div className="rt-consent">
        <label htmlFor={id} className="rt-check">
          <Checkbox
            id={id}
            name={field}
            required={isRequired}
            defaultChecked={
              field === "consent_marketing" && values.consent_marketing === "on"
            }
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? `${id}-error` : undefined}
          />
          <span>
            {field === "consent_kvkk"
              ? t("lead.consent_kvkk")
              : t("lead.consent_marketing")}
            {isRequired ? <span aria-hidden> *</span> : null}
            {field === "consent_kvkk" ? (
              <>
                {" "}
                <a href="/privacy" target="_blank" rel="noopener noreferrer">
                  {t("lead.privacy_link")}
                  <span className="rt-sr-only"> {t("lead.new_tab")}</span>
                </a>
              </>
            ) : null}
          </span>
        </label>
        {error ? (
          <p id={`${id}-error`} className="rt-field-error" role="alert">
            {error}
          </p>
        ) : null}
      </div>
    );
  };
  return (
    <div className={className ?? "rt-consents"}>
      {box("consent_kvkk")}
      {box("consent_marketing")}
    </div>
  );
}

const AUTOCOMPLETE: Partial<Record<LeadField, string>> = {
  name: "name",
  email: "email",
  phone: "tel",
  company: "organization",
  city: "address-level2",
};

/** One text field of a page form. */
function LeadInput({
  ctx,
  kind,
  field,
  result,
  values,
}: {
  ctx: Ctx;
  kind: LeadKind;
  field: LeadField;
  result: LeadResult | null;
  values: LeadValues;
}) {
  const { t } = ctx;
  const id = `${kind}-${field}`;
  const required = LEAD_FORM_FIELDS[kind].required.includes(field);
  const label = `${t(`lead.field.${field}` as MessageKey)}${
    required ? " *" : ` (${t("lead.optional")})`
  }`;
  const common = {
    name: field,
    required,
    defaultValue: values[field] ?? "",
    maxLength: LEAD_LIMITS[field],
  };
  const control =
    field === "message" ? (
      <Textarea {...common} rows={5} />
    ) : field === "fleet_size" ? (
      <Input
        name={field}
        type="number"
        inputMode="numeric"
        min={0}
        max={FLEET_SIZE_MAX}
        step={1}
        required={required}
        defaultValue={values[field] ?? ""}
      />
    ) : (
      <Input
        {...common}
        type={field === "email" ? "email" : field === "phone" ? "tel" : "text"}
        autoComplete={AUTOCOMPLETE[field]}
        spellCheck={field === "email" ? false : undefined}
      />
    );
  return (
    <Field
      id={id}
      label={label}
      error={leadErrorText(ctx, field, result?.errors[field])}
      className={field === "message" ? "rt-span-all" : undefined}
    >
      {control}
    </Field>
  );
}

/**
 * A page form (contact, corporate, franchise): title, fields in two
 * columns, consents and the submit button.
 */
export async function LeadForm({
  ctx,
  kind,
  result,
  title,
  className,
}: {
  ctx: Ctx;
  kind: Exclude<LeadKind, "newsletter">;
  result: LeadResult | null;
  title: string;
  className?: string;
}) {
  const { t } = ctx;
  const values = await leadValues(kind, result);
  const titleId = `${leadAnchor(kind)}-title`;
  return (
    <form
      id={leadAnchor(kind)}
      action={submitLead}
      className={className ?? "rt-lead-form"}
      aria-labelledby={titleId}
    >
      <h2 id={titleId}>{title}</h2>
      <LeadStatus ctx={ctx} kind={kind} result={result} />
      <LeadHidden ctx={ctx} kind={kind} />
      <p className="rt-field-hint">{t("lead.required_note")}</p>
      <div className="rt-lead-grid">
        {LEAD_FORM_FIELDS[kind].fields.map((field) => (
          <LeadInput
            key={field}
            ctx={ctx}
            kind={kind}
            field={field}
            result={result}
            values={values}
          />
        ))}
      </div>
      <LeadConsents ctx={ctx} kind={kind} result={result} values={values} />
      <button type="submit" className="rt-button rt-button--primary">
        {t(`lead.submit.${kind}` as MessageKey)}
        <ArrowRight aria-hidden size={17} />
      </button>
    </form>
  );
}
