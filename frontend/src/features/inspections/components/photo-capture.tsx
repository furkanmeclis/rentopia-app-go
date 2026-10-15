"use client";

import {
  AlertTriangle,
  Camera,
  CheckCircle2,
  CloudOff,
  ImageOff,
  Loader2,
  RotateCw,
  Trash2,
  UploadCloud,
} from "lucide-react";
import { useRef } from "react";

import { EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { apiConfig } from "@/config/api";
import { MAX_PHOTO_BYTES, PHOTO_TYPES } from "@/features/inspections/constants";
import type { QueueView } from "@/features/inspections/hooks/use-inspections";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { appToast } from "@/providers/toast-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** API-owned media link → same-origin URL (BFF prefix). */
export function mediaUrl(url: string | undefined | null): string | null {
  const safe = assertServiceMediaURL(url);
  if (!safe) return null;
  return safe.startsWith("/v1/")
    ? `${apiConfig.baseUrl.replace(/\/$/, "")}${safe}`
    : safe;
}

/** A photo from a short-lived API link, or a local preview of the queue. */
export function PhotoThumb({
  src,
  alt,
  className,
}: {
  src: string | null | undefined;
  alt: string;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "bg-muted text-muted-foreground relative flex aspect-[4/3] w-full items-center justify-center overflow-hidden rounded-md",
        className,
      )}
    >
      {src ? (
        // eslint-disable-next-line @next/next/no-img-element -- API media link or local blob preview
        <img src={src} alt={alt} className="size-full object-cover" />
      ) : (
        <ImageOff className="size-6" aria-hidden />
      )}
    </div>
  );
}

/**
 * Camera / file button: images only, checked before they enter the
 * device queue (the API checks again from the bytes).
 */
export function CaptureButton({
  label,
  onFile,
  disabled,
  variant = "outline",
  testId,
}: {
  label: string;
  onFile: (file: File) => void;
  disabled?: boolean;
  variant?: "outline" | "default" | "secondary";
  testId?: string;
}) {
  const { t } = useLocale();
  const ref = useRef<HTMLInputElement>(null);
  return (
    <>
      <input
        ref={ref}
        type="file"
        accept={PHOTO_TYPES.join(",")}
        capture="environment"
        className="sr-only"
        data-testid={testId}
        disabled={disabled}
        onChange={(event) => {
          const file = event.target.files?.[0];
          event.target.value = "";
          if (!file) return;
          const type = file.type || "image/jpeg";
          if (!(PHOTO_TYPES as readonly string[]).includes(type)) {
            appToast.error(t("inspections.photos.invalid_type"));
            return;
          }
          if (file.size > MAX_PHOTO_BYTES) {
            appToast.error(t("inspections.photos.too_large"));
            return;
          }
          onFile(file);
        }}
      />
      <Button
        type="button"
        size="sm"
        variant={variant}
        disabled={disabled}
        onClick={() => ref.current?.click()}
      >
        <Camera className="size-4" />
        {label}
      </Button>
    </>
  );
}

const STATUS_ICON = {
  queued: CloudOff,
  uploading: Loader2,
  completing: UploadCloud,
  done: CheckCircle2,
  failed: AlertTriangle,
} as const;

const STATUS_KEYS = {
  queued: "inspections.queue.statuses.queued",
  uploading: "inspections.queue.statuses.uploading",
  completing: "inspections.queue.statuses.completing",
  done: "inspections.queue.statuses.done",
  failed: "inspections.queue.statuses.failed",
} as const;

/**
 * The device upload queue: what is stored on the device, uploading,
 * waiting for the server check, done or failed (retry).
 */
export function UploadQueuePanel({
  items,
  slotLabel,
  onRetry,
  onRemove,
}: {
  items: QueueView[];
  slotLabel: (slot: string) => string;
  onRetry: () => void;
  onRemove: (id: string) => void;
}) {
  const { t } = useLocale();
  const open = items.filter((i) => i.status !== "done");
  const failed = items.filter((i) => i.status === "failed").length;
  if (items.length === 0) return null;
  return (
    <EntitySectionCard
      title={t("inspections.queue.title")}
      badge={open.length}
      actions={
        failed > 0 ? (
          <Button type="button" size="sm" variant="outline" onClick={onRetry}>
            <RotateCw className="size-4" />
            {t("inspections.queue.retry")}
          </Button>
        ) : null
      }
    >
      <ul className="divide-y" data-testid="upload-queue">
        {items.map((item) => {
          const Icon = STATUS_ICON[item.status];
          return (
            <li
              key={item.id}
              className="flex items-center gap-3 py-2"
              data-status={item.status}
            >
              <PhotoThumb
                src={item.previewUrl}
                alt={slotLabel(item.slot)}
                className="w-14 shrink-0"
              />
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">
                  {slotLabel(item.slot)}
                </p>
                <p className="text-muted-foreground flex items-center gap-1 text-xs">
                  <Icon
                    className={cn(
                      "size-3.5",
                      item.status === "uploading" && "animate-spin",
                      item.status === "failed" && "text-destructive",
                      item.status === "done" && "text-emerald-600",
                    )}
                    aria-hidden
                  />
                  {t(STATUS_KEYS[item.status])}
                  {item.error ? ` · ${item.error}` : ""}
                </p>
              </div>
              <Badge variant="outline" className="shrink-0 tabular-nums">
                {Math.max(1, Math.round(item.size / 1024))} KB
              </Badge>
              {item.status === "failed" || item.status === "done" ? (
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  aria-label={t("inspections.queue.remove")}
                  onClick={() => onRemove(item.id)}
                >
                  <Trash2 className="size-4" />
                </Button>
              ) : null}
            </li>
          );
        })}
      </ul>
    </EntitySectionCard>
  );
}
