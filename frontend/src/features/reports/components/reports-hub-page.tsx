"use client";

import {
  ArrowLeftRight,
  ArrowRight,
  Building2,
  CalendarClock,
  Gauge,
  Share2,
  TrendingUp,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import {
  Card,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

type Entry = { key: string; icon: LucideIcon; href: string };

/** Raporlar (TEC-738, §15.1): the report center. */
export function ReportsHubPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const entries: Entry[] = [
    {
      key: "occupancy",
      icon: Gauge,
      href: routes.tenant.reports.occupancy(slug),
    },
    {
      key: "income_expense",
      icon: ArrowLeftRight,
      href: routes.tenant.reports.incomeExpense(slug),
    },
    {
      key: "channels",
      icon: Share2,
      href: routes.tenant.reports.channels(slug),
    },
    {
      key: "aging",
      icon: CalendarClock,
      href: routes.tenant.reports.aging(slug),
    },
    {
      key: "branches",
      icon: Building2,
      href: routes.tenant.reports.branches(slug),
    },
    {
      key: "vehicles",
      icon: TrendingUp,
      href: routes.tenant.finance.profitability(slug),
    },
  ];
  return (
    <EntityPage
      title={t("reports.title")}
      description={t("reports.description")}
      permission={permissions.tenant.finance.reports.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reports.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("reports.title") },
      ]}
    >
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {entries.map(({ key, icon: Icon, href }) => (
          <Link
            key={key}
            href={href}
            className="group focus-visible:ring-ring rounded-xl focus-visible:ring-2 focus-visible:outline-none"
          >
            <Card className="hover:border-primary/40 h-full transition-colors">
              <CardHeader className="gap-2">
                <div className="flex items-center justify-between">
                  <span className="bg-muted text-foreground flex size-9 items-center justify-center rounded-lg">
                    <Icon className="size-4" aria-hidden />
                  </span>
                  <ArrowRight
                    className="text-muted-foreground size-4 transition-transform group-hover:translate-x-0.5"
                    aria-hidden
                  />
                </div>
                <CardTitle className="text-base">
                  {t(`reports.hub.${key}.title`)}
                </CardTitle>
                <CardDescription>
                  {t(`reports.hub.${key}.description`)}
                </CardDescription>
              </CardHeader>
            </Card>
          </Link>
        ))}
      </div>
    </EntityPage>
  );
}
