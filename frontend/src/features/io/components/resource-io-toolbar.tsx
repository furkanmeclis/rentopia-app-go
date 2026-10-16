"use client";

import type { ReactNode } from "react";

import { ExportMenu } from "@/features/io/components/export-menu";
import { ImportButton } from "@/features/io/components/import-button";
import type { IoResource, ResourceCapabilities } from "@/features/io/types";
import { permissions } from "@/config/permissions";
import { usePermission } from "@/providers/permission-provider";

type ResourceIOToolbarProps = {
  resource: IoResource;
  query?: Record<string, string | undefined>;
  capabilities?: ResourceCapabilities;
  onImportComplete?: () => void;
};

const EXPORT_PERM: Partial<Record<IoResource, string>> = {
  "platform.users": permissions.users.export,
  "platform.roles": permissions.roles.export,
  "platform.notifications": permissions.notifications.export,
  "platform.activity": permissions.activity.read,
  "platform.organizations": permissions.organizations.export,
  "tenant.branches": permissions.tenant.branches.export,
  // Fleet exports need read access only (no separate export slug).
  "tenant.segments": permissions.tenant.fleet.read,
  "tenant.rental_groups": permissions.tenant.fleet.read,
  "tenant.rental_products": permissions.tenant.fleet.read,
  "tenant.vehicles": permissions.tenant.vehicles.export,
  "tenant.vehicle_documents": permissions.tenant.vehicles.export,
  "tenant.vehicle_maintenance": permissions.tenant.vehicles.export,
  "tenant.transfers": permissions.tenant.transfers.read,
  "tenant.one_way_rules": permissions.tenant.oneWay.read,
  "tenant.rate_plans": permissions.tenant.pricing.read,
  "tenant.date_rules": permissions.tenant.pricing.read,
  "tenant.customers": permissions.tenant.customers.export,
  "tenant.extras": permissions.tenant.extras.read,
  "tenant.campaigns": permissions.tenant.campaigns.read,
  "tenant.reservations": permissions.tenant.reservations.export,
  // Rental exports need read access only (no separate export slug).
  "tenant.rentals": permissions.tenant.rentals.read,
  "tenant.finance_accounts": permissions.tenant.finance.accounts.export,
  "tenant.ledger_entries": permissions.tenant.finance.accounts.export,
  "tenant.cash_accounts": permissions.tenant.finance.cash.export,
  "tenant.cash_movements": permissions.tenant.finance.cash.export,
  "tenant.expenses": permissions.tenant.finance.expenses.export,
  "tenant.toll_passes": permissions.tenant.tolls.export,
  "tenant.finance_collections": permissions.tenant.finance.accounts.export,
};

const IMPORT_PERM: Partial<Record<IoResource, string>> = {
  "platform.users": permissions.users.import,
  "platform.roles": permissions.roles.import,
  "tenant.branches": permissions.tenant.branches.import,
  "tenant.vehicles": permissions.tenant.vehicles.import,
  // The price list import only writes drafts.
  "tenant.rate_plans": permissions.tenant.pricing.write,
  "tenant.customers": permissions.tenant.customers.import,
  "tenant.cash_accounts": permissions.tenant.finance.cash.import,
  "tenant.cash_movements": permissions.tenant.finance.cash.import,
  "tenant.expenses": permissions.tenant.finance.expenses.import,
  "tenant.toll_passes": permissions.tenant.tolls.import,
};

export function ResourceIOToolbar({
  resource,
  query,
  capabilities,
  onImportComplete,
}: ResourceIOToolbarProps) {
  const { can } = usePermission();
  const nodes: ReactNode[] = [];

  const exportPerm = EXPORT_PERM[resource];
  if (capabilities?.export && exportPerm && can(exportPerm)) {
    nodes.push(<ExportMenu key="export" resource={resource} query={query} />);
  }

  const importPerm = IMPORT_PERM[resource];
  if (capabilities?.import && importPerm && can(importPerm)) {
    nodes.push(
      <ImportButton
        key="import"
        resource={resource}
        onComplete={onImportComplete}
      />,
    );
  }

  if (!nodes.length) return null;
  return <>{nodes}</>;
}
