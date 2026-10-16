"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { HealthTab } from "@/features/platform-health/components/health-tab";
import { IssuesTab } from "@/features/platform-health/components/issues-tab";
import { UsageTab } from "@/features/platform-health/components/usage-tab";
import { useLocale } from "@/providers/locale-provider";

const TABS = ["usage", "health", "issues"] as const;
type HealthPageTab = (typeof TABS)[number];

/**
 * Platform usage and connection health (TEC-739): usage per organization,
 * the health of every registered source, and the issue list the failed
 * work alerts link to (`?tab=issues`).
 */
export function PlatformHealthPage() {
  const { t } = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const raw = searchParams.get("tab");
  const tab: HealthPageTab = TABS.includes(raw as HealthPageTab)
    ? (raw as HealthPageTab)
    : "usage";
  const source = searchParams.get("source") ?? undefined;

  const go = (next: string, extra?: Record<string, string>) => {
    const qs = new URLSearchParams();
    if (next !== "usage") qs.set("tab", next);
    for (const [k, v] of Object.entries(extra ?? {})) qs.set(k, v);
    const query = qs.toString();
    router.replace(query ? `${pathname}?${query}` : pathname, {
      scroll: false,
    });
  };

  return (
    <EntityPage
      title={t("platform_health.title")}
      description={t("platform_health.description")}
      permission={permissions.platformHealth.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("platform_health.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("platform_health.title") },
      ]}
    >
      <Tabs value={tab} onValueChange={(v) => go(v)} className="space-y-4">
        <TabsList className="flex h-auto flex-wrap">
          <TabsTrigger value="usage">
            {t("platform_health.tabs.usage")}
          </TabsTrigger>
          <TabsTrigger value="health">
            {t("platform_health.tabs.health")}
          </TabsTrigger>
          <TabsTrigger value="issues">
            {t("platform_health.tabs.issues")}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="usage">
          <UsageTab />
        </TabsContent>
        <TabsContent value="health">
          <HealthTab onViewIssues={(code) => go("issues", { source: code })} />
        </TabsContent>
        <TabsContent value="issues">
          <IssuesTab key={source ?? "all"} initialSource={source} />
        </TabsContent>
      </Tabs>
    </EntityPage>
  );
}
