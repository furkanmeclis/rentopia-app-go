"use client";

import { ArrowLeft, ArrowRight, Star, Trash2 } from "lucide-react";
import { useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { EntityDeleteDialog } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { FileUploader } from "@/components/upload/file-uploader";
import { permissions } from "@/config/permissions";
import { mediaSrc } from "@/features/fleet/components/products/product-card-preview";
import {
  MAX_PRODUCT_IMAGES,
  MAX_PRODUCT_IMAGE_BYTES,
} from "@/features/fleet/constants";
import { useProductImages } from "@/features/fleet/hooks/use-fleet";
import type { ProductImage } from "@/features/fleet/types";
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Ordered product images: upload (JPEG / PNG / WebP, API-sniffed), move to
 * reorder (first = cover), delete. Images render from the API-owned
 * `public_url`; the browser never builds storage URLs.
 */
export function ProductImagesManager({
  productUuid,
  images,
}: {
  productUuid: string;
  images: ProductImage[];
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.fleet.write);
  const { upload, reorder, remove } = useProductImages(productUuid);
  const [deleteTarget, setDeleteTarget] = useState<ProductImage | null>(null);
  const ordered = [...images].sort((a, b) => a.sort_order - b.sort_order);
  const full = ordered.length >= MAX_PRODUCT_IMAGES;

  const move = (index: number, delta: number) => {
    const next = [...ordered];
    const target = index + delta;
    if (target < 0 || target >= next.length) return;
    [next[index], next[target]] = [next[target]!, next[index]!];
    reorder.mutate(next.map((image) => image.uuid));
  };

  return (
    <div className="space-y-4">
      {ordered.length ? (
        <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
          {ordered.map((image, index) => {
            const src = mediaSrc(image.public_url);
            return (
              <li
                key={image.uuid}
                className="bg-card overflow-hidden rounded-lg border"
              >
                <div className="bg-muted relative aspect-[4/3]">
                  {src ? (
                    // eslint-disable-next-line @next/next/no-img-element -- API-owned media stream
                    <img
                      src={src}
                      alt={image.filename}
                      className="size-full object-cover"
                      loading="lazy"
                    />
                  ) : null}
                  {index === 0 ? (
                    <Badge className="absolute start-2 top-2 gap-1">
                      <Star className="size-3" aria-hidden />
                      {t("fleet.images.cover")}
                    </Badge>
                  ) : null}
                </div>
                {canWrite ? (
                  <div className="flex items-center justify-between gap-1 p-1.5">
                    <div className="flex gap-1">
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        className="size-8"
                        aria-label={t("fleet.images.move_left")}
                        disabled={index === 0 || reorder.isPending}
                        onClick={() => move(index, -1)}
                      >
                        <ArrowLeft className="size-4" />
                      </Button>
                      <Button
                        type="button"
                        size="icon"
                        variant="ghost"
                        className="size-8"
                        aria-label={t("fleet.images.move_right")}
                        disabled={
                          index === ordered.length - 1 || reorder.isPending
                        }
                        onClick={() => move(index, 1)}
                      >
                        <ArrowRight className="size-4" />
                      </Button>
                    </div>
                    <Button
                      type="button"
                      size="icon"
                      variant="ghost"
                      className="text-destructive size-8"
                      aria-label={t("fleet.images.delete")}
                      onClick={() => setDeleteTarget(image)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("fleet.images.empty")}
        </p>
      )}

      <PermissionGuard permission={permissions.tenant.fleet.write}>
        {full ? (
          <p className="text-muted-foreground text-sm">
            {t("fleet.images.limit_reached", { max: MAX_PRODUCT_IMAGES })}
          </p>
        ) : (
          <div className="space-y-1">
            <FileUploader
              accept="image/jpeg,image/png,image/webp"
              maxSizeBytes={MAX_PRODUCT_IMAGE_BYTES}
              onUpload={async (files) => {
                const room = MAX_PRODUCT_IMAGES - ordered.length;
                for (const file of files.slice(0, room)) {
                  await upload.mutateAsync(file);
                }
                appToast.success(
                  t("fleet.images.toast.uploaded", {
                    count: Math.min(files.length, room),
                  }),
                );
              }}
            />
            <p className="text-muted-foreground text-xs">
              {t("fleet.images.hint", { max: MAX_PRODUCT_IMAGES })}
            </p>
          </div>
        )}
      </PermissionGuard>

      <EntityDeleteDialog
        open={Boolean(deleteTarget)}
        softDelete={false}
        entityLabel={deleteTarget?.filename || t("fleet.images.image")}
        isPending={remove.isPending}
        onCancel={() => setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return;
          remove.mutate(deleteTarget.uuid, {
            onSettled: () => setDeleteTarget(null),
          });
        }}
      />
    </div>
  );
}
