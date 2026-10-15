"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { AlertTriangle, CheckCircle2, Plus, Search, Undo2 } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { PermissionGuard } from "@/components/common/permission-guard";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import {
  AppDatePicker,
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
} from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { AllocationStatusChip } from "@/features/extras/components/extras-shared";
import {
  ALLOCATIONS_RESOURCE,
  ALLOCATION_KINDS,
  ALLOCATION_STATUSES,
  ALLOCATION_STATUS_LABEL_KEYS,
  KIND_LABEL_KEYS,
} from "@/features/extras/constants";
import {
  useConfirmAllocation,
  useEquipmentAllocations,
  useExtrasAvailability,
  useReleaseAllocation,
  useReserveEquipment,
} from "@/features/extras/hooks/use-extras";
import type {
  EquipmentAllocation,
  Extra,
  ExtrasAvailability,
  ListAllocationsParams,
} from "@/features/extras/types";
import {
  filterValues,
  joinFilter,
} from "@/features/fleet/components/shared/list-utils";
import {
  localInputFromNow,
  localInputToISO,
} from "@/features/fleet-ops/lib/datetime";
import { useBranchOptions } from "@/features/pricing/hooks/use-pricing-options";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { tenantLiveList } from "@/features/tenant/lib/live";
import { ApiError } from "@/lib/api/errors";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const FORM_ID = "equipment-reserve-form";

type ReserveValues = {
  branch_uuid: string;
  starts_at: string;
  ends_at: string;
  quantity: string;
  kind: "hold" | "reservation";
  hold_minutes: string;
  note: string;
};

type StockProblem = { stock: number; available: number; requested: number };

/** EQUIPMENT_UNAVAILABLE carries stock / available / requested in data. */
function stockProblem(error: unknown): StockProblem | null {
  if (!(error instanceof ApiError) || error.code !== "EQUIPMENT_UNAVAILABLE") {
    return null;
  }
  const body = error.body as
    (typeof error.body & { data?: StockProblem | null }) | undefined;
  return body?.data ?? { stock: 0, available: 0, requested: 0 };
}

/**
 * Timed equipment allocations of a physical extra (holds, reservations)
 * with the "Ekipman ayır" dialog, confirm / release row actions and a
 * free-units check for a branch and range. Live list.
 */
