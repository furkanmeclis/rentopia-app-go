"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Clock, Unlock } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { AppForm, AppInput } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  DEFAULT_HOLD_MINUTES,
  HOLDS_RESOURCE,
  MAX_HOLD_MINUTES,
} from "@/features/availability/constants";
import {
  useExtendHold,
  useHoldsList,
  useReleaseHold,
} from "@/features/availability/hooks/use-availability";
import type { AvailabilityHold } from "@/features/availability/types";
import {
  RangeCell,
  RouteCell,
} from "@/features/fleet-ops/components/fleet-ops-cells";
import {
  SALES_CHANNEL_LABEL_KEYS,
  SALES_CHANNELS,
} from "@/features/tenant-settings/constants";
import type { SalesChannelCode } from "@/features/tenant-settings/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { PlateBadge } from "@/features/vehicles/components/vehicle-cells";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

const EXTEND_FORM_ID = "availability-extend-form";

function channelLabel(t: (key: string) => string, code: string) {
  return SALES_CHANNELS.includes(code as SalesChannelCode)
    ? t(SALES_CHANNEL_LABEL_KEYS[code as SalesChannelCode])
    : code;
}

/**
 * Active holds ("opsiyonlar") in the caller's branch scope, newest first:
 * the provisional plate (or the promised one), places, range and expiry,
 * with extend and release. Live: refreshes when a hold changes.
 */
