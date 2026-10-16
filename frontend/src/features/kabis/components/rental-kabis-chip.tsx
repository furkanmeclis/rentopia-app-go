"use client";

import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  KABIS_OFFICIAL,
  KABIS_STATUS_LABEL_KEYS,
} from "@/features/kabis/constants";
import { useKabisList } from "@/features/kabis/hooks/use-kabis";
import type { KabisNotification } from "@/features/kabis/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const TONE_VARIANTS = {
  success: "success",
  warning: "warning",
  danger: "danger",
} as const;

function toneOf(n: KabisNotification): keyof typeof TONE_VARIANTS {
  if (KABIS_OFFICIAL.includes(n.status)) return "success";
  if (n.needs_action) return "danger";
  return "warning";
}

/**
 * KABİS state of a rental for its detail header: the start (its latest
 * correction when there is one) and the end, each as its own chip linking
 * to the KABİS screen filtered to the rental. Nothing before the handover.
 */
export function RentalKabisChip({ rentalUuid }: { rentalUuid: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.kabis.read);
  const query = useKabisList(
    { limit: 20, offset: 0, rental: rentalUuid, sort: "created_at" },
    allowed,
  );
  if (!allowed || !query.data?.items.length) return null;
  const items = query.data.items;
  const start = [...items]
    .filter((n) => n.event !== "rental_end")
    .sort((a, b) => a.created_at.localeCompare(b.created_at))
    .pop();
  const end = items.find((n) => n.event === "rental_end");
  const href = `${routes.tenant.kabis.root(slug)}?rental=${rentalUuid}`;
  return (
    <>
      {[
        { n: start, label: "kabis.chip.start" },
        { n: end, label: "kabis.chip.end" },
      ].map(({ n, label }) =>
        n ? (
          <Link key={label} href={href} className="inline-flex">
            <Badge
              variant={TONE_VARIANTS[toneOf(n)]}
              className="font-normal whitespace-nowrap"
            >
              {t(label, { status: t(KABIS_STATUS_LABEL_KEYS[n.status]) })}
            </Badge>
          </Link>
        ) : null,
      )}
    </>
  );
}
