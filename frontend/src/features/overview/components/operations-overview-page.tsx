"use client";

import { Banknote, Building2, Gauge, KeyRound, RefreshCw } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import type { ReactNode } from "react";

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import { ErrorState } from "@/components/common/error-state";
import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout";
import { Button } from "@/components/ui/button";
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  ApprovalsCard,
  AvailabilityProblemsCard,
  AwaitingPaymentCard,
  DocumentsCard,
  IntegrationJobsCard,
  OverdueReturnsCard,
  TodayOperationsSection,
  useNow,
} from "@/features/overview/components/overview-sections";
import {
  useOverview,
  useOverviewLiveResources,
} from "@/features/overview/hooks/use-overview";
import type { TenantOverview } from "@/features/overview/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useTenantLiveRefetch } from "@/features/tenant/hooks/use-tenant-live-refetch";
import { tenantRoleLabel } from "@/features/tenant/lib/role-label";
import { moneyText } from "@/features/pricing/lib/format";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const ALL_BRANCHES = "__all__";

function KpiCard({
  title,
  icon,
  value,
  hint,
  children,
}: {
  title: string;
  icon: ReactNode;
  value: ReactNode;
  hint?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="text-muted-foreground text-sm font-medium">
          {title}
        </CardTitle>
        <span className="text-muted-foreground">{icon}</span>
      </CardHeader>
      <CardContent className="space-y-2">
        <div className="text-2xl font-semibold tracking-tight tabular-nums">
          {value}
        </div>
        {children}
        {hint ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
      </CardContent>
    </Card>
  );
}

function Kpis({ data }: { data: TenantOverview }) {
  const { t, locale } = useLocale();
  const { occupancy, active_rentals: active } = data.kpis;
  const collections = data.kpis.collections_today;
  if (!occupancy && !active && !collections) return null;
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {occupancy ? (
        <KpiCard
          title={t("overview.kpi.occupancy")}
          icon={<Gauge className="size-4" />}
          value={`%${occupancy.rate.toLocaleString(locale === "tr" ? "tr-TR" : "en-US")}`}
          hint={t("overview.kpi.occupancy_hint", {
            rented: occupancy.rented,
            rentable: occupancy.rentable,
          })}
        >
          <Progress
            value={occupancy.rate}
            aria-label={t("overview.kpi.occupancy")}
          />
        </KpiCard>
      ) : null}
      {active ? (
        <KpiCard
          title={t("overview.kpi.active_rentals")}
          icon={<KeyRound className="size-4" />}
          value={active.active}
          hint={
            active.overdue > 0
              ? t("overview.kpi.active_overdue", { count: active.overdue })
              : t("overview.kpi.active_none_overdue")
          }
        />
      ) : null}
      {collections ? (
        <KpiCard
          title={t("overview.kpi.collections")}
          icon={<Banknote className="size-4" />}
          value={moneyText(collections.net, locale)}
          hint={t("overview.kpi.collections_hint", {
            count: collections.count,
            amount: moneyText(collections.amount, locale),
            refunded: moneyText(collections.refunded, locale),
          })}
        />
      ) : null}
    </div>
  );
}

function OverviewSkeleton() {
  return (
    <div className="space-y-4" aria-busy>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-32 w-full" />
        ))}
      </div>
      <Skeleton className="h-64 w-full" />
      <div className="grid gap-4 lg:grid-cols-2">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-48 w-full" />
        ))}
      </div>
    </div>
  );
}

/**
 * Tenant panel home ("Operasyon özeti", requirement §15.1, TEC-716): KPIs
 * and what needs attention today, each part shown only with its read
 * permission and limited to the member's branches. Live: refetches
 * (debounced) when reservations, rentals, tasks, payments, resolution
 * tasks, integration jobs, documents or vehicles change.
 */
export function OperationsOverviewPage() {
  const { t, locale } = useLocale();
  const tenant = useTenant();
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const branch = search.get("branch") || undefined;
  const query = useOverview(branch);
  const live = useOverviewLiveResources();
  useTenantLiveRefetch(live, () => query.refetch());
  const now = useNow();
  const data = query.data;
  const roleLabel = tenantRoleLabel(t, tenant.role);

  const setBranch = (value: string) => {
    const next = new URLSearchParams(search.toString());
    if (value === ALL_BRANCHES) next.delete("branch");
    else next.set("branch", value);
    const q = next.toString();
    router.replace(q ? `${pathname}?${q}` : pathname, { scroll: false });
  };

  const branchFilter =
    data && data.branches.length > 1 ? (
      <Select value={branch ?? ALL_BRANCHES} onValueChange={setBranch}>
        <SelectTrigger
          className="min-w-0 flex-1 sm:w-56 sm:flex-none"
          aria-label={t("overview.branch_filter")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ALL_BRANCHES}>
            {data.branch_scope === "all"
              ? t("overview.all_branches")
              : t("overview.my_branches")}
          </SelectItem>
          {data.branches.map((b) => (
            <SelectItem key={b.uuid} value={b.uuid}>
              {b.code} · {b.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    ) : null;

  const sectionProps = data
    ? { data, slug: tenant.slug, branch, now }
    : undefined;
  const hasAnything =
    data &&
    (data.kpis.occupancy ||
      data.kpis.active_rentals ||
      data.kpis.collections_today ||
      data.today_operations ||
      data.overdue_returns ||
      data.approvals ||
      data.awaiting_payment ||
      data.availability_problems ||
      data.integration_jobs ||
      data.documents);

  return (
    <div className="space-y-6">
      <PageHeader
        title={tenant.organization?.name ?? tenant.slug}
        description={t("overview.description")}
        icon={<Building2 className="size-6" />}
        actions={
          <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto">
            {branchFilter}
            <Button
              type="button"
              variant="outline"
              size="icon"
              onClick={() => void query.refetch()}
              disabled={query.isFetching}
              aria-label={t("overview.refresh")}
            >
              <RefreshCw
                className={query.isFetching ? "size-4 animate-spin" : "size-4"}
              />
            </Button>
          </div>
        }
      />
      {data ? (
        <p className="text-muted-foreground -mt-3 text-xs">
          {t("overview.updated_at", {
            at: datetime(data.generated_at, "HH:mm:ss", locale),
          })}
          {data.branch
            ? ` · ${t("overview.filtered_branch", { branch: `${data.branch.code} · ${data.branch.name}` })}`
            : null}
        </p>
      ) : null}

      {query.isError && !data ? (
        <ErrorState
          title={t("overview.load_failed")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !data || !sectionProps ? (
        <OverviewSkeleton />
      ) : !hasAnything ? (
        <EmptyState
          title={t("overview.nothing_title")}
          description={t("overview.nothing_description", {
            role: roleLabel || "—",
          })}
        />
      ) : (
        <>
          <Kpis data={data} />
          <TodayOperationsSection {...sectionProps} />
          <div className="grid gap-4 lg:grid-cols-2">
            <OverdueReturnsCard {...sectionProps} />
            <ApprovalsCard {...sectionProps} />
            <AwaitingPaymentCard {...sectionProps} />
            <AvailabilityProblemsCard {...sectionProps} />
            <DocumentsCard {...sectionProps} />
            <IntegrationJobsCard {...sectionProps} />
          </div>
        </>
      )}
    </div>
  );
}
