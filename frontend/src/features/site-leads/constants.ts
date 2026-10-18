import type { SiteLeadKind, SiteLeadStatus } from "@/features/site-leads/types";

export const SITE_LEADS_RESOURCE = "tenant.site_leads";

export const LEAD_KINDS: SiteLeadKind[] = [
  "contact",
  "corporate",
  "franchise",
  "newsletter",
];

export const LEAD_STATUSES: SiteLeadStatus[] = [
  "new",
  "in_progress",
  "done",
  "spam",
];

export const KIND_LABEL_KEYS: Record<SiteLeadKind, string> = {
  contact: "site_leads.kinds.contact",
  corporate: "site_leads.kinds.corporate",
  franchise: "site_leads.kinds.franchise",
  newsletter: "site_leads.kinds.newsletter",
};

export const STATUS_LABEL_KEYS: Record<SiteLeadStatus, string> = {
  new: "site_leads.statuses.new",
  in_progress: "site_leads.statuses.in_progress",
  done: "site_leads.statuses.done",
  spam: "site_leads.statuses.spam",
};

export const STATUS_TONES: Record<
  SiteLeadStatus,
  "default" | "success" | "warning" | "danger"
> = {
  new: "warning",
  in_progress: "default",
  done: "success",
  spam: "danger",
};

/** API limit of the staff note (characters). */
export const NOTE_MAX = 5000;