export function HoldsTable() {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const listState = useServerListState({
    initialPageSize: 10,
    realtime: tenantLiveList(organization, HOLDS_RESOURCE),
  });
  const params = useMemo(
    () => ({
      limit: listState.params.limit,
      offset: listState.params.offset,
      status: "active",
    }),
    [listState.params.limit, listState.params.offset],
  );
  const query = useHoldsList(params);
  const release = useReleaseHold();
  const extend = useExtendHold();
  const [releaseTarget, setReleaseTarget] = useState<AvailabilityHold | null>(
    null,
  );
  const [extendTarget, setExtendTarget] = useState<AvailabilityHold | null>(
    null,
  );
  const extendSchema = useMemo(
    () =>
      z.object({
        minutes: z
          .string()
          .trim()
          .refine((v) => {
            const n = Number(v);
            return Number.isInteger(n) && n >= 1 && n <= MAX_HOLD_MINUTES;
          }, t("availability.validation.hold_minutes")),
      }),
    [t],
  );
  const hold = permissions.tenant.availability.hold;

  const columns = useMemo(
    () =>
      [
        createColumn<AvailabilityHold>({
          id: "product",
          accessorFn: (row) => row.product.code,
          labelKey: "availability.holds.columns.product",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span>
              <code className="text-xs">{row.original.product.code}</code>{" "}
              {row.original.product.title}
            </span>
          ),
        }),
        createColumn<AvailabilityHold>({
          id: "vehicle",
          accessorFn: (row) => row.vehicle.plate,
          labelKey: "availability.holds.columns.vehicle",
          enableSorting: false,
          cell: ({ row }) => (
            <div className="flex flex-wrap items-center gap-1.5">
              <PlateBadge plate={row.original.vehicle.plate} />
              <StatusChip
                label={
                  row.original.plate_fixed
                    ? t("availability.holds.plate_fixed")
                    : t("availability.holds.plate_provisional")
                }
                tone={row.original.plate_fixed ? "success" : "default"}
              />
            </div>
          ),
        }),
        createColumn<AvailabilityHold>({
          id: "route",
          accessorFn: (row) => row.pickup_branch.code,
          labelKey: "availability.holds.columns.route",
          enableSorting: false,
          cell: ({ row }) => (
            <RouteCell
              from={row.original.pickup_branch}
              to={row.original.return_branch}
            />
          ),
        }),
        createColumn<AvailabilityHold>({
          id: "range",
          accessorFn: (row) => row.starts_at,
          labelKey: "availability.holds.columns.range",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <RangeCell
              start={row.original.starts_at}
              end={row.original.ends_at}
            />
          ),
        }),
        createColumn<AvailabilityHold>({
          id: "expires_at",
          accessorFn: (row) => row.expires_at ?? "",
          labelKey: "availability.holds.columns.expires_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.expires_at ? (
              <span
                className="whitespace-nowrap tabular-nums"
                title={datetime(row.original.expires_at, undefined, locale)}
              >
                {relativeDatetime(row.original.expires_at, locale)}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<AvailabilityHold>({
          id: "channel",
          accessorFn: (row) => row.channel,
          labelKey: "availability.holds.columns.channel",
          enableSorting: false,
          cell: ({ row }) => channelLabel(t, row.original.channel),
        }),
        createColumn<AvailabilityHold>({
          id: "created_by",
          accessorFn: (row) => row.created_by?.email ?? "",
          labelKey: "availability.holds.columns.created_by",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.created_by
              ? `${row.original.created_by.name} ${row.original.created_by.surname}`.trim() ||
                row.original.created_by.email
              : "—",
        }),
        createColumn<AvailabilityHold>({
          id: "note",
          accessorFn: (row) => row.note,
          labelKey: "availability.holds.columns.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<AvailabilityHold>({
          id: "actions",
          labelKey: "availability.holds.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => (
            <EntityRowActions
              actions={[
                {
                  id: "extend",
                  label: t("availability.actions.extend"),
                  icon: Clock,
                  permission: hold,
                  onSelect: () => setExtendTarget(row.original),
                },
                {
                  id: "release",
                  label: t("availability.actions.release"),
                  icon: Unlock,
                  permission: hold,
                  variant: "destructive",
                  onSelect: () => setReleaseTarget(row.original),
                },
              ]}
            />
          ),
        }),
      ] as ColumnDef<AvailabilityHold, unknown>[],
    [hold, locale, t],
  );

  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 10)),
  );

  return (
    <>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorDescription={t("availability.holds.load_failed")}
        onRetry={() => void query.refetch()}
        realtime={listState.realtimeFor(() => query.refetch())}
        emptyTitle={t("availability.holds.empty_title")}
        emptyDescription={t("availability.holds.empty_description")}
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "tenant-availability-holds-v1",
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />

      <AppDialog
        open={Boolean(extendTarget)}
        onOpenChange={(open) => {
          if (!open) setExtendTarget(null);
        }}
        title={t("availability.extend_dialog.title")}
        description={t("availability.extend_dialog.description")}
        className="sm:max-w-md"
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setExtendTarget(null)}
            >
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              form={EXTEND_FORM_ID}
              disabled={extend.isPending}
            >
              {t("availability.actions.extend")}
            </Button>
          </>
        }
      >
        {extendTarget ? (
          <AppForm<{ minutes: string }>
            key={extendTarget.uuid}
            id={EXTEND_FORM_ID}
            schema={extendSchema}
            defaultValues={{ minutes: String(DEFAULT_HOLD_MINUTES) }}
            onSubmit={async (values) => {
              await extend.mutateAsync({
                uuid: extendTarget.uuid,
                minutes: Number(values.minutes),
              });
              setExtendTarget(null);
            }}
          >
            <AppInput
              name="minutes"
              type="number"
              inputMode="numeric"
              min={1}
              max={MAX_HOLD_MINUTES}
              label={t("availability.fields.extend_minutes")}
              description={t("availability.fields.extend_minutes_hint")}
            />
          </AppForm>
        ) : null}
      </AppDialog>

      <ConfirmDialog
        open={Boolean(releaseTarget)}
        title={t("availability.release_dialog.title")}
        description={
          releaseTarget
            ? t("availability.release_dialog.description", {
                product: releaseTarget.product.code,
                plate: releaseTarget.vehicle.plate,
              })
            : undefined
        }
        confirmLabel={t("availability.actions.release")}
        variant="destructive"
        isPending={release.isPending}
        onCancel={() => setReleaseTarget(null)}
        onConfirm={() => {
          if (!releaseTarget) return;
          release.mutate(releaseTarget.uuid, {
            onSettled: () => setReleaseTarget(null),
          });
        }}
      />
    </>
  );
}
