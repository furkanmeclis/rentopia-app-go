import {
  Activity,
  ArrowLeftRight,
  BadgePercent,
  Banknote,
  Bell,
  Boxes,
  Building2,
  Calculator,
  CalendarCheck,
  CalendarClock,
  CalendarDays,
  CalendarRange,
  CarFront,
  Cpu,
  Download,
  Gauge,
  HardDrive,
  KeyRound,
  Layers,
  LayoutDashboard,
  LogIn,
  Palette,
  Route,
  ScrollText,
  Settings2,
  Share2,
  Shield,
  SlidersHorizontal,
  Sparkles,
  Store,
  Tags,
  Upload,
  Users,
} from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNav } from "@/features/nav-engine";
import type { NavCatalog } from "@/features/nav-engine";
import { usersNavItem } from "@/features/users/nav";

const integrationReadPermissions = [
  permissions.integrations.google.read,
  permissions.integrations.github.read,
  permissions.integrations.facebook.read,
  permissions.integrations.apple.read,
];

/**
 * System Settings sub-sidebar (`/platform/settings/*`). Rendered by
 * `features/settings` (not by `NavEngine`), but uses the same catalog shape
 * and `permission` / `anyPermission` gating.
 */
export const settingsNav = defineNav({
  id: "settings",
  groups: [
    {
      id: "system",
      labelKey: "settings.subnav.group_system",
      icon: Activity,
      defaultOpen: true,
      items: [
        {
          id: "system-overview",
          titleKey: "settings.subnav.system_overview",
          href: routes.platform.settings.root,
          icon: Activity,
          permission: permissions.settings.read,
        },
        {
          id: "system-environment",
          titleKey: "settings.subnav.system_environment",
          href: routes.platform.settings.environment,
          icon: Cpu,
          permission: permissions.settings.read,
        },
        {
          id: "logs",
          titleKey: "layout.nav_logs",
          href: routes.platform.settings.logs,
          icon: ScrollText,
          permission: permissions.logs.read,
        },
        {
          id: "activity",
          titleKey: "layout.nav_activity",
          href: routes.platform.settings.activity,
          icon: Activity,
          permission: permissions.activity.read,
        },
      ],
    },
    {
      id: "general",
      labelKey: "settings.subnav.group_general",
      icon: Settings2,
      defaultOpen: true,
      items: [
        {
          id: "general-branding",
          titleKey: "settings.subnav.general_branding",
          href: routes.platform.settings.general,
          icon: Palette,
          permission: permissions.settings.read,
        },
      ],
    },
    {
      id: "security",
      labelKey: "settings.subnav.group_security",
      icon: Shield,
      defaultOpen: true,
      items: [
        {
          id: "roles",
          titleKey: "layout.nav_roles",
          href: routes.platform.settings.roles,
          icon: Shield,
          permission: permissions.roles.read,
        },
        {
          id: "auth-settings",
          titleKey: "settings.subnav.auth_methods",
          href: routes.platform.settings.auth,
          icon: LogIn,
          permission: permissions.authSettings.read,
        },
        {
          id: "access-policy",
          titleKey: "settings.subnav.access_policy",
          href: routes.platform.settings.access,
          icon: KeyRound,
          permission: permissions.access.read,
        },
      ],
    },
    {
      id: "integrations",
      labelKey: "settings.subnav.group_integrations",
      icon: Share2,
      defaultOpen: true,
      items: [
        {
          id: "oauth-integrations",
          titleKey: "settings.subnav.oauth_providers",
          href: routes.platform.settings.integrations,
          icon: Share2,
          anyPermission: integrationReadPermissions,
        },
        {
          id: "storage",
          titleKey: "settings.subnav.storage",
          href: routes.platform.settings.storage,
          icon: HardDrive,
          permission: permissions.storage.read,
        },
      ],
    },
    {
      id: "io",
      labelKey: "settings.subnav.group_maintenance",
      icon: Upload,
      defaultOpen: true,
      items: [
        {
          id: "imports",
          titleKey: "layout.nav_imports",
          href: routes.platform.settings.imports,
          icon: Upload,
          permission: permissions.imports.read,
        },
        {
          id: "exports",
          titleKey: "layout.nav_exports",
          href: routes.platform.settings.exports,
          icon: Download,
          permission: permissions.exports.read,
        },
        {
          id: "purge-rules",
          titleKey: "settings.subnav.purge_rules",
          href: routes.platform.settings.maintenance,
          icon: Sparkles,
          permission: permissions.logs.read,
        },
      ],
    },
  ],
});

/** Every slug that unlocks at least one settings subpage (OR-gate for the shell). */
function catalogPermissions(catalog: NavCatalog): string[] {
  const slugs = new Set<string>();
  for (const group of catalog.groups) {
    for (const item of group.items) {
      if (typeof item.permission === "string") slugs.add(item.permission);
      else if (item.permission) item.permission.forEach((p) => slugs.add(p));
      item.anyPermission?.forEach((p) => slugs.add(p));
    }
  }
  return [...slugs].sort();
}

export const settingsNavPermissions = catalogPermissions(settingsNav);

export const platformNav = defineNav({
  id: "platform",
  groups: [
    {
      id: "general",
      labelKey: "layout.nav_platform",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.platform.home,
          icon: LayoutDashboard,
        },
      ],
    },
    {
      id: "manage",
      labelKey: "layout.section_manage",
      icon: Shield,
      defaultOpen: true,
      items: [
        {
          id: "organizations",
          titleKey: "layout.nav_organizations",
          href: routes.platform.organizations.root,
          icon: Building2,
          permission: permissions.organizations.read,
        },
        usersNavItem,
        {
          id: "catalog",
          titleKey: "layout.nav_catalog",
          href: routes.platform.catalog.root,
          icon: CarFront,
          permission: permissions.catalog.read,
        },
        {
          id: "notifications",
          titleKey: "layout.nav_notifications",
          href: routes.platform.notifications.root,
          icon: Bell,
          permission: permissions.notifications.platformRead,
        },
        {
          id: "settings",
          titleKey: "layout.nav_settings",
          href: routes.platform.settings.root,
          icon: Settings2,
          anyPermission: settingsNavPermissions,
        },
      ],
    },
  ],
});

