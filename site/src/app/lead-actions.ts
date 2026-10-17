"use server";

import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { ApiError } from "@/lib/api";
import {
  LEAD_FLASH_COOKIE,
  LEAD_PATHS,
  apiLeadErrors,
  encodeLeadFlash,
  isLeadKind,
  leadFlags,
  leadRedirect,
  leadStatusOf,
  readLeadForm,
  type LeadKind,
  type LeadResult,
  type LeadValues,
} from "@/lib/leads";
import { bookingApi, getCtx } from "@/lib/server";

/**
 * Site lead forms → POST /v1/public/site/leads with the visitor's booking
 * session (the same session and origin as every other public API call),
 * then post / redirect / get back to the form. "Sent" is shown only after
 * the API accepted the submission (202).
 */
export async function submitLead(form: FormData) {
  const kind = form.get("kind");
  if (!isLeadKind(kind)) redirect("/");
  const { site, locale } = await getCtx();

  const finish = async (
    result: LeadResult,
    values?: LeadValues,
  ): Promise<never> => {
    const jar = await cookies();
    if (values && result.status !== "sent") {
      const flash = encodeLeadFlash(kind, values);
      if (flash) {
        jar.set(LEAD_FLASH_COOKIE, flash, {
          httpOnly: true,
          sameSite: "lax",
          secure: site.origin.startsWith("https:"),
          path: LEAD_PATHS[kind],
          maxAge: 5 * 60,
        });
      }
    } else {
      jar.delete({ name: LEAD_FLASH_COOKIE, path: LEAD_PATHS[kind] });
    }
    redirect(leadRedirect(kind, result));
  };

  const { input, values, errors } = readLeadForm(
    kind,
    (name) => form.get(name),
    locale,
  );
  if (site.preview) return finish({ status: "preview", errors: {} }, values);
  if (!leadFlags(site)[kind as LeadKind])
    return finish({ status: "closed", errors: {} }, values);
  if (Object.keys(errors).length)
    return finish({ status: "invalid", errors }, values);

  try {
    await bookingApi<{ accepted: boolean }>("/v1/public/site/leads", {
      method: "POST",
      body: input,
    });
  } catch (err) {
    if (!(err instanceof ApiError))
      return finish({ status: "failed", errors: {} }, values);
    const status = leadStatusOf(err.status, err.code);
    return finish(
      {
        status,
        errors: status === "invalid" ? apiLeadErrors(err.details) : {},
      },
      values,
    );
  }
  return finish({ status: "sent", errors: {} });
}
