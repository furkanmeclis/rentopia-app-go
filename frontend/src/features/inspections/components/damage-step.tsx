"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Armchair, Trash2 } from "lucide-react";
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
import { CaptureButton } from "@/features/inspections/components/photo-capture";
import { VehicleDiagram } from "@/features/inspections/components/vehicle-diagram";
import {
  DAMAGE_TYPES,
  DAMAGE_TYPE_LABEL_KEYS,
  SEVERITIES,
  SEVERITY_LABEL_KEYS,
  type Zone,
  zoneLabel,
} from "@/features/inspections/constants";
import { useDamageActions } from "@/features/inspections/hooks/use-inspections";
import { inspectionProblemText } from "@/features/inspections/lib/errors";
import type {
  DamageSeverity,
  DamageType,
  Inspection,
  InspectionDamage,
} from "@/features/inspections/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Draft = {
  zone: Zone;
  damage_type: DamageType;
  severity: DamageSeverity;
  description: string;
  is_new: boolean;
  disputed: boolean;
};

/**
 * Step 4: damages on the vehicle diagram — pick a zone, describe it, then
 * photograph it (every damage needs a completed photo).
 */
export function DamageStep({
  ins,
  readOnly,
  onPhoto,
}: {
  ins: Inspection;
  readOnly: boolean;
  onPhoto: (file: File, damageUuid: string) => void;
}) {
  const { t } = useLocale();
  const actions = useDamageActions(ins.uuid);
  const [draft, setDraft] = useState<Draft | null>(null);
  const isReturn = ins.kind === "return";
  const photoCount = (damage: string) =>
    ins.photos.filter((p) => p.damage_uuid === damage).length;

  const open = (zone: Zone) =>
    setDraft({
      zone,
      damage_type: "scratch",
      severity: "minor",
      description: "",
      is_new: isReturn,
      disputed: false,
    });

  const columns = useMemo(
    () =>
      [
        createColumn<InspectionDamage>({
          accessorKey: "zone",
          labelKey: "inspections.damages.columns.zone",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1.5">
              <span className="font-medium">
                {zoneLabel(t, row.original.zone)}
              </span>
              {isReturn && row.original.is_new ? (
                <Badge variant="danger">{t("inspections.damages.new")}</Badge>
              ) : null}
              {row.original.disputed ? (
                <Badge variant="warning">
                  {t("inspections.damages.disputed")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<InspectionDamage>({
          accessorKey: "damage_type",
          labelKey: "inspections.damages.columns.type",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            `${t(DAMAGE_TYPE_LABEL_KEYS[row.original.damage_type])} · ${t(SEVERITY_LABEL_KEYS[row.original.severity])}`,
        }),
        createColumn<InspectionDamage>({
          accessorKey: "description",
          labelKey: "inspections.damages.columns.description",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="line-clamp-2 text-sm">
              {row.original.description || "—"}
            </span>
          ),
        }),
        createColumn<InspectionDamage>({
          id: "photos",
          labelKey: "inspections.damages.columns.photos",
          enableSorting: false,
          cell: ({ row }) => {
            const n = photoCount(row.original.uuid);
            return (
              <span className="flex items-center gap-2">
                <Badge variant={n ? "success" : "danger"}>{n}</Badge>
                {!readOnly ? (
                  <CaptureButton
                    label={t("inspections.damages.add_photo")}
                    testId={`damage-photo-${row.original.uuid}`}
                    onFile={(file) => onPhoto(file, row.original.uuid)}
                  />
                ) : null}
              </span>
            );
          },
        }),
        createColumn<InspectionDamage>({
          id: "actions",
          labelKey: "inspections.damages.columns.actions",
          enableSorting: false,
          enableHiding: false,
          cell: ({ row }) =>
            readOnly ? null : (
              <EntityRowActions
                actions={[
                  {
                    id: "delete",
                    label: t("inspections.damages.remove"),
                    icon: Trash2,
                    variant: "destructive" as const,
                    onSelect: () =>
                      actions.remove.mutate(row.original.uuid, {
                        onError: (e) =>
                          appToast.error(inspectionProblemText(t, e)),
                      }),
                  },
                ]}
              />
            ),
        }),
      ] as ColumnDef<InspectionDamage, unknown>[],
    // eslint-disable-next-line react-hooks/exhaustive-deps -- photo counts follow ins
    [actions.remove, ins.photos, isReturn, onPhoto, readOnly, t],
  );

  return (
    <div className="grid gap-4">
      <EntitySectionCard
        title={t("inspections.damages.title")}
        badge={ins.damages.length}
      >
        <p className="text-muted-foreground mb-3 text-sm">
          {t(
            isReturn
              ? "inspections.damages.hint_return"
              : "inspections.damages.hint",
          )}
        </p>
        <VehicleDiagram
          damages={ins.damages}
          baseline={ins.baseline?.damages ?? []}
          selected={draft?.zone ?? null}
          disabled={readOnly}
          onSelect={open}
        />
        {!readOnly ? (
          <div className="mt-3 flex justify-center">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => open("interior")}
            >
              <Armchair className="size-4" />
              {t("inspections.zones.interior")}
            </Button>
          </div>
        ) : null}
      </EntitySectionCard>
      <EntityTable
        columns={columns}
        data={ins.damages}
        getRowId={(row) => row.uuid}
        emptyTitle={t("inspections.damages.empty")}
        emptyDescription={t("inspections.damages.empty_description")}
        manual={{ filtering: false, sorting: false, pagination: false }}
        features={{
          persistKey: "tenant-inspection-damages-v1",
          rowSelection: false,
          globalFilter: false,
          sorting: false,
        }}
      />
      <AppDialog
        open={Boolean(draft)}
        onOpenChange={(next) => {
          if (!next && !actions.add.isPending) setDraft(null);
        }}
        title={draft ? zoneLabel(t, draft.zone) : ""}
        description={t("inspections.damages.dialog_description")}
        className="sm:max-w-md"
        footer={
          <>
            <Button
              type="button"
              variant="outline"
              onClick={() => setDraft(null)}
            >
              {t("form.cancel")}
            </Button>
            <Button
              type="button"
              disabled={actions.add.isPending}
              onClick={() => {
                if (!draft) return;
                actions.add.mutate(
                  {
                    zone: draft.zone,
                    damage_type: draft.damage_type,
                    severity: draft.severity,
                    description: draft.description.trim(),
                    is_new: isReturn ? draft.is_new : false,
                    disputed: isReturn ? draft.disputed : false,
                  },
                  {
                    onSuccess: () => {
                      setDraft(null);
                      appToast.success(t("inspections.damages.added"));
                    },
                    onError: (e) => appToast.error(inspectionProblemText(t, e)),
                  },
                );
              }}
            >
              {t("inspections.damages.save")}
            </Button>
          </>
        }
      >
        {draft ? (
          <div className="grid gap-4">
            <div className="grid gap-1.5">
              <Label>{t("inspections.damages.columns.type")}</Label>
              <Select
                value={draft.damage_type}
                onValueChange={(v) =>
                  setDraft({ ...draft, damage_type: v as DamageType })
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {DAMAGE_TYPES.map((v) => (
                    <SelectItem key={v} value={v}>
                      {t(DAMAGE_TYPE_LABEL_KEYS[v])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label>{t("inspections.damages.severity")}</Label>
              <Select
                value={draft.severity}
                onValueChange={(v) =>
                  setDraft({ ...draft, severity: v as DamageSeverity })
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {SEVERITIES.map((v) => (
                    <SelectItem key={v} value={v}>
                      {t(SEVERITY_LABEL_KEYS[v])}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="damage-description">
                {t("inspections.damages.columns.description")}
              </Label>
              <Textarea
                id="damage-description"
                rows={2}
                value={draft.description}
                onChange={(e) =>
                  setDraft({ ...draft, description: e.target.value })
                }
              />
            </div>
            {isReturn ? (
              <>
                <div className="flex items-center justify-between gap-3">
                  <Label htmlFor="damage-new">
                    {t("inspections.damages.is_new")}
                  </Label>
                  <Switch
                    id="damage-new"
                    checked={draft.is_new}
                    onCheckedChange={(v) => setDraft({ ...draft, is_new: v })}
                  />
                </div>
                <div className="flex items-center justify-between gap-3">
                  <Label htmlFor="damage-disputed">
                    {t("inspections.damages.customer_disputes")}
                  </Label>
                  <Switch
                    id="damage-disputed"
                    checked={draft.disputed}
                    onCheckedChange={(v) => setDraft({ ...draft, disputed: v })}
                  />
                </div>
              </>
            ) : null}
          </div>
        ) : null}
      </AppDialog>
    </div>
  );
}
