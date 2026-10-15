import { z } from "zod";

import { IDENTITY_TYPES_BY_CUSTOMER } from "@/features/customers/constants";
import { isValidPhone, splitTags } from "@/features/customers/lib/format";
import type {
  Customer,
  CustomerCreateRequest,
  CustomerType,
  IdentityType,
} from "@/features/customers/types";
import {
  isValidTCKN,
  isValidVKN,
} from "@/features/tenant-settings/lib/tax-number";

type Translate = (key: string) => string;

/** `none` in the identity select clears the identity on save. */
export const NO_IDENTITY = "none";

export type CustomerFormValues = {
  customer_type: CustomerType;
  first_name: string;
  last_name: string;
  company_title: string;
  identity_type: IdentityType | typeof NO_IDENTITY;
  /** Write-only: empty on edit keeps the stored number. */
  identity_number: string;
  nationality: string;
  tax_office: string;
  /** Write-only: empty on edit keeps the stored value. */
  phone: string;
  email: string;
  billing_address: string;
  billing_city: string;
  billing_district: string;
  billing_postal_code: string;
  billing_country: string;
  e_invoice: boolean;
  tags: string;
  note: string;
};

/**
 * Checks an identity number by kind (TCKN / VKN checksums mirror the API;
 * a passport has 5–20 letters or digits). Returns an i18n key or null.
 */
export function identityError(kind: string, raw: string): string | null {
  const digits = raw.replace(/\s/g, "");
  if (kind === "tckn") {
    if (!/^\d{11}$/.test(digits)) return "customers.validation.tckn_length";
    return isValidTCKN(digits) ? null : "customers.validation.tckn_checksum";
  }
  if (kind === "vkn") {
    if (!/^\d{10}$/.test(digits)) return "customers.validation.vkn_length";
    return isValidVKN(digits) ? null : "customers.validation.vkn_checksum";
  }
  if (kind === "passport") {
    const doc = raw.replace(/[^0-9A-Za-z]/g, "");
    return doc.length >= 5 && doc.length <= 20
      ? null
      : "customers.validation.passport";
  }
  return null;
}

export function createCustomerFormSchema(
  t: Translate,
  mode: "create" | "edit",
  hasStoredIdentity: boolean,
) {
  return z
    .object({
      customer_type: z.enum(["individual", "corporate"]),
      first_name: z.string().max(128),
      last_name: z.string().max(128),
      company_title: z.string().max(255),
      identity_type: z.enum(["none", "tckn", "vkn", "passport"]),
      identity_number: z.string().max(32),
      nationality: z.string(),
      tax_office: z.string().max(128),
      phone: z.string().max(32),
      email: z.union([
        z.literal(""),
        z.email(t("customers.validation.email")).max(255),
      ]),
      billing_address: z.string().max(1000),
      billing_city: z.string().max(128),
      billing_district: z.string().max(128),
      billing_postal_code: z.string().max(16),
      billing_country: z.string(),
      e_invoice: z.boolean(),
      tags: z.string().max(1000),
      note: z.string().max(5000),
    })
    .superRefine((v, ctx) => {
      const issue = (path: string, key: string) =>
        ctx.addIssue({ code: "custom", path: [path], message: t(key) });
      if (v.customer_type === "individual") {
        if (!v.first_name.trim())
          issue("first_name", "customers.validation.required");
        if (!v.last_name.trim())
          issue("last_name", "customers.validation.required");
      } else if (!v.company_title.trim()) {
        issue("company_title", "customers.validation.required");
      }
      if (v.identity_type !== NO_IDENTITY) {
        const allowed = IDENTITY_TYPES_BY_CUSTOMER[v.customer_type];
        if (!allowed.includes(v.identity_type)) {
          issue("identity_type", "customers.validation.identity_type");
        }
        const number = v.identity_number.trim();
        // On edit an empty number keeps the stored one (same type only).
        const keep = mode === "edit" && hasStoredIdentity && !number;
        if (!number && !keep) {
          issue("identity_number", "customers.validation.required");
        } else if (number) {
          const err = identityError(v.identity_type, number);
          if (err) issue("identity_number", err);
        }
        if (v.identity_type === "passport" && !v.nationality) {
          issue("nationality", "customers.validation.nationality_passport");
        }
      }
      if (!isValidPhone(v.phone)) issue("phone", "customers.validation.phone");
      if (splitTags(v.tags).some((tag) => tag.length > 40)) {
        issue("tags", "customers.validation.tag_length");
      }
    });
}

export function customerFormDefaults(customer?: Customer): CustomerFormValues {
  return {
    customer_type: customer?.customer_type ?? "individual",
    first_name: customer?.first_name ?? "",
    last_name: customer?.last_name ?? "",
    company_title: customer?.company_title ?? "",
    identity_type: customer?.identity_type ?? (customer ? NO_IDENTITY : "tckn"),
    identity_number: "",
    nationality: customer?.nationality ?? (customer ? "" : "TR"),
    tax_office: customer?.tax_office ?? "",
    phone: "",
    email: "",
    billing_address: customer?.billing.address ?? "",
    billing_city: customer?.billing.city ?? "",
    billing_district: customer?.billing.district ?? "",
    billing_postal_code: customer?.billing.postal_code ?? "",
    billing_country: customer?.billing.country ?? (customer ? "" : "TR"),
    e_invoice: customer?.billing.e_invoice ?? false,
    tags: (customer?.tags ?? []).join(", "),
    note: customer?.note ?? "",
  };
}

/**
 * Form → request. Sensitive values are only sent when typed: on edit an
 * empty identity number / phone / e-mail keeps what is stored; choosing
 * "no identity" clears it.
 */
export function customerFormToRequest(
  v: CustomerFormValues,
  mode: "create" | "edit",
  stored?: Customer,
): CustomerCreateRequest {
  const corporate = v.customer_type === "corporate";
  const body: CustomerCreateRequest = {
    customer_type: v.customer_type,
    first_name: corporate ? "" : v.first_name.trim(),
    last_name: corporate ? "" : v.last_name.trim(),
    company_title: corporate ? v.company_title.trim() : "",
    nationality: v.nationality || null,
    tax_office: v.tax_office.trim(),
    billing_address: v.billing_address.trim(),
    billing_city: v.billing_city.trim(),
    billing_district: v.billing_district.trim(),
    billing_postal_code: v.billing_postal_code.trim(),
    billing_country: v.billing_country || null,
    e_invoice: v.e_invoice,
    tags: splitTags(v.tags),
    note: v.note.trim(),
  };
  if (v.identity_type === NO_IDENTITY) {
    if (mode === "edit" && stored?.identity_type) body.identity_number = "";
  } else if (v.identity_number.trim()) {
    body.identity_type = v.identity_type;
    body.identity_number = v.identity_number.trim();
  } else if (mode === "edit" && stored?.identity_type !== v.identity_type) {
    body.identity_type = v.identity_type;
  }
  if (v.phone.trim() || mode === "create") body.phone = v.phone.trim();
  if (v.email.trim() || mode === "create") body.email = v.email.trim();
  return body;
}
