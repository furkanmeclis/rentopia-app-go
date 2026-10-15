"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { ClipboardCheck, FilePen, KeyRound, PenLine } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  PhotoThumb,
  mediaUrl,
} from "@/features/inspections/components/photo-capture";
import { usePhotoLinks } from "@/features/inspections/components/photos-step";
import { VehicleDiagram } from "@/features/inspections/components/vehicle-diagram";
import {
  DAMAGE_TYPE_LABEL_KEYS,
  KIND_LABEL_KEYS,
  REVIEW_LABEL_KEYS,
  REVIEW_TONES,
  SEVERITY_LABEL_KEYS,
  SIGNATURE_LABEL_KEYS,
  STATUS_LABEL_KEYS,
  STATUS_TONES,
  VEHICLE_STATUSES_AFTER,
  VEHICLE_STATUS_AFTER_LABEL_KEYS,
  zoneLabel,
} from "@/features/inspections/constants";
import {
  useInspection,
  useReviewInspection,
  useReviseInspection,
} from "@/features/inspections/hooks/use-inspections";
import type {
  Inspection,
  InspectionDamage,
  VehicleStatusAfter,
} from "@/features/inspections/types";
import { ReasonDialog } from "@/features/reservations/components/reason-dialog";
import { actorName } from "@/features/reservations/components/reservation-cells";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Handover vs return, slot by slot (same template slots). */
function ComparisonCard({ ins }: { ins: Inspection }) {
  const { t } = useLocale();
  const links = usePhotoLinks(ins.uuid, ins.photos);
  const baseLinks = usePhotoLinks(
    ins.baseline?.uuid,
    ins.baseline?.photos ?? [],
  );
  const base = ins.baseline;
  const latest = (photos: Inspection["photos"], slot: string) =>
    photos.filter((p) => p.slot === slot && p.status === "completed").at(-1);
  const slots = [
    ...ins.template.photo_slots,
    { key: "damage", label: t("inspections.photos.damage"), required: false },
  ];
  return (
    <EntitySectionCard
      title={
        base
          ? t("inspections.detail.comparison")
          : t("inspections.detail.photos")
      }
    >
      {base ? (
        <EntityDetail
          className="mb-4"
          sections={[
            {
              id: "readings",
              fields: [
                {
                  key: "km",
                  label: t("inspections.readings.km"),
                  value: `${base.km ?? "—"} → ${ins.km ?? "—"}${
                    base.km != null && ins.km != null
                      ? ` (${ins.km - base.km} km)`
                      : ""
                  }`,
                },
                {
                  key: "fuel",
                  label: t("inspections.readings.fuel"),
                  value: `%${base.fuel ?? "—"} → %${ins.fuel ?? "—"}`,
                },
                {
                  key: "missing",
                  label: t("inspections.review_step.missing_equipment"),
                  value:
                    ins.template.checklist
                      .filter(
                        (i) =>
                          base.equipment[i.key] &&
                          ins.equipment[i.key] === false,
                      )
                      .map((i) => i.label)
                      .join(", ") || t("inspections.review_step.none"),
                },
              ],
            },
          ]}
        />
      ) : null}
      <div className="grid gap-4 sm:grid-cols-2">
        {slots.map((slot) => {
          const now = latest(ins.photos, slot.key);
          const then = base ? latest(base.photos, slot.key) : undefined;
          if (!now && !then) return null;
          return (
            <div key={slot.key} className="grid gap-2" data-compare={slot.key}>
              <span className="text-sm font-medium">{slot.label}</span>
              <div className={base ? "grid grid-cols-2 gap-2" : "grid"}>
                {base ? (
                  <div className="grid gap-1">
                    <span className="text-muted-foreground text-xs">
                      {t("inspections.photos.at_handover")}
                    </span>
                    <PhotoThumb
                      src={then ? mediaUrl(baseLinks[then.uuid]) : null}
                      alt={`${slot.label} · ${t("inspections.photos.at_handover")}`}
                    />
                  </div>
                ) : null}
                <div className="grid gap-1">
                  {base ? (
                    <span className="text-muted-foreground text-xs">
                      {t(KIND_LABEL_KEYS[ins.kind])}
                    </span>
                  ) : null}
                  <PhotoThumb
                    src={now ? mediaUrl(links[now.uuid]) : null}
                    alt={slot.label}
                  />
                </div>
              </div>
              {now ? (
                <p className="text-muted-foreground text-[11px] break-all">
                  {t("inspections.detail.evidence_line", {
                    at: now.uploaded_at
                      ? datetime(now.uploaded_at, "d MMM HH:mm")
                      : "—",
                    hash: now.sha256.slice(0, 12),
                  })}
                  {now.exif.taken_at || now.exif.taken_local
                    ? ` · ${t("inspections.detail.exif_time", {
                        at: now.exif.taken_at
                          ? datetime(now.exif.taken_at, "d MMM HH:mm")
                          : now.exif.taken_local,
                      })}`
                    : ""}
                </p>
              ) : null}
            </div>
          );
        })}
      </div>
    </EntitySectionCard>
  );
}

