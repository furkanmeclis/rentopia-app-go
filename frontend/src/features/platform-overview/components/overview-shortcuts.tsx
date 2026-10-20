"use client";

import {
  Activity,
  Bell,
  Building2,
  CarFront,
  Globe,
  Settings2,
  Shield,
  Users,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { settingsNavPermissions } from "@/config/nav";
import { routes } from "@/config/routes";
import type { OverviewScopes } from "@/features/platform-overview/hooks/use-overview-query";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Shortcut = {
  id: string;
  labelKey: string;
  href: string;
  icon: LucideIcon;
  visible: boolean;
};

/** One compact row of the platform's main areas the user can open. */
export function OverviewShortcuts({ scopes }: { scopes: OverviewScopes }) {
  const { t } = useLocale();
  const { canAny } = usePermission();

  const shortcuts: Shortcut[] = [
    {
      id: "organizations",
      labelKey: "layout.nav_organizations",
      href: routes.platform.organizations.root,
      icon: Building2,
      visible: scopes.organizations,
    },
    {
      id: "health",
      labelKey: "layout.nav_platform_health",
      href: routes.platform.health,
      icon: Activity,
      visible: scopes.health,
    },
    {
      id: "domains",
      labelKey: "layout.nav_site_domains",
      href: routes.platform.siteDomains,
      icon: Globe,
      visible: scopes.siteDomains,
    },
    {
      id: "users",
      labelKey: "layout.nav_users",
      href: routes.platform.users.root,
      icon: Users,
      visible: scopes.users,
    },
    {
      id: "roles",
      labelKey: "layout.nav_roles",
      href: routes.platform.roles.root,
      icon: Shield,
      visible: scopes.roles,
    },
    {
      id: "catalog",
      labelKey: "layout.nav_catalog",
      href: routes.platform.catalog.root,
      icon: CarFront,
      visible: scopes.catalog,
    },
    {
      id: "notifications",
      labelKey: "layout.nav_notifications",
      href: routes.platform.notifications.root,
      icon: Bell,
      visible: scopes.notifications,
    },
    {
      id: "settings",
      labelKey: "layout.nav_settings",
      href: routes.platform.settings.root,
      icon: Settings2,
      visible: canAny(settingsNavPermissions),
    },
  ];

  const visible = shortcuts.filter((s) => s.visible);
  if (!visible.length) return null;

  return (
    <nav
      aria-label={t("dashboard.shortcuts")}
      className="-mx-1 overflow-x-auto"
    >
      <ul className="flex w-max min-w-full flex-nowrap gap-2 px-1 pb-1 sm:w-auto sm:flex-wrap">
        {visible.map(({ id, labelKey, href, icon: Icon }) => (
          <li key={id}>
            <Button
              asChild
              variant="outline"
              size="sm"
              className="bg-card h-9 rounded-full px-3.5"
            >
              <Link href={href}>
                <Icon className="text-muted-foreground size-4" />
                {t(labelKey)}
              </Link>
            </Button>
          </li>
        ))}
      </ul>
    </nav>
  );
}
