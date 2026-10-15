"use client";

import { CheckCircle2, Clock, Trash2 } from "lucide-react";

import { EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  CaptureButton,
  PhotoThumb,
  mediaUrl,
} from "@/features/inspections/components/photo-capture";
import {
  type QueueView,
  useDeletePhoto,
  useMediaLinks,
} from "@/features/inspections/hooks/use-inspections";
import { inspectionProblemText } from "@/features/inspections/lib/errors";
import type { Inspection, InspectionPhoto } from "@/features/inspections/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function linkVersion(photos: InspectionPhoto[]) {
  return photos
    .filter((p) => p.status === "completed")
    .map((p) => p.uuid)
    .sort()
    .join(",");
}

/** Short-lived links of a form's completed photos (uuid → URL). */
export function usePhotoLinks(
  uuid: string | null | undefined,
  photos: InspectionPhoto[],
) {
  const query = useMediaLinks(uuid ?? null, linkVersion(photos));
  return query.data ?? {};
}

/**
 * Step 5: the template's required photos (a return shows the handover
 * photo of the same slot next to the new one). A slot is done when one of
 * its photos is uploaded and verified by the server.
 */
export function PhotosStep({
  ins,
  readOnly,
  queue,
  onPhoto,
}: {
  ins: Inspection;
  readOnly: boolean;
  queue: QueueView[];
  onPhoto: (file: File, slot: string) => void;
}) {
  const { t } = useLocale();
  const links = usePhotoLinks(ins.uuid, ins.photos);
  const baseLinks = usePhotoLinks(
    ins.baseline?.uuid,
    ins.baseline?.photos ?? [],
  );
  const remove = useDeletePhoto(ins.uuid);
  const slots = [
    ...ins.template.photo_slots,
    { key: "extra", label: t("inspections.photos.extra"), required: false },
  ];
  const done =
    ins.readiness.required_slots.length - ins.readiness.missing_slots.length;
  return (
    <EntitySectionCard
      title={t("inspections.photos.title")}
      badge={`${done}/${ins.readiness.required_slots.length}`}
    >
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {slots.map((slot) => {
          const photos = ins.photos.filter((p) => p.slot === slot.key);
          const completed = photos.filter((p) => p.status === "completed");
          const latest = completed[completed.length - 1];
          const pending = queue.filter(
            (q) => q.slot === slot.key && q.status !== "done",
          );
          const local = pending[pending.length - 1];
          const base = ins.baseline?.photos.filter(
            (p) => p.slot === slot.key && p.status === "completed",
          );
          const baseLatest = base?.[base.length - 1];
          const ok = completed.length > 0;
          return (
            <div
              key={slot.key}
              data-slot={slot.key}
              className={cn(
                "grid gap-2 rounded-md border p-3",
                slot.required && !ok && "border-amber-500/60",
              )}
            >
              <div className="flex items-center justify-between gap-2">
                <span className="font-medium">{slot.label}</span>
                {ok ? (
                  <Badge variant="success">
                    <CheckCircle2 className="size-3" />
                    {t("inspections.photos.done")}
                  </Badge>
                ) : pending.length ? (
                  <Badge variant="warning">
                    <Clock className="size-3" />
                    {t("inspections.photos.in_queue")}
                  </Badge>
                ) : slot.required ? (
                  <Badge variant="danger">
                    {t("inspections.photos.required")}
                  </Badge>
                ) : null}
              </div>
              <div
                className={cn(
                  "grid gap-2",
                  ins.baseline ? "grid-cols-2" : "grid-cols-1",
                )}
              >
                {ins.baseline ? (
                  <div className="grid gap-1">
                    <span className="text-muted-foreground text-xs">
                      {t("inspections.photos.at_handover")}
                    </span>
                    <PhotoThumb
                      src={
                        baseLatest ? mediaUrl(baseLinks[baseLatest.uuid]) : null
                      }
                      alt={`${slot.label} · ${t("inspections.photos.at_handover")}`}
                    />
                  </div>
                ) : null}
                <div className="grid gap-1">
                  {ins.baseline ? (
                    <span className="text-muted-foreground text-xs">
                      {t("inspections.photos.now")}
                    </span>
                  ) : null}
                  <PhotoThumb
                    src={
                      local?.previewUrl ??
                      (latest ? mediaUrl(links[latest.uuid]) : null)
                    }
                    alt={slot.label}
                  />
                </div>
              </div>
              {latest ? (
                <p className="text-muted-foreground truncate font-mono text-[10px]">
                  SHA-256 {latest.sha256.slice(0, 16)}…
                </p>
              ) : null}
              {!readOnly ? (
                <div className="flex flex-wrap items-center gap-2">
                  <CaptureButton
                    label={
                      ok
                        ? t("inspections.photos.retake")
                        : t("inspections.photos.take")
                    }
                    variant={ok ? "outline" : "default"}
                    testId={`photo-input-${slot.key}`}
                    onFile={(file) => onPhoto(file, slot.key)}
                  />
                  {latest ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      aria-label={t("inspections.photos.remove")}
                      onClick={() =>
                        remove.mutate(latest.uuid, {
                          onError: (e) =>
                            appToast.error(inspectionProblemText(t, e)),
                        })
                      }
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  ) : null}
                </div>
              ) : null}
            </div>
          );
        })}
      </div>
    </EntitySectionCard>
  );
}