function DamagesCard({ ins }: { ins: Inspection }) {
  const { t } = useLocale();
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
              {zoneLabel(t, row.original.zone)}
              {ins.kind === "return" && row.original.is_new ? (
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
          cell: ({ row }) => row.original.description || "—",
        }),
      ] as ColumnDef<InspectionDamage, unknown>[],
    [ins.kind, t],
  );
  return (
    <EntitySectionCard
      title={t("inspections.damages.title")}
      badge={ins.damages.length}
    >
      <div className="grid gap-4 md:grid-cols-[260px_1fr]">
        <VehicleDiagram
          damages={ins.damages}
          baseline={ins.baseline?.damages ?? []}
          disabled
        />
        <EntityTable
          columns={columns}
          data={ins.damages}
          getRowId={(row) => row.uuid}
          emptyTitle={t("inspections.damages.empty")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          features={{
            persistKey: "tenant-inspection-detail-damages-v1",
            rowSelection: false,
            globalFilter: false,
            sorting: false,
          }}
        />
      </div>
    </EntitySectionCard>
  );
}

/** Inspection detail: evidence, comparison, review queue actions. */
export function InspectionDetailPage({ uuid }: { uuid: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const router = useRouter();
  const query = useInspection(uuid);
  const review = useReviewInspection(uuid);
  const revise = useReviseInspection(uuid);
  const [reviewOpen, setReviewOpen] = useState(false);
  const [reviseOpen, setReviseOpen] = useState(false);
  const [note, setNote] = useState("");
  const [vehicleStatus, setVehicleStatus] =
    useState<VehicleStatusAfter>("cleaning");
  const ins = query.data;
  const closePerm = can(permissions.tenant.rentals.close);
  const waivePerm = can(permissions.tenant.signature.waive);

  if (query.isLoading) return <Loading />;
  if (!ins) {
    return (
      <ErrorState
        title={t("inspections.detail.not_found")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  const pending = ins.review_status === "pending";
  const unsigned = ins.signature_status !== "signed";
  const wizard =
    ins.kind === "handover"
      ? routes.tenant.field.handover(slug, ins.rental.uuid)
      : routes.tenant.field.return(slug, ins.rental.uuid);
  const at = (v: string | null) =>
    v ? datetime(v, "d MMM yyyy HH:mm", locale) : "—";

  return (
    <EntityPage
      title={`${t(KIND_LABEL_KEYS[ins.kind])} · ${ins.rental.reference}`}
      permission={permissions.tenant.rentals.read}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("inspections.list.title"),
          href: routes.tenant.inspections.root(slug),
        },
        { label: ins.rental.reference },
      ]}
    >
      <div className="grid gap-4">
        <EntityHeader
          title={`${ins.rental.vehicle.plate} · ${ins.rental.customer.display_name}`}
          badges={
            <span className="flex flex-wrap gap-1.5">
              <StatusChip
                label={t(STATUS_LABEL_KEYS[ins.status])}
                tone={STATUS_TONES[ins.status]}
              />
              <StatusChip
                label={t(REVIEW_LABEL_KEYS[ins.review_status])}
                tone={REVIEW_TONES[ins.review_status]}
              />
              <Badge variant="outline">v{ins.version}</Badge>
            </span>
          }
        >
          <div className="flex flex-wrap gap-2">
            <Button asChild variant="outline" size="sm">
              <Link href={routes.tenant.rentals.detail(slug, ins.rental.uuid)}>
                <KeyRound className="size-4" />
                {t("inspections.complete.open_rental")}
              </Link>
            </Button>
            {ins.status === "draft" ? (
              <Button asChild size="sm">
                <Link href={wizard}>
                  <PenLine className="size-4" />
                  {t("inspections.detail.continue")}
                </Link>
              </Button>
            ) : null}
            {pending && closePerm ? (
              <Button
                size="sm"
                data-testid="open-review"
                onClick={() => {
                  setNote("");
                  setVehicleStatus(ins.vehicle_status_after ?? "cleaning");
                  setReviewOpen(true);
                }}
              >
                <ClipboardCheck className="size-4" />
                {t("inspections.detail.resolve")}
              </Button>
            ) : null}
            {ins.status === "finalized" && closePerm ? (
              <Button
                size="sm"
                variant="outline"
                onClick={() => setReviseOpen(true)}
              >
                <FilePen className="size-4" />
                {t("inspections.detail.revise")}
              </Button>
            ) : null}
          </div>
        </EntityHeader>

        {pending ? (
          <Alert variant="destructive">
            <AlertTitle>{t("inspections.detail.review_title")}</AlertTitle>
            <AlertDescription>
              {t("inspections.detail.review_hint")}
            </AlertDescription>
          </Alert>
        ) : null}

        <EntitySectionCard title={t("inspections.detail.facts")}>
          <EntityDetail
            sections={[
              {
                id: "facts",
                fields: [
                  {
                    key: "occurred",
                    label: t("inspections.detail.occurred_at"),
                    value: at(ins.occurred_at),
                  },
                  {
                    key: "finalized",
                    label: t("inspections.detail.finalized_at"),
                    value: `${at(ins.finalized_at)}${ins.finalized_by ? ` · ${actorName(ins.finalized_by)}` : ""}`,
                  },
                  {
                    key: "km",
                    label: t("inspections.readings.km"),
                    value: ins.km ?? "—",
                  },
                  {
                    key: "fuel",
                    label: t("inspections.readings.fuel"),
                    value: ins.fuel != null ? `%${ins.fuel}` : "—",
                  },
                  {
                    key: "signature",
                    label: t("inspections.list.columns.signature"),
                    value: `${t(SIGNATURE_LABEL_KEYS[ins.signature_status])}${ins.signature_note ? ` — ${ins.signature_note}` : ""}`,
                  },
                  {
                    key: "disputed",
                    label: t("inspections.list.disputed"),
                    value: ins.disputed
                      ? ins.dispute_note || t("inspections.detail.yes")
                      : t("inspections.detail.no"),
                  },
                  {
                    key: "statement",
                    label: t("inspections.review_step.statement"),
                    value: ins.customer_statement || "—",
                  },
                  {
                    key: "revision",
                    label: t("inspections.detail.revision_reason"),
                    value: ins.revision_reason || "—",
                  },
                  {
                    key: "review_note",
                    label: t("inspections.detail.review_note"),
                    value: ins.review_note
                      ? `${ins.review_note} · ${actorName(ins.reviewed_by)}`
                      : "—",
                  },
                ],
              },
            ]}
          />
        </EntitySectionCard>

        <ComparisonCard ins={ins} />
        <DamagesCard ins={ins} />
      </div>

      <AppDialog
        open={reviewOpen}
        onOpenChange={(open) => {
          if (!open && !review.isPending) setReviewOpen(false);
        }}
        title={t("inspections.detail.resolve")}
        description={
          unsigned
            ? t("inspections.detail.resolve_unsigned")
            : t("inspections.detail.resolve_hint")
        }
        className="sm:max-w-md"
        footer={
          <>
            <Button variant="outline" onClick={() => setReviewOpen(false)}>
              {t("form.cancel")}
            </Button>
            <Button
              data-testid="confirm-review"
              disabled={
                review.isPending ||
                note.trim().length < 3 ||
                (unsigned && !waivePerm)
              }
              onClick={() =>
                review.mutate(
                  { note: note.trim(), vehicle_status: vehicleStatus },
                  { onSuccess: () => setReviewOpen(false) },
                )
              }
            >
              {t("inspections.detail.resolve")}
            </Button>
          </>
        }
      >
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="review-note">
              {t("inspections.detail.review_note")}
            </Label>
            <Textarea
              id="review-note"
              rows={3}
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          <div className="grid gap-1.5">
            <Label>{t("inspections.charges.vehicle_after")}</Label>
            <Select
              value={vehicleStatus}
              onValueChange={(v) => setVehicleStatus(v as VehicleStatusAfter)}
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
          {unsigned && !waivePerm ? (
            <span className="text-destructive text-xs">
              {t("inspections.detail.waive_needed")}
            </span>
          ) : null}
        </div>
      </AppDialog>
      <ReasonDialog
        open={reviseOpen}
        title={t("inspections.detail.revise")}
        description={t("inspections.detail.revise_hint")}
        label={t("inspections.detail.revision_reason")}
        confirmLabel={t("inspections.detail.revise")}
        pending={revise.isPending}
        onCancel={() => setReviseOpen(false)}
        onConfirm={(reason) =>
          revise.mutate(reason, {
            onSuccess: () => {
              setReviseOpen(false);
              router.push(wizard);
            },
          })
        }
      />
    </EntityPage>
  );
}
