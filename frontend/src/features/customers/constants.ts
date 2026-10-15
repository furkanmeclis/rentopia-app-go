import type {
  ConsentChannel,
  ConsentSource,
  CustomerDocumentType,
  CustomerType,
  IdentityType,
} from "@/features/customers/types";

/** Resource slug shared by io / bulk / activity / live lists. */
export const CUSTOMERS_RESOURCE = "tenant.customers" as const;

export const CUSTOMER_TYPES: CustomerType[] = ["individual", "corporate"];

export const CUSTOMER_TYPE_LABEL_KEYS: Record<CustomerType, string> = {
  individual: "customers.types.individual",
  corporate: "customers.types.corporate",
};

/** Identity kinds per customer type (same rule as the API). */
export const IDENTITY_TYPES_BY_CUSTOMER: Record<CustomerType, IdentityType[]> =
  {
    individual: ["tckn", "passport"],
    corporate: ["vkn", "tckn"],
  };

export const IDENTITY_TYPE_LABEL_KEYS: Record<IdentityType, string> = {
  tckn: "customers.identity_types.tckn",
  vkn: "customers.identity_types.vkn",
  passport: "customers.identity_types.passport",
};

export const CONSENT_CHANNELS: ConsentChannel[] = [
  "sms",
  "email",
  "call",
  "whatsapp",
];

export const CONSENT_CHANNEL_LABEL_KEYS: Record<ConsentChannel, string> = {
  sms: "customers.consent_channels.sms",
  email: "customers.consent_channels.email",
  call: "customers.consent_channels.call",
  whatsapp: "customers.consent_channels.whatsapp",
};

/** Consent facet: the channels plus "no consent". */
export const CONSENT_FACET_LABEL_KEYS: Record<string, string> = {
  ...CONSENT_CHANNEL_LABEL_KEYS,
  none: "customers.consent_channels.none",
};

export const CONSENT_SOURCES: ConsentSource[] = [
  "counter",
  "web",
  "phone",
  "import",
  "other",
];

export const CONSENT_SOURCE_LABEL_KEYS: Record<ConsentSource, string> = {
  counter: "customers.consent_sources.counter",
  web: "customers.consent_sources.web",
  phone: "customers.consent_sources.phone",
  import: "customers.consent_sources.import",
  other: "customers.consent_sources.other",
};

export const DOCUMENT_TYPES: CustomerDocumentType[] = [
  "identity",
  "passport",
  "driving_licence",
  "tax_certificate",
  "signature_circular",
  "contract",
  "other",
];

export const DOCUMENT_TYPE_LABEL_KEYS: Record<CustomerDocumentType, string> = {
  identity: "customers.document_types.identity",
  passport: "customers.document_types.passport",
  driving_licence: "customers.document_types.driving_licence",
  tax_certificate: "customers.document_types.tax_certificate",
  signature_circular: "customers.document_types.signature_circular",
  contract: "customers.document_types.contract",
  other: "customers.document_types.other",
};

/** Eligibility reasons of the driver condition check. */
export const ELIGIBILITY_REASON_LABEL_KEYS: Record<string, string> = {
  birth_date_missing: "customers.eligibility.reasons.birth_date_missing",
  under_min_age: "customers.eligibility.reasons.under_min_age",
  licence_missing: "customers.eligibility.reasons.licence_missing",
  licence_issue_date_missing:
    "customers.eligibility.reasons.licence_issue_date_missing",
  under_min_licence_years:
    "customers.eligibility.reasons.under_min_licence_years",
  licence_expired: "customers.eligibility.reasons.licence_expired",
};

export const DOCUMENT_FILE_ACCEPT =
  "application/pdf,image/jpeg,image/png,image/webp";
export const MAX_DOCUMENT_FILE_BYTES = 10 * 1024 * 1024;

/** Common nationalities first; labels come from `Intl.DisplayNames`. */
export const COUNTRY_CODES = [
  "TR",
  "DE",
  "GB",
  "NL",
  "FR",
  "BE",
  "AT",
  "CH",
  "IT",
  "ES",
  "SE",
  "NO",
  "DK",
  "FI",
  "PL",
  "RO",
  "BG",
  "GR",
  "CY",
  "RU",
  "UA",
  "BY",
  "GE",
  "AZ",
  "AM",
  "KZ",
  "UZ",
  "TM",
  "KG",
  "IR",
  "IQ",
  "SY",
  "LB",
  "JO",
  "IL",
  "SA",
  "AE",
  "QA",
  "KW",
  "BH",
  "OM",
  "EG",
  "LY",
  "TN",
  "DZ",
  "MA",
  "US",
  "CA",
  "BR",
  "CN",
  "JP",
  "KR",
  "IN",
  "PK",
  "AF",
  "AU",
] as const;
