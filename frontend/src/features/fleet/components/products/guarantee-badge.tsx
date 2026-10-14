"use client";

import { BadgeCheck, CarFront, Shuffle } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { BADGE_LABEL_KEYS } from "@/features/fleet/constants";
import type { GuaranteeBadge } from "@/features/fleet/types";
import { useLocale } from "@/providers/locale-provider";

const ICONS = {
  or_similar: Shuffle,
  model_guaranteed: BadgeCheck,
  vehicle_guaranteed: CarFront,
} as const;

/** "veya benzeri" / "model garantili" / "araç garantili" badge. */
export function GuaranteeBadgeChip({ badge }: { badge: GuaranteeBadge }) {
  const { t } = useLocale();
  const Icon = ICONS[badge];
  return (
    <Badge
      variant={badge === "or_similar" ? "outline" : "secondary"}
      className="gap-1"
    >
      <Icon className="size-3" aria-hidden />
      {t(BADGE_LABEL_KEYS[badge])}
    </Badge>
  );
}