export function EquipmentAllocationsTab({ extra }: { extra: Extra }) {
  const { t, locale } = useLocale();
  const { organization } = useTenant();
  const listState = useServerListState({
    initialSort: "-starts_at",
    initialPageSize: 20,
    realtime: tenantLiveList(organization, ALLOCATIONS_RESOURCE),
  });
  const listParams = useMemo<ListAllocationsParams>(() => {
    const column = (id: string) => filterValues(listState.columnFilters, id);
    return {
      ...listState.params,
      extra: extra.uuid,
      status: joinFilter(column("status")),
      kind: joinFilter(column("kind")),
    };
  }, [extra.uuid, listState.columnFilters, listState.params]);
  const query = useEquipmentAllocations(listParams);
  const confirm = useConfirmAllocation();
  const release = useReleaseAllocation();
  const [reserveOpen, setReserveOpen] = useState(false);
  const [action, setAction] = useState<{
    kind: "confirm" | "release";
    allocation: EquipmentAllocation;
  } | null>(null);
  const hold = permissions.tenant.availability.hold;

  const columns = useMemo(
    () =>
      [
        createColumn<EquipmentAllocation>({
          id: "branch",
          accessorFn: (row) => row.branch.code,
          labelKey: "extras.allocations.columns.branch",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span>
              <code className="text-xs">{row.original.branch.code}</code>{" "}
              {row.original.branch.name}
            </span>
          ),
        }),
        createColumn<EquipmentAllocation>({
          accessorKey: "starts_at",
          labelKey: "extras.allocations.columns.starts_at",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) =>
            datetime(row.original.starts_at, undefined, locale),
        }),
        createColumn<EquipmentAllocation>({
          accessorKey: "ends_at",
          labelKey: "extras.allocations.columns.ends_at",
          enableSorting: true,
          cell: ({ row }) => datetime(row.original.ends_at, undefined, locale),
        }),
        createColumn<EquipmentAllocation>({
          accessorKey: "quantity",
          labelKey: "extras.allocations.columns.quantity",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.quantity}</span>
          ),
        }),
        createColumn<EquipmentAllocation>({
          accessorKey: "kind",
          labelKey: "extras.allocations.columns.kind",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ALLOCATION_KINDS.map((v) => ({
            value: v,
            labelKey: KIND_LABEL_KEYS[v],
            label: t(KIND_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => t(KIND_LABEL_KEYS[row.original.kind]),
        }),
        createColumn<EquipmentAllocation>({
          accessorKey: "status",
          labelKey: "extras.allocations.columns.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: ALLOCATION_STATUSES.map((v) => ({
            value: v,
            labelKey: ALLOCATION_STATUS_LABEL_KEYS[v],
            label: t(ALLOCATION_STATUS_LABEL_KEYS[v]),
          })),
          cell: ({ row }) => (
            <AllocationStatusChip
              status={row.original.status}
              lapsed={row.original.lapsed}
            />
          ),
        }),
        createColumn<EquipmentAllocation>({
          id: "expires_at",
          accessorFn: (row) => row.expires_at ?? "",
          labelKey: "extras.allocations.columns.expires_at",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.expires_at
              ? datetime(row.original.expires_at, undefined, locale)
              : "—",
        }),
        createColumn<EquipmentAllocation>({
          id: "vehicle_allocation",
          accessorFn: (row) => row.vehicle_allocation_uuid ?? "",
          labelKey: "extras.allocations.columns.vehicle_allocation",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.vehicle_allocation_uuid ? (
              <code className="text-xs">
                {row.original.vehicle_allocation_uuid.slice(0, 8)}
              </code>
            ) : (
              "—"
            ),
        }),
        createColumn<EquipmentAllocation>({
          accessorKey: "note",
          labelKey: "extras.allocations.columns.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note || "—",
        }),
        createColumn<EquipmentAllocation>({
          accessorKey: "created_at",
          labelKey: "extras.allocations.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<EquipmentAllocation>({
          id: "actions",
          labelKey: "extras.allocations.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const a = row.original;
            if (a.status !== "active") return null;
            return (
              <EntityRowActions
                actions={[
                  ...(a.kind === "hold" && !a.lapsed
                    ? [
                        {
                          id: "confirm",
                          label: t("extras.allocations.actions.confirm"),
                          icon: CheckCircle2,
                          permission: hold,
                          onSelect: () =>
                            setAction({ kind: "confirm", allocation: a }),
                        },
                      ]
                    : []),
                  {
                    id: "release",
                    label: t("extras.allocations.actions.release"),
                    icon: Undo2,
                    permission: hold,
                    variant: "destructive" as const,
                    onSelect: () =>
                      setAction({ kind: "release", allocation: a }),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<EquipmentAllocation, unknown>[],
    [hold, locale, t],
  );

  const total = query.data?.total ?? 0;
  const pageCount = Math.max(
    1,
    Math.ceil(total / (listState.pagination.pageSize || 20)),
  );

  return (
    <div className="space-y-4">
      <AvailabilityCheckCard extra={extra} />
      <EntitySectionCard
        title={t("extras.allocations.title")}
        badge={query.data ? total : undefined}
      >
        <p className="text-muted-foreground mb-4 text-sm">
          {t("extras.allocations.help")}
        </p>
        <EntityTable
          columns={columns}
          data={query.data?.items ?? []}
          getRowId={(row) => row.uuid}
          isLoading={query.isLoading}
          isError={query.isError}
          onRetry={() => void query.refetch()}
          realtime={listState.realtimeFor(() => query.refetch())}
          emptyTitle={t("extras.allocations.empty")}
          emptyDescription={t("extras.allocations.empty_description")}
          pageCount={pageCount}
          state={listState.tableState}
          features={{
            persistKey: "tenant-extra-allocations-v1",
            rowSelection: false,
          }}
          toolbarExtra={
            <>
              <PermissionGuard permission={hold}>
                <Button
                  type="button"
                  size="sm"
                  disabled={extra.status === "archived"}
                  onClick={() => setReserveOpen(true)}
                >
                  <Plus className="size-4" />
                  {t("extras.allocations.reserve")}
                </Button>
              </PermissionGuard>
              <EntityToolbar
                onRefresh={() => void query.refetch()}
                refreshDisabled={query.isFetching}
              />
            </>
          }
        />
      </EntitySectionCard>
      <ReserveDialog
        extra={extra}
        open={reserveOpen}
        onOpenChange={setReserveOpen}
      />
      <ConfirmDialog
        open={Boolean(action)}
        title={
          action?.kind === "confirm"
            ? t("extras.allocations.confirm_title")
            : t("extras.allocations.release_title")
        }
        description={
          action?.kind === "confirm"
            ? t("extras.allocations.confirm_body")
            : t("extras.allocations.release_body")
        }
        variant={action?.kind === "release" ? "destructive" : "default"}
        isPending={confirm.isPending || release.isPending}
        onCancel={() => setAction(null)}
        onConfirm={() => {
          if (!action) return;
          const done = { onSettled: () => setAction(null) };
          if (action.kind === "confirm")
            confirm.mutate(action.allocation.uuid, done);
          else release.mutate(action.allocation.uuid, done);
        }}
      />
    </div>
  );
}

/** "Ekipman ayır": hold / reserve units at a branch for a range. */
function ReserveDialog({
  extra,
  open,
  onOpenChange,
}: {
  extra: Extra;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const branches = useBranchOptions();
  const reserve = useReserveEquipment();
  const [problem, setProblem] = useState<StockProblem | null>(null);
  const schema = useMemo(
    () =>
      z
        .object({
          branch_uuid: z.string().min(1, t("extras.validation.branch")),
          starts_at: z.string().min(1, t("extras.validation.starts_at")),
          ends_at: z.string().min(1, t("extras.validation.ends_at")),
          quantity: z
            .string()
            .trim()
            .refine(
              (v) =>
                /^\d{1,3}$/.test(v) &&
                Number(v) >= 1 &&
                Number(v) <= extra.max_quantity,
              t("extras.validation.reserve_quantity", {
                max: extra.max_quantity,
              }),
            ),
          kind: z.enum(["hold", "reservation"]),
          hold_minutes: z
            .string()
            .trim()
            .refine(
              (v) => /^\d{1,4}$/.test(v) && Number(v) >= 1 && Number(v) <= 1440,
              t("extras.validation.hold_minutes"),
            ),
          note: z.string().max(2000),
        })
        .refine((v) => v.ends_at > v.starts_at, {
          path: ["ends_at"],
          message: t("extras.validation.ends_after_starts"),
        }),
    [extra.max_quantity, t],
  ) as unknown as z.ZodType<ReserveValues>;
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={close}
      title={t("extras.allocations.reserve_title")}
      description={t("extras.allocations.reserve_description", {
        name: extra.name,
      })}
      className="sm:max-w-lg"
      footer={
        <>
          <Button type="button" variant="outline" onClick={() => close(false)}>
            {t("form.cancel")}
          </Button>
          <Button type="submit" form={FORM_ID} disabled={reserve.isPending}>
            {t("extras.allocations.reserve")}
          </Button>
        </>
      }
    >
      {open ? (
        <div className="space-y-4">
          {problem ? (
            <Alert variant="destructive">
              <AlertTriangle className="size-4" />
              <AlertTitle>
                {t("extras.allocations.unavailable_title")}
              </AlertTitle>
              <AlertDescription>
                {t("extras.allocations.unavailable_body", {
                  available: problem.available,
                  stock: problem.stock,
                })}
              </AlertDescription>
            </Alert>
          ) : null}
          <AppForm<ReserveValues>
            id={FORM_ID}
            schema={schema}
            className="space-y-4"
            defaultValues={{
              branch_uuid: "",
              starts_at: localInputFromNow(24),
              ends_at: localInputFromNow(24 * 4),
              quantity: "1",
              kind: "hold",
              hold_minutes: "30",
              note: "",
            }}
            onSubmit={async (values) => {
              setProblem(null);
              try {
                await reserve.mutateAsync({
                  extra_uuid: extra.uuid,
                  branch_uuid: values.branch_uuid,
                  starts_at: localInputToISO(values.starts_at),
                  ends_at: localInputToISO(values.ends_at),
                  quantity: Number(values.quantity),
                  kind: values.kind,
                  hold_minutes:
                    values.kind === "hold"
                      ? Number(values.hold_minutes)
                      : undefined,
                  note: values.note.trim(),
                });
                close(false);
              } catch (error) {
                const p = stockProblem(error);
                if (p) {
                  setProblem(p);
                  return;
                }
                appToast.error(
                  error instanceof Error
                    ? error.message
                    : t("extras.toast.reserve_failed"),
                );
              }
            }}
          >
            {(form) => (
              <>
                <AppSelect
                  name="branch_uuid"
                  label={t("extras.allocations.fields.branch")}
                  placeholder={t(
                    "extras.allocations.fields.branch_placeholder",
                  )}
                  options={branches.options}
                />
                <div className="grid gap-4 sm:grid-cols-2">
                  <AppDatePicker
                    name="starts_at"
                    withTime
                    label={t("extras.allocations.fields.starts_at")}
                  />
                  <AppDatePicker
                    name="ends_at"
                    withTime
                    label={t("extras.allocations.fields.ends_at")}
                  />
                </div>
                <div className="grid gap-4 sm:grid-cols-3">
                  <AppInput
                    name="quantity"
                    inputMode="numeric"
                    label={t("extras.allocations.fields.quantity")}
                  />
                  <AppSelect
                    name="kind"
                    label={t("extras.allocations.fields.kind")}
                    options={(["hold", "reservation"] as const).map((v) => ({
                      value: v,
                      label: t(KIND_LABEL_KEYS[v]),
                    }))}
                  />
                  {form.watch("kind") === "hold" ? (
                    <AppInput
                      name="hold_minutes"
                      inputMode="numeric"
                      label={t("extras.allocations.fields.hold_minutes")}
                    />
                  ) : null}
                </div>
                <AppTextarea
                  name="note"
                  label={t("extras.allocations.fields.note")}
                />
              </>
            )}
          </AppForm>
        </div>
      ) : null}
    </AppDialog>
  );
}

/** Free units of this extra at a branch for a range (read-only). */
function AvailabilityCheckCard({ extra }: { extra: Extra }) {
  const { t } = useLocale();
  const branches = useBranchOptions();
  const check = useExtrasAvailability();
  const [branch, setBranch] = useState("");
  const [startsAt, setStartsAt] = useState(localInputFromNow(24));
  const [endsAt, setEndsAt] = useState(localInputFromNow(24 * 4));
  const [result, setResult] = useState<ExtrasAvailability | null>(null);
  const item = result?.items[0];
  const valid = branch !== "" && startsAt !== "" && endsAt > startsAt;

  return (
    <EntitySectionCard title={t("extras.availability.title")}>
      <p className="text-muted-foreground mb-4 text-sm">
        {t("extras.availability.help")}
      </p>
      <div className="grid gap-3 md:grid-cols-[minmax(0,14rem)_minmax(0,14rem)_minmax(0,14rem)_auto] md:items-end">
        <Select value={branch} onValueChange={setBranch}>
          <SelectTrigger
            aria-label={t("extras.allocations.fields.branch")}
            className="w-full"
          >
            <SelectValue
              placeholder={t("extras.allocations.fields.branch_placeholder")}
            />
          </SelectTrigger>
          <SelectContent>
            {branches.options.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <DatePicker
          value={startsAt}
          onChange={(v) => setStartsAt(v ?? "")}
          withTime
          placeholder={t("extras.allocations.fields.starts_at")}
        />
        <DatePicker
          value={endsAt}
          onChange={(v) => setEndsAt(v ?? "")}
          withTime
          placeholder={t("extras.allocations.fields.ends_at")}
        />
        <Button
          type="button"
          variant="outline"
          className="md:justify-self-start"
          disabled={!valid || check.isPending}
          onClick={() =>
            check.mutate(
              {
                branch_uuid: branch,
                starts_at: localInputToISO(startsAt) ?? startsAt,
                ends_at: localInputToISO(endsAt) ?? endsAt,
                extra_uuids: [extra.uuid],
              },
              { onSuccess: setResult },
            )
          }
        >
          <Search className="size-4" />
          {t("extras.availability.run")}
        </Button>
      </div>
      {item ? (
        <dl className="mt-4 grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
          {(
            [
              ["extras.availability.stock", item.stock],
              ["extras.availability.reserved", item.reserved],
              ["extras.availability.held", item.held],
              ["extras.availability.available", item.available],
            ] as const
          ).map(([labelKey, value]) => (
            <div key={labelKey} className="bg-muted/40 rounded-lg border p-3">
              <dt className="text-muted-foreground text-xs">{t(labelKey)}</dt>
              <dd className="font-display text-2xl font-semibold tabular-nums">
                {value}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
    </EntitySectionCard>
  );
}
