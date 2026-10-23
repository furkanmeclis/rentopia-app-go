"use client";

import { ListChecks, RefreshCw, Settings2 } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ResourceIOToolbar } from "@/features/io";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tollProblemText } from "@/features/tolls/components/toll-decision-dialogs";
import { TollPassesTable } from "@/features/tolls/components/toll-passes-table";
import { TollSettingsDialog } from "@/features/tolls/components/toll-settings-dialog";
import { TOLLS_RESOURCE } from "@/features/tolls/constants";
import {
  useFetchTollsNow,
  useTollFacets,
  useTollPassesMeta,
} from "@/features/tolls/hooks/use-tolls";
import { useFeature } from "@/providers/feature-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

function HeaderActions({ review }: { review: boolean }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const facets = useTollFacets(!review);
  const fetchNow = useFetchTollsNow();
  const hgs = useFeature("hgs");
  const [settingsOpen, setSettingsOpen] = useState(false);
  const queue = facets.data?.review ?? 0;
  return (
    <div className="flex flex-wrap items-center gap-2">
      {!review ? (
        <Button asChild variant="outline" size="sm">
          <Link href={routes.tenant.tolls.review(slug)}>
            <ListChecks className="size-4" />
            {t("tolls.review.title")}
            {queue > 0 ? (
              <Badge variant="warning" className="ml-1 px-1.5 py-0">
                {queue}
              </Badge>
            ) : null}
          </Link>
        </Button>
      ) : null}
      {hgs && can(permissions.tenant.tolls.import) ? (
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={fetchNow.isPending}
          onClick={() =>
            fetchNow.mutate(undefined, {
              onError: (error) => appToast.error(tollProblemText(t, error)),
            })
          }
        >
          <RefreshCw className="size-4" />
          {t("tolls.actions.fetch_now")}
        </Button>
      ) : null}
      <Button
        type="button"
        variant="outline"
        size="sm"
        onClick={() => setSettingsOpen(true)}
      >
        <Settings2 className="size-4" />
        {t("tolls.actions.settings")}
      </Button>
      <TollSettingsDialog
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
      />
    </div>
  );
}

/**
 * "HGS/OGS geçişleri": every toll pass in the member's branch scope with
 * the file import (provider fallback), export, "fetch now" and settings.
 */
export function TollPassesPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const meta = useTollPassesMeta();
  const hgs = useFeature("hgs");
  // Without the hgs feature the list and the export stay; no import.
  const capabilities =
    hgs || !meta.data?.capabilities
      ? meta.data?.capabilities
      : { ...meta.data.capabilities, import: false };
  return (
    <EntityPage
      title={t("tolls.title")}
      description={t("tolls.description")}
      permission={permissions.tenant.tolls.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tolls.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("tolls.title") },
      ]}
      actions={<HeaderActions review={false} />}
    >
      <TollPassesTable
        mode="all"
        persistKey="tenant-toll-passes-v1"
        toolbarExtra={(query) => (
          <ResourceIOToolbar
            resource={TOLLS_RESOURCE}
            query={query}
            capabilities={capabilities}
          />
        )}
      />
    </EntityPage>
  );
}

/**
 * "HGS inceleme kuyruğu": passes that wait for a person (no rental found,
 * missing handover times, overlap, transfer, unknown vehicle, unassigned),
 * oldest first, with assign / ignore and the reason dialog.
 */
export function TollReviewPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <EntityPage
      title={t("tolls.review.title")}
      description={t("tolls.review.description")}
      permission={permissions.tenant.tolls.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tolls.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("tolls.title"), href: routes.tenant.tolls.root(slug) },
        { label: t("tolls.review.title") },
      ]}
      actions={<HeaderActions review />}
    >
      <TollPassesTable mode="review" persistKey="tenant-toll-review-v1" />
    </EntityPage>
  );
}
