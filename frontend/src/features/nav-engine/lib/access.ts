import type { FeatureKey } from "@/config/features";
import type { NavGroupDef, NavItemDef } from "@/features/nav-engine/types";

type AccessFns = {
  can: (permission: string | string[]) => boolean;
  canAny: (permissions: string[]) => boolean;
  /** Plan feature check of the current organization (omitted = no gate). */
  hasFeature?: (feature: FeatureKey | readonly FeatureKey[]) => boolean;
};

export function isNavEntryVisible(
  entry: Pick<NavItemDef, "permission" | "anyPermission"> & {
    feature?: NavItemDef["feature"];
    featureOff?: NavItemDef["featureOff"];
  },
  access: AccessFns,
): boolean {
  if (entry.permission && !access.can(entry.permission)) return false;
  if (entry.anyPermission?.length && !access.canAny(entry.anyPermission)) {
    return false;
  }
  if (
    entry.feature &&
    entry.featureOff !== "readonly" &&
    access.hasFeature &&
    !access.hasFeature(entry.feature)
  ) {
    return false;
  }
  return true;
}

/** True when a visible item's feature is off (shown read-only). */
export function isNavEntryReadOnly(
  entry: Pick<NavItemDef, "feature">,
  access: AccessFns,
): boolean {
  return Boolean(
    entry.feature && access.hasFeature && !access.hasFeature(entry.feature),
  );
}

export function visibleNavItems(group: NavGroupDef, access: AccessFns) {
  if (!isNavEntryVisible(group, access)) return [];
  return group.items.filter((item) => isNavEntryVisible(item, access));
}
