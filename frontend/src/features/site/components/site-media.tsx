"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Check, ImageOff, ImagePlus, Pencil, Trash2, X } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { Controller, get, useFormContext } from "react-hook-form";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { FormFieldShell } from "@/components/forms/form-field";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import {
  useDeleteSiteMedia,
  usePatchSiteMedia,
  useSiteMedia,
  useSiteMediaUrls,
  useUploadSiteMedia,
} from "@/features/site/hooks/use-site-media";
import { MEDIA_RULES } from "@/features/site/lib/site-options";
import type { SiteMedia } from "@/features/site/types";
import { cn } from "@/lib/utils";
import { datetime, fileSize } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** Thumbnail of a library item (API-owned or local preview URL only). */
export function MediaThumb({
  src,
  alt,
  className,
}: {
  src: string | undefined;
  alt: string;
  className?: string;
}) {
  const { t } = useLocale();
  return (
    <div
      className={cn(
        "bg-muted/40 text-muted-foreground flex items-center justify-center overflow-hidden rounded-md border",
        className,
      )}
    >
      {src ? (
        // eslint-disable-next-line @next/next/no-img-element -- API-owned media stream / local File preview; not optimizable.
        <img src={src} alt={alt} className="size-full object-cover" />
      ) : (
        <span
          className="flex flex-col items-center gap-1 p-2 text-center text-[11px] leading-tight"
          title={t("site.media.no_thumbnail_hint")}
        >
          <ImageOff className="size-5" aria-hidden />
          {t("site.media.no_thumbnail")}
        </span>
      )}
    </div>
  );
}

function altOf(item: SiteMedia, locale: string) {
  return (
    (locale === "en"
      ? item.alt.en || item.alt.tr
      : item.alt.tr || item.alt.en) ?? ""
  );
}

/** Upload button (multiple files) with the client-side precheck. */
export function MediaUploadButton({
  onUploaded,
  size = "sm",
  variant = "outline",
}: {
  onUploaded?: (items: SiteMedia[]) => void;
  size?: "sm" | "default";
  variant?: "outline" | "default";
}) {
  const { t } = useLocale();
  const input = useRef<HTMLInputElement>(null);
  const upload = useUploadSiteMedia();
  return (
    <>
      <Input
        ref={input}
        type="file"
        multiple
        accept={MEDIA_RULES.accept.join(",")}
        className="hidden"
        aria-label={t("site.media.upload")}
        onChange={(e) => {
          const files = Array.from(e.target.files ?? []);
          e.target.value = "";
          if (!files.length) return;
          upload.mutate(files, {
            onSuccess: (items) => onUploaded?.(items),
          });
        }}
      />
      <Button
        type="button"
        size={size}
        variant={variant}
        disabled={upload.isPending}
        onClick={() => input.current?.click()}
      >
        <ImagePlus className="size-4" />
        {upload.isPending ? t("site.media.uploading") : t("site.media.upload")}
      </Button>
    </>
  );
}

/** "12 / 200 dosya · 48 MB / 500 MB" with a progress bar. */
function QuotaIndicator({
  items,
  total,
}: {
  items: SiteMedia[];
  total: number;
}) {
  const { t } = useLocale();
  const bytes = items.reduce((sum, m) => sum + m.size, 0);
  const ratio = Math.max(
    total / MEDIA_RULES.maxFiles,
    bytes / MEDIA_RULES.maxTotalBytes,
  );
  return (
    <div className="min-w-48 space-y-1.5">
      <p className="text-muted-foreground text-xs tabular-nums">
        {t("site.media.quota", {
          count: total,
          max: MEDIA_RULES.maxFiles,
          size: fileSize(bytes),
          max_size: fileSize(MEDIA_RULES.maxTotalBytes),
        })}
      </p>
      <Progress
        value={Math.min(100, Math.round(ratio * 100))}
        aria-label={t("site.media.quota_label")}
        className={cn(ratio >= 0.9 && "[&>*]:bg-destructive")}
      />
    </div>
  );
}