/**
 * Organization shell (`/t/{slug}`) sidebar. Built per slug because every href
 * carries the organization. Items gate on `tenant.*` permissions, which the
 * tenant shell resolves for the current organization only.
 */
export function tenantNav(slug: string): NavCatalog {
  return defineNav({
    id: "tenant",
    groups: [
      {
        id: "general",
        labelKey: "layout.section_organization",
        icon: Building2,
        defaultOpen: true,
        items: [
          {
            id: "home",
            titleKey: "layout.home",
            href: routes.tenant.home(slug),
            icon: LayoutDashboard,
          },
        ],
      },
      {
        id: "operations",
        labelKey: "layout.section_operations",
        icon: Store,
        defaultOpen: true,
        items: [
          {
            id: "branches",
            titleKey: "layout.nav_branches",
            href: routes.tenant.branches.root(slug),
            icon: Store,
            permission: permissions.tenant.branches.read,
          },
          {
            id: "availability",
            titleKey: "layout.nav_availability",
            href: routes.tenant.availability(slug),
            icon: CalendarCheck,
            permission: permissions.tenant.availability.read,
          },
        ],
      },
      {
        id: "fleet",
        labelKey: "layout.section_fleet",
        icon: CarFront,
        defaultOpen: true,
        items: [
          {
            id: "fleet-vehicles",
            titleKey: "layout.nav_fleet_vehicles",
            href: routes.tenant.fleet.vehicles.root(slug),
            icon: CarFront,
            permission: permissions.tenant.vehicles.read,
          },
          {
            id: "fleet-calendar",
            titleKey: "layout.nav_fleet_calendar",
            href: routes.tenant.fleet.calendar(slug),
            icon: CalendarRange,
            permission: permissions.tenant.vehicles.read,
          },
          {
            id: "fleet-alerts",
            titleKey: "layout.nav_fleet_alerts",
            href: routes.tenant.fleet.alerts(slug),
            icon: CalendarClock,
            permission: permissions.tenant.vehicles.read,
          },
          {
            id: "fleet-transfers",
            titleKey: "layout.nav_fleet_transfers",
            href: routes.tenant.fleet.transfers.root(slug),
            icon: ArrowLeftRight,
            permission: permissions.tenant.transfers.read,
          },
          {
            id: "fleet-one-way-rules",
            titleKey: "layout.nav_fleet_one_way_rules",
            href: routes.tenant.fleet.oneWayRules.root(slug),
            icon: Route,
            permission: permissions.tenant.oneWay.read,
          },
          {
            id: "fleet-capacity-rules",
            titleKey: "layout.nav_fleet_capacity_rules",
            href: routes.tenant.fleet.capacityRules(slug),
            icon: Gauge,
            permission: permissions.tenant.capacityRules.read,
          },
          {
            id: "fleet-segments",
            titleKey: "layout.nav_fleet_segments",
            href: routes.tenant.fleet.segments.root(slug),
            icon: Layers,
            permission: permissions.tenant.fleet.read,
          },
          {
            id: "fleet-groups",
            titleKey: "layout.nav_fleet_groups",
            href: routes.tenant.fleet.groups.root(slug),
            icon: Boxes,
            permission: permissions.tenant.fleet.read,
          },
          {
            id: "fleet-products",
            titleKey: "layout.nav_fleet_products",
            href: routes.tenant.fleet.products.root(slug),
            icon: Tags,
            permission: permissions.tenant.fleet.read,
          },
        ],
      },
      {
        id: "pricing",
        labelKey: "layout.section_pricing",
        icon: BadgePercent,
        defaultOpen: true,
        items: [
          {
            id: "pricing-plans",
            titleKey: "layout.nav_pricing_plans",
            href: routes.tenant.pricing.plans.root(slug),
            icon: Banknote,
            permission: permissions.tenant.pricing.read,
          },
          {
            id: "pricing-rules",
            titleKey: "layout.nav_pricing_rules",
            href: routes.tenant.pricing.rules.root(slug),
            icon: CalendarDays,
            permission: permissions.tenant.pricing.read,
          },
          {
            id: "pricing-preview",
            titleKey: "layout.nav_pricing_preview",
            href: routes.tenant.pricing.preview(slug),
            icon: Calculator,
            permission: permissions.tenant.pricing.read,
          },
        ],
      },
      {
        id: "settings",
        labelKey: "layout.section_tenant_settings",
        icon: Settings2,
        defaultOpen: false,
        items: [
          {
            id: "settings-general",
            titleKey: "layout.nav_tenant_settings_general",
            href: routes.tenant.settings.root(slug),
            icon: SlidersHorizontal,
            permission: permissions.tenant.settings.read,
          },
          {
            id: "settings-members",
            titleKey: "layout.nav_tenant_settings_members",
            href: routes.tenant.settings.members(slug),
            icon: Users,
            permission: permissions.tenant.members.read,
          },
          {
            id: "settings-roles",
            titleKey: "layout.nav_tenant_settings_roles",
            href: routes.tenant.settings.roles.root(slug),
            icon: Shield,
            permission: permissions.tenant.roles.read,
          },
        ],
      },
    ],
  });
}
