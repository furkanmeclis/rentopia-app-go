"use client";

import { createNavAdornment } from "@/features/nav-engine";
import type { NavAdornment } from "@/features/nav-engine";
import {
  KABIS_NEEDS_ACTION,
  KABIS_STATUS_LABEL_KEYS,
} from "@/features/kabis/constants";
import { useKabisNeedsAction } from "@/features/kabis/hooks/use-kabis";
import { useLocale } from "@/providers/locale-provider";

/** KABİS item: notifications that need a person, split by state. */
function useKabisNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const facets = useKabisNeedsAction();
  if (!facets) return { badges: [] };
  const rows = KABIS_NEEDS_ACTION.map((s) => ({
    label: t(KABIS_STATUS_LABEL_KEYS[s]),
    value: facets.statuses[s] ?? 0,
    tone:
      s === "waiting_dependency" ? ("warning" as const) : ("danger" as const),
  })).filter((r) => r.value > 0);
  return {
    badges: [{ kind: "count", value: facets.needs_action, variant: "danger" }],
    info:
      facets.needs_action > 0
        ? { title: t("kabis.nav.info_title"), rows }
        : null,
  };
}

export const KabisNavAdornment = createNavAdornment(useKabisNavAdornment);
