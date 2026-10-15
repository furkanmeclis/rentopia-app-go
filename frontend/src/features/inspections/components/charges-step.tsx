"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil } from "lucide-react";
import { useMemo, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import type { StepProps } from "@/features/inspections/components/wizard-steps";
import {
  VEHICLE_STATUSES_AFTER,
  VEHICLE_STATUS_AFTER_LABEL_KEYS,
} from "@/features/inspections/constants";
import { useInspectionCharges } from "@/features/inspections/hooks/use-inspections";
import type {
  InspectionChargeLine,
  InspectionChargeOverride,
  VehicleStatusAfter,
} from "@/features/inspections/types";
import { CHARGE_KIND_LABEL_KEYS } from "@/features/rentals/constants";
import { currency } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Return step: the charges computed from the policy snapshot (late, km,
 * fuel, missing equipment) and the damage amounts, reviewed by staff.
 * Changing a computed amount needs `price.override`; the customer's dispute
 * and where the vehicle goes next are recorded here too.
 */
export function ChargesStep({ ins, readOnly, commit }: StepProps) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const preview = useInspectionCharges(ins.uuid, true);
  const overridePerm = can(permissions.tenant.price.override);
  const [editing, setEditing] = useState<InspectionChargeLine | null>(null);
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");
  const [note, setNote] = useState(ins.dispute_note);
  const [seenNote, setSeenNote] = useState(ins.dispute_note);
  if (seenNote !== ins.dispute_note) {
    setSeenNote(ins.dispute_note);
    setNote(ins.dispute_note);
  }
  const fmt = (v: string, code: string) => {
    const n = Number(v);
    return Number.isFinite(n) ? currency(n, code || "TRY", locale) : v;
  };

  const save = async () => {
    if (!editing) return;
    const rest = ins.charge_overrides.filter((o) => o.key !== editing.key);
    const next: InspectionChargeOverride[] = [
      ...rest,
      { key: editing.key, amount: amount.replace(",", "."), reason },
    ];
    if (await commit({ charge_overrides: next })) {
      setEditing(null);
      void preview.refetch();
    }
  };

  const columns = useMemo(
    () =>
      [
        createColumn<InspectionChargeLine>({
          accessorKey: "kind",
          labelKey: "inspections.charges.columns.kind",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1.5">
              <span className="font-medium">
                {t(CHARGE_KIND_LABEL_KEYS[row.original.kind])}
              </span>
              {row.original.disputed ? (
                <Badge variant="danger">
                  {t("inspections.charges.disputed")}
                </Badge>
              ) : null}
              {row.original.overridden ? (
                <Badge variant="outline">
                  {t("inspections.charges.overridden")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<InspectionChargeLine>({
          accessorKey: "amount",
          labelKey: "inspections.charges.columns.amount",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="flex flex-col tabular-nums">
              <span className="font-medium">
                {fmt(row.original.amount, row.original.currency)}
              </span>
              {row.original.computed &&
              row.original.computed !== row.original.amount ? (
                <span className="text-muted-foreground text-xs">
                  {t("inspections.charges.computed", {
                    amount: fmt(row.original.computed, row.original.currency),
                  })}
                </span>
              ) : null}
            </span>
          ),
        }),
        createColumn<InspectionChargeLine>({
          accessorKey: "reason",
          labelKey: "inspections.charges.columns.reason",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="line-clamp-2 text-sm">{row.original.reason}</span>
          ),
        }),
        createColumn<InspectionChargeLine>({
          id: "actions",
          labelKey: "inspections.charges.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) => {
            const line = row.original;
            const computed = line.source === "policy";
            if (readOnly || (computed && !overridePerm)) return null;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("inspections.charges.edit"),
                    icon: Pencil,
                    onSelect: () => {
                      setEditing(line);
                      setAmount(line.amount);
                      setReason(
                        ins.charge_overrides.find((o) => o.key === line.key)
                          ?.reason ?? "",
                      );
                    },
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<InspectionChargeLine, unknown>[],
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fmt follows locale
    [ins.charge_overrides, locale, overridePerm, readOnly, t],
  );

  const data = preview.data;
  return (
    <div className="grid gap-4">
      <EntitySectionCard
        title={t("inspections.charges.title")}
        badge={data ? fmt(data.total, data.currency) : undefined}
      >
        {data ? (
          <p className="text-muted-foreground mb-3 text-sm">
            {t("inspections.charges.summary", {
              late: data.late_minutes,
              km: data.driven_km,
              version: data.settings_version,
            })}
          </p>
        ) : null}
        <EntityTable
          columns={columns}
          data={data?.lines ?? []}
          getRowId={(row) => row.key}
          isLoading={preview.isLoading}
          isError={preview.isError}
          onRetry={() => void preview.refetch()}
          emptyTitle={t("inspections.charges.empty")}
          emptyDescription={t("inspections.charges.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-inspection-charges-v1",
            rowSelection: false,
            globalFilter: false,
            sorting: false,
          }}
        />
      </EntitySectionCard>
      <EntitySectionCard title={t("inspections.charges.dispute_title")}>
        <div className="grid gap-4">
          <div className="flex items-center justify-between gap-3">
            <Label htmlFor="disputed">
              {t("inspections.charges.disputes")}
            </Label>
            <Switch
              id="disputed"
              checked={ins.disputed}
              disabled={readOnly}
              onCheckedChange={(v) => void commit({ disputed: v })}
            />
          </div>
          {ins.disputed ? (
            <div className="grid gap-1.5">
              <Label htmlFor="dispute-note">
                {t("inspections.charges.dispute_note")}
              </Label>
              <Textarea
                id="dispute-note"
                rows={2}
                value={note}
                disabled={readOnly}
                onChange={(e) => setNote(e.target.value)}
                onBlur={() => {
                  if (note !== ins.dispute_note) {
                    void commit({ dispute_note: note });
                  }
                }}
              />
              <span className="text-muted-foreground text-xs">
                {t("inspections.charges.dispute_hint")}
              </span>
            </div>
          ) : null}
          <div className="grid gap-1.5">
            <Label>{t("inspections.charges.vehicle_after")}</Label>
            <Select
              value={ins.vehicle_status_after ?? "cleaning"}
              disabled={readOnly}
              onValueChange={(v) =>
                void commit({ vehicle_status_after: v as VehicleStatusAfter })
              }
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {VEHICLE_STATUSES_AFTER.map((v) => (
                  <SelectItem key={v} value={v}>
                    {t(VEHICLE_STATUS_AFTER_LABEL_KEYS[v])}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        </div>
      </EntitySectionCard>
      <AppDialog
        open={Boolean(editing)}
        onOpenChange={(next) => {
          if (!next) setEditing(null);
        }}
        title={t("inspections.charges.edit_title")}
        description={
          editing
            ? `${t(CHARGE_KIND_LABEL_KEYS[editing.kind])} · ${editing.reason}`
            : undefined
        }
        className="sm:max-w-md"
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setEditing(null)}
            >
              {t("form.cancel")}
            </Button>
            <Button
              type="button"
              disabled={
                !/^\d+([.,]\d{1,2})?$/.test(amount.trim()) ||
                (editing?.source === "policy" && reason.trim().length < 3)
              }
              onClick={() => void save()}
            >
              {t("inspections.charges.save")}
            </Button>
          </>
        }
      >
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="charge-amount">
              {t("inspections.charges.columns.amount")}
            </Label>
            <Input
              id="charge-amount"
              inputMode="decimal"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="charge-reason">
              {t("inspections.charges.reason")}
            </Label>
            <Textarea
              id="charge-reason"
              rows={2}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
        </div>
      </AppDialog>
    </div>
  );
}
