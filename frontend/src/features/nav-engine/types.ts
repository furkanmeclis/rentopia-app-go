import type { LucideIcon } from "lucide-react";
import type { ComponentType, ReactNode } from "react";

import type { FeatureKey } from "@/config/features";

export type NavBadgeVariant =
  "default" | "secondary" | "success" | "warning" | "danger" | "outline";

export type NavCountBadge = {
  kind: "count";
  value: number;
  variant?: NavBadgeVariant;
  max?: number;
  hiddenWhenZero?: boolean;
};

export type NavLabelBadge = {
  kind: "label";
  text: string;
  variant?: NavBadgeVariant;
};

export type NavDotBadge = {
  kind: "dot";
  variant?: NavBadgeVariant;
};

export type NavCustomBadge = {
  kind: "custom";
  id: string;
  render: () => ReactNode;
};

export type NavBadge =
  NavCountBadge | NavLabelBadge | NavDotBadge | NavCustomBadge;

export type NavInfoRow = {
  label: string;
  value: ReactNode;
  tone?: NavBadgeVariant;
};

export type NavInfo = {
  title?: string;
  description?: string;
  rows?: NavInfoRow[];
  /** Replaces the default title/rows panel when set. */
  render?: () => ReactNode;
};

export type NavAdornment = {
  badges?: NavBadge[];
  info?: NavInfo | null;
};

export type NavAdornmentComponent = ComponentType<{
  children: (adornment: NavAdornment) => ReactNode;
}>;

export type NavItemDef = {
  id: string;
  titleKey: string;
  href: string;
  icon: LucideIcon;
  permission?: string | string[];
  anyPermission?: string[];
  /**
   * Plan feature (TEC-674) the item needs; several = any of them. Hidden
   * when the organization's plan lacks it (the API refuses it too).
   */
  feature?: FeatureKey | FeatureKey[];
  /**
   * Without the feature: `hide` (default) removes the item; `readonly`
   * keeps it with an "not in plan" label, for screens whose history stays
   * readable after a downgrade.
   */
  featureOff?: "hide" | "readonly";
  soon?: boolean;
  /** Command palette icon key (see resolveSearchIcon). */
  searchIcon?: string;
  /** Static badges merged after the live adornment. */
  badges?: NavBadge[];
  info?: NavInfo;
  Adornment?: NavAdornmentComponent;
};

export type NavGroupDef = {
  id: string;
  labelKey: string;
  /** Icon used as the collapsed (icon-mode) accordion trigger. */
  icon?: LucideIcon;
  defaultOpen?: boolean;
  collapsible?: boolean;
  permission?: string | string[];
  anyPermission?: string[];
  items: NavItemDef[];
};

export type ResolvedNavItem = {
  item: NavItemDef;
  adornment: NavAdornment;
};

export type NavCatalog = {
  id: string;
  groups: NavGroupDef[];
};
