import {
  Archive,
  ArrowRightLeft,
  CircleCheck,
  CircleOff,
  Hammer,
  Layers,
  ListChecks,
  Trash2,
  UserCheck,
  UserX,
  Wrench,
  type LucideIcon,
} from "lucide-react";

import type {
  BulkActionDef,
  BulkActionMeta,
  BulkResource,
} from "@/features/bulk-engine/types";

const bulkActionIconCatalog: Record<
  BulkResource,
  Record<string, LucideIcon>
> = {
  "platform.users": {
    disable: UserX,
    enable: UserCheck,
  },
  "platform.roles": {
    delete: Trash2,
  },
  "platform.organizations": {
    activate: CircleCheck,
    suspend: CircleOff,
  },
  "tenant.branches": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "tenant.rental_groups": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "tenant.rental_products": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "tenant.vehicles": {
    set_status: Wrench,
    move_branch: ArrowRightLeft,
    add_rental_group: ListChecks,
    replace_rental_group: Layers,
    activate: CircleCheck,
    deactivate: CircleOff,
    start_maintenance: Hammer,
  },
  "tenant.one_way_rules": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
  "tenant.extras": {
    publish: CircleCheck,
    archive: Archive,
  },
  "tenant.campaigns": {
    activate: CircleCheck,
    deactivate: CircleOff,
  },
};

export function resolveBulkActionIcon(
  resource: BulkResource,
  actionId: string,
): LucideIcon {
  const icon = bulkActionIconCatalog[resource]?.[actionId];
  if (!icon) {
    throw new Error(
      `bulk-engine: missing icon for ${resource} action "${actionId}"`,
    );
  }
  return icon;
}

/** Attach mandatory icons to meta bulk actions for UI rendering. */
export function resolveBulkActionsWithIcons(
  resource: BulkResource,
  actions: BulkActionMeta[],
): BulkActionDef[] {
  return actions.map((action) => ({
    ...action,
    icon: resolveBulkActionIcon(resource, action.id),
  }));
}