function MediaCard({
  item,
  src,
  selected,
}: {
  item: SiteMedia;
  src: string | undefined;
  selected?: boolean;
}) {
  const { locale } = useLocale();
  const alt = altOf(item, locale);
  return (
    <div className="space-y-2">
      <div className="relative">
        <MediaThumb src={src} alt={alt} className="aspect-video w-full" />
        {selected ? (
          <span className="bg-primary text-primary-foreground absolute end-2 top-2 rounded-full p-1">
            <Check className="size-3.5" aria-hidden />
          </span>
        ) : null}
      </div>
      <div className="min-w-0 text-sm">
        <p className="truncate font-medium">{alt || "—"}</p>
        <p className="text-muted-foreground text-xs tabular-nums">
          {item.width}×{item.height} · {fileSize(item.size)}
        </p>
      </div>
    </div>
  );
}

/** Dialog editing the tr / en alt text of one item (no nested form). */
function AltDialog({
  item,
  onClose,
}: {
  item: SiteMedia | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const patch = usePatchSiteMedia();
  const [alt, setAlt] = useState({ tr: "", en: "" });
  const [loadedFor, setLoadedFor] = useState<string | null>(null);
  if (item && loadedFor !== item.uuid) {
    setLoadedFor(item.uuid);
    setAlt({ tr: item.alt.tr ?? "", en: item.alt.en ?? "" });
  }
  const tooLong = alt.tr.length > 300 || alt.en.length > 300;
  return (
    <AppDialog
      open={item !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={t("site.media.alt_title")}
      description={t("site.media.alt_description")}
      footer={
        <>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="button"
            disabled={patch.isPending || tooLong}
            onClick={() => {
              if (!item) return;
              patch.mutate(
                {
                  uuid: item.uuid,
                  alt: { tr: alt.tr.trim(), en: alt.en.trim() },
                },
                { onSuccess: onClose },
              );
            }}
          >
            {t("common.save")}
          </Button>
        </>
      }
    >
      <div className="grid gap-4">
        {(["tr", "en"] as const).map((loc) => (
          <div key={loc} className="grid gap-1.5">
            <Label htmlFor={`media-alt-${loc}`}>
              {t("site.media.alt_label", {
                locale: t(`site.locales.${loc}`),
              })}
            </Label>
            <Input
              id={`media-alt-${loc}`}
              value={alt[loc]}
              maxLength={300}
              onChange={(e) =>
                setAlt((prev) => ({ ...prev, [loc]: e.target.value }))
              }
            />
          </div>
        ))}
      </div>
    </AppDialog>
  );
}

/**
 * Görseller → Kütüphane (TEC-781): the organization's site media as a
 * DataTable (table or grid), multiple upload, alt text, delete (refused
 * while a draft or any published revision uses the image) and the quota.
 */
export function SiteMediaLibrary({ canManage }: { canManage: boolean }) {
  const { t, locale } = useLocale();
  const media = useSiteMedia();
  const urls = useSiteMediaUrls();
  const remove = useDeleteSiteMedia();
  const [editing, setEditing] = useState<SiteMedia | null>(null);
  const [deleting, setDeleting] = useState<SiteMedia | null>(null);
  const items = useMemo(() => media.data?.items ?? [], [media.data]);

  const columns = useMemo(
    () =>
      [
        createColumn<SiteMedia>({
          id: "thumbnail",
          accessorFn: (row) => row.uuid,
          labelKey: "site.media.columns.thumbnail",
          enableSorting: false,
          cell: ({ row }) => (
            <MediaThumb
              src={urls[row.original.uuid]}
              alt={altOf(row.original, locale)}
              className="h-12 w-20"
            />
          ),
        }),
        createColumn<SiteMedia>({
          id: "alt_tr",
          accessorFn: (row) => row.alt.tr ?? "",
          labelKey: "site.media.columns.alt_tr",
          gridPrimary: true,
          cell: ({ row }) =>
            row.original.alt.tr || (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<SiteMedia>({
          id: "alt_en",
          accessorFn: (row) => row.alt.en ?? "",
          labelKey: "site.media.columns.alt_en",
          cell: ({ row }) =>
            row.original.alt.en || (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<SiteMedia>({
          accessorKey: "size",
          labelKey: "site.media.columns.size",
          cell: ({ row }) => (
            <span className="tabular-nums">{fileSize(row.original.size)}</span>
          ),
        }),
        createColumn<SiteMedia>({
          id: "dimensions",
          accessorFn: (row) => row.width * row.height,
          labelKey: "site.media.columns.dimensions",
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {row.original.width}×{row.original.height}
            </span>
          ),
        }),
        createColumn<SiteMedia>({
          accessorKey: "created_at",
          labelKey: "site.media.columns.created_at",
          cell: ({ row }) =>
            datetime(row.original.created_at, undefined, locale),
        }),
        createColumn<SiteMedia>({
          id: "actions",
          labelKey: "site.media.columns.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) =>
            canManage ? (
              <EntityRowActions
                actions={[
                  {
                    id: "alt",
                    label: t("site.media.edit_alt"),
                    icon: Pencil,
                    onSelect: () => setEditing(row.original),
                  },
                  {
                    id: "delete",
                    label: t("site.media.delete"),
                    icon: Trash2,
                    variant: "destructive",
                    onSelect: () => setDeleting(row.original),
                  },
                ]}
              />
            ) : null,
        }),
      ] as ColumnDef<SiteMedia, unknown>[],
    [canManage, locale, t, urls],
  );

  return (
    <EntitySectionCard title={t("site.media.title")}>
      <div className="space-y-4">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <p className="text-muted-foreground max-w-xl text-sm">
            {t("site.media.description", {
              size: MEDIA_RULES.maxBytes / 1024 / 1024,
            })}
          </p>
          {media.data ? (
            <QuotaIndicator items={items} total={media.data.total} />
          ) : null}
        </div>
        <EntityTable
          columns={columns}
          data={items}
          getRowId={(row) => row.uuid}
          isLoading={media.isLoading}
          isError={media.isError}
          errorDescription={t("site.media.load_failed")}
          onRetry={() => void media.refetch()}
          emptyTitle={t("site.media.empty_title")}
          emptyDescription={t("site.media.empty_description")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          initialState={{ sorting: [{ id: "created_at", desc: true }] }}
          features={{
            persistKey: "tenant-site-media-v1",
            rowSelection: false,
            viewMode: true,
          }}
          renderGridItem={(item) => (
            <div className="space-y-3">
              <MediaCard item={item} src={urls[item.uuid]} />
              {canManage ? (
                <div className="flex justify-end gap-1">
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("site.media.edit_alt")}
                    onClick={() => setEditing(item)}
                  >
                    <Pencil className="size-4" />
                  </Button>
                  <Button
                    type="button"
                    size="icon-sm"
                    variant="ghost"
                    aria-label={t("site.media.delete")}
                    onClick={() => setDeleting(item)}
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </div>
              ) : null}
            </div>
          )}
          toolbarExtra={
            <EntityToolbar
              onRefresh={() => void media.refetch()}
              refreshDisabled={media.isFetching}
            >
              {canManage ? <MediaUploadButton /> : null}
            </EntityToolbar>
          }
        />
      </div>
      <AltDialog item={editing} onClose={() => setEditing(null)} />
      <ConfirmDialog
        open={deleting !== null}
        title={t("site.media.delete_title")}
        description={t("site.media.delete_description")}
        confirmLabel={t("site.media.delete")}
        cancelLabel={t("common.cancel")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setDeleting(null)}
        onConfirm={() => {
          if (!deleting) return;
          remove.mutate(deleting.uuid, {
            onSettled: () => setDeleting(null),
          });
        }}
      />
    </EntitySectionCard>
  );
}

/**
 * Reusable picker: the library as a grid with upload; a click picks the
 * image. Used by hero slides and the about image (later campaigns).
 */
export function MediaPickerDialog({
  open,
  onOpenChange,
  selected,
  onSelect,
  canUpload = true,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selected?: string;
  onSelect: (uuid: string) => void;
  canUpload?: boolean;
}) {
  const { t } = useLocale();
  const media = useSiteMedia();
  const urls = useSiteMediaUrls();
  const items = useMemo(() => media.data?.items ?? [], [media.data]);
  const pick = (uuid: string) => {
    onSelect(uuid);
    onOpenChange(false);
  };
  const columns = useMemo(
    () =>
      [
        createColumn<SiteMedia>({
          id: "alt",
          accessorFn: (row) => `${row.alt.tr ?? ""} ${row.alt.en ?? ""}`,
          labelKey: "site.media.columns.alt_tr",
          gridPrimary: true,
        }),
        createColumn<SiteMedia>({
          accessorKey: "created_at",
          labelKey: "site.media.columns.created_at",
        }),
      ] as ColumnDef<SiteMedia, unknown>[],
    [],
  );
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("site.media.picker_title")}
      description={t("site.media.picker_description")}
      className="max-h-[90vh] overflow-y-auto sm:max-w-4xl"
    >
      {open ? (
        <EntityTable
          columns={columns}
          data={items}
          getRowId={(row) => row.uuid}
          isLoading={media.isLoading}
          isError={media.isError}
          errorDescription={t("site.media.load_failed")}
          onRetry={() => void media.refetch()}
          emptyTitle={t("site.media.empty_title")}
          emptyDescription={t("site.media.picker_empty")}
          manual={{ filtering: false, sorting: false, pagination: false }}
          initialState={{
            viewMode: "grid",
            sorting: [{ id: "created_at", desc: true }],
          }}
          features={{
            rowSelection: false,
            viewMode: false,
            columnVisibility: false,
            density: false,
          }}
          onRowClick={(row) => pick(row.uuid)}
          renderGridItem={(item) => (
            <MediaCard
              item={item}
              src={urls[item.uuid]}
              selected={item.uuid === selected}
            />
          )}
          toolbarExtra={
            canUpload ? (
              <MediaUploadButton
                onUploaded={(created) => {
                  if (created.length === 1) pick(created[0].uuid);
                }}
              />
            ) : null
          }
        />
      ) : null}
    </AppDialog>
  );
}

/** Form field holding a library UUID ("" = none) with a picker. */
export function MediaField({
  name,
  label,
  description,
  disabled,
}: {
  name: string;
  label: string;
  description?: string;
  disabled?: boolean;
}) {
  const { t, locale } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext();
  const error = get(errors, name)?.message as string | undefined;
  const media = useSiteMedia();
  const urls = useSiteMediaUrls();
  const [open, setOpen] = useState(false);
  return (
    <FormFieldShell
      name={name}
      label={label}
      description={description}
      error={error}
    >
      <Controller
        name={name}
        control={control}
        render={({ field }) => {
          const uuid = String(field.value ?? "");
          const item = media.data?.items.find((m) => m.uuid === uuid);
          return (
            <div className="flex flex-wrap items-center gap-3">
              {uuid ? (
                <MediaThumb
                  src={urls[uuid]}
                  alt={item ? altOf(item, locale) : ""}
                  className="h-16 w-28"
                />
              ) : (
                <div className="text-muted-foreground flex h-16 w-28 items-center justify-center rounded-md border border-dashed text-xs">
                  {t("site.media.none_selected")}
                </div>
              )}
              <div className="min-w-0 flex-1 space-y-1">
                {uuid && !item && media.isSuccess ? (
                  <p className="text-destructive text-xs">
                    {t("site.media.missing")}
                  </p>
                ) : item ? (
                  <p className="text-muted-foreground truncate text-xs">
                    {altOf(item, locale) || t("site.media.no_alt")} ·{" "}
                    {item.width}×{item.height}
                  </p>
                ) : null}
                {!disabled ? (
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => setOpen(true)}
                    >
                      <ImagePlus className="size-4" />
                      {uuid ? t("site.media.change") : t("site.media.choose")}
                    </Button>
                    {uuid ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => field.onChange("")}
                      >
                        <X className="size-4" />
                        {t("site.media.clear")}
                      </Button>
                    ) : null}
                  </div>
                ) : null}
              </div>
              <MediaPickerDialog
                open={open}
                onOpenChange={setOpen}
                selected={uuid}
                onSelect={(id) => field.onChange(id)}
              />
            </div>
          );
        }}
      />
    </FormFieldShell>
  );
}
