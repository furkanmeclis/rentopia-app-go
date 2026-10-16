"use client";

import { createNavAdornment } from "@/features/nav-engine";
import type { NavAdornment } from "@/features/nav-engine";
import { useChannelNeedsAction } from "@/features/channels/hooks/use-channels";
import { useLocale } from "@/providers/locale-provider";

/** Channel manager item: failed messages waiting in the error queue. */
function useChannelsNavAdornment(): NavAdornment {
  const { t } = useLocale();
  const facets = useChannelNeedsAction();
  if (!facets || facets.needs_action === 0) return { badges: [] };
  return {
    badges: [{ kind: "count", value: facets.needs_action, variant: "danger" }],
    info: {
      title: t("channels.nav.info_title"),
      rows: [
        {
          label: t("channels.message_statuses.failed"),
          value: facets.needs_action,
          tone: "danger" as const,
        },
      ],
    },
  };
}

export const ChannelsNavAdornment = createNavAdornment(useChannelsNavAdornment);
