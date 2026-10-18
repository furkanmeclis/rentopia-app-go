"use client";

import { createNavAdornment } from "@/features/nav-engine";
import type { NavAdornment } from "@/features/nav-engine";
import { useNewSiteLeadsCount } from "@/features/site-leads/hooks/use-site-leads";
import { useLocale } from "@/providers/locale-provider";

/** Website enquiries waiting for a person (status `new`). */
function useSiteLeadsNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const count = useNewSiteLeadsCount();
  if (!count) return { badges: [] };
  return {
    badges: [{ kind: "count", value: count, variant: "warning" }],
    info: {
      title: t("site_leads.nav.info_title"),
      rows: [
        {
          label: t("site_leads.statuses.new"),
          value: count,
          tone: "warning",
        },
      ],
    },
  };
}

export const SiteLeadsNavAdornment = createNavAdornment(
  useSiteLeadsNavAdornment,
);
