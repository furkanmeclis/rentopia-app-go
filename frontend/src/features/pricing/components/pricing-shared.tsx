"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { AlertTriangle, CheckCircle2 } from "lucide-react";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { Timeline } from "@/components/common/timeline";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { EntitySectionCard, EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  ISSUE_LABEL_KEYS,
  VERSION_STATUS_LABEL_KEYS,
  VERSION_STATUS_TONES,
} from "@/features/pricing/constants";
import { usePricingActivity } from "@/features/pricing/hooks/use-pricing";
import { bandLabel, moneyText } from "@/features/pricing/lib/format";
import type {
  ActivationCheck,
  PricingIssue,
  RateBand,
  VersionStatus,
} from "@/features/pricing/types";
import { ApiError } from "@/lib/api";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Translated activation issue ("4-7. günler bant dışında"). */
export function issueText(t: Translate, issue: PricingIssue): string {
  const key = ISSUE_LABEL_KEYS[issue.code];
  if (!key) return issue.message;
  const params: Record<string, string | number> = { ...(issue.params ?? {}) };
  if (issue.ref) params.name = issue.ref.name || issue.ref.uuid.slice(0, 8);
  return t(key, params);
}

/** Issues of a refused activation (422 details carry field + code only). */
export function issuesFromError(error: unknown): PricingIssue[] {
  if (!(error instanceof ApiError) || error.status !== 422) return [];
  return error.details.map((d) => ({
    field: d.field ?? "",
    code: d.code ?? "",
    message: d.message ?? "",
  }));
}

/** Alert listing activation issues, or a "ready" note. */
export function IssueList({
  issues,
  ready,
}: {
  issues: PricingIssue[];
  ready?: boolean;
}) {
  const { t } = useLocale();
  if (issues.length === 0) {
    if (!ready) return null;
    return (
      <Alert>
        <CheckCircle2 className="size-4" />
        <AlertTitle>{t("pricing.activation.ready_title")}</AlertTitle>
        <AlertDescription>
          {t("pricing.activation.ready_body")}
        </AlertDescription>
      </Alert>
    );
  }
  return (
    <Alert variant="destructive">
      <AlertTriangle className="size-4" />
      <AlertTitle>
        {t("pricing.activation.blocked_title", { count: issues.length })}
      </AlertTitle>
      <AlertDescription>
        <ul className="list-disc space-y-1 pl-4">
          {issues.map((issue, i) => (
            <li key={`${issue.field}-${issue.code}-${i}`}>
              {issueText(t, issue)}
            </li>
          ))}
        </ul>
      </AlertDescription>
    </Alert>
  );
}

/**
 * Activation confirmation: shows the activation check (or the issues of a
 * refused attempt), an optional change note, and confirms. Activation freezes
 * the draft; reservations later bind to that version.
 */
export function ActivationDialog({
  open,
  onOpenChange,
  title,
  description,
  check,
  checkLoading,
  error,
  pending,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  check?: ActivationCheck;
  checkLoading?: boolean;
  error?: unknown;
  pending?: boolean;
  onConfirm: (note: string) => void;
}) {
  const { t } = useLocale();
  const [note, setNote] = useState("");
  const serverIssues = issuesFromError(error);
  const issues = serverIssues.length ? serverIssues : (check?.issues ?? []);
  const ready = Boolean(check?.ready) && serverIssues.length === 0;
  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setNote("");
        onOpenChange(next);
      }}
      title={title}
      description={description}
      className="sm:max-w-lg"
      footer={
        <>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={pending}
          >
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={!ready || pending}
            onClick={() => onConfirm(note.trim())}
          >
            {t("pricing.actions.activate")}
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        {checkLoading ? <Loading label={t("common.loading")} /> : null}
        {!checkLoading ? <IssueList issues={issues} ready={ready} /> : null}
        <div className="space-y-2">
          <Label htmlFor="activation-note">
            {t("pricing.fields.change_note")}
          </Label>
          <Textarea
            id="activation-note"
            value={note}
            maxLength={2000}
            placeholder={t("pricing.activation.note_placeholder")}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
      </div>
    </AppDialog>
  );
}

/** Read-only bands of a version (DataTable). */
export function BandsTable({
  bands,
  persistKey,
}: {
  bands: RateBand[];
  persistKey: string;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<RateBand>({
          id: "band",
          accessorFn: (row) => row.min_days,
          labelKey: "pricing.columns.band",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="font-medium">
              {bandLabel(t, row.original.min_days, row.original.max_days)}
            </span>
          ),
        }),
        createColumn<RateBand>({
          accessorKey: "daily_price",
          labelKey: "pricing.columns.daily_price",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.daily_price, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<RateBand, unknown>[],
    [locale, t],
  );
  return (
    <EntityTable
      columns={columns}
      data={bands}
      getRowId={(row) => String(row.min_days)}
      emptyTitle={t("pricing.bands.empty")}
      features={{
        persistKey,
        rowSelection: false,
        globalFilter: false,
        columnFilters: false,
        facetedFilters: false,
        sorting: false,
        pagination: false,
        viewMode: false,
      }}
    />
  );
}

export function VersionChip({ status }: { status: VersionStatus }) {
  const { t } = useLocale();
  return (
    <StatusChip
      label={t(VERSION_STATUS_LABEL_KEYS[status])}
      tone={VERSION_STATUS_TONES[status]}
    />
  );
}

export function VersionBadge({ version }: { version: number }) {
  return (
    <Badge variant="outline" className="tabular-nums">
      v{version}
    </Badge>
  );
}

const ACTIVITY_PAGE = 10;

/** Audit trail of a plan or rule. */
export function PricingActivityCard({
  kind,
  uuid,
}: {
  kind: "plans" | "rules";
  uuid: string;
}) {
  const { t, locale } = useLocale();
  const [limit, setLimit] = useState(ACTIVITY_PAGE);
  const query = usePricingActivity(kind, uuid, { limit, offset: 0 });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;
  const describe = (action: string) => {
    const key = `activity.actions.${action}`;
    const text = t(key);
    return text === key ? action : text;
  };
  return (
    <EntitySectionCard
      title={t("pricing.activity.title")}
      badge={query.data ? total : undefined}
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <EmptyState className="py-8" title={t("pricing.activity.empty")} />
      ) : null}
      {items.length > 0 ? (
        <div className="space-y-4">
          <Timeline
            items={items.map((entry) => ({
              id: entry.uuid,
              title: describe(entry.action),
              description: entry.actor
                ? `${entry.actor.name} ${entry.actor.surname}`.trim() ||
                  entry.actor.email
                : t("pricing.activity.system_actor"),
              meta: (
                <time
                  dateTime={entry.created_at}
                  title={datetime(entry.created_at, undefined, locale)}
                >
                  {relativeDatetime(entry.created_at, locale)}
                </time>
              ),
            }))}
          />
          {items.length < total ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={query.isFetching}
              onClick={() => setLimit((current) => current + ACTIVITY_PAGE)}
            >
              {t("pricing.activity.load_more")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </EntitySectionCard>
  );
}
