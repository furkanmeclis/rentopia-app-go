"use client";

import { ImageOff, ImageUp, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { FormSection } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  useRemoveSiteImage,
  useSitePreview,
  useUploadSiteImage,
} from "@/features/site/hooks/use-site";
import type { SiteImageKind, SiteSettings } from "@/features/site/types";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const ACCEPT = "image/png,image/jpeg,image/webp";
const LIMITS: Record<SiteImageKind, number> = {
  logo: 2 * 1024 * 1024,
  favicon: 1024 * 1024,
  cover: 5 * 1024 * 1024,
};
const KINDS: SiteImageKind[] = ["logo", "favicon", "cover"];

function ImageCard({
  kind,
  settings,
  src,
  loading,
  canManage,
}: {
  kind: SiteImageKind;
  settings: SiteSettings;
  src: string | undefined;
  loading: boolean;
  canManage: boolean;
}) {
  const { t } = useLocale();
  const input = useRef<HTMLInputElement>(null);
  const upload = useUploadSiteImage();
  const remove = useRemoveSiteImage();
  const [confirmRemove, setConfirmRemove] = useState(false);
  const has = settings.assets[kind];

  const pick = (file: File | undefined) => {
    if (!file) return;
    if (!ACCEPT.split(",").includes(file.type)) {
      appToast.error(t("site.images.invalid_type"));
      return;
    }
    if (file.size > LIMITS[kind]) {
      appToast.error(
        t("site.images.too_large", { size: LIMITS[kind] / 1024 / 1024 }),
      );
      return;
    }
    upload.mutate({ kind, file, version: settings.version });
  };

  return (
    <div className="flex flex-col gap-3 rounded-lg border p-4">
      <div>
        <p className="text-sm font-medium">{t(`site.images.kinds.${kind}`)}</p>
        <p className="text-muted-foreground text-xs">
          {t(`site.images.hints.${kind}`, {
            size: LIMITS[kind] / 1024 / 1024,
          })}
        </p>
      </div>
      <div
        className={cn(
          "bg-muted/40 flex items-center justify-center overflow-hidden rounded-md border",
          kind === "cover" ? "aspect-[16/7]" : "aspect-square max-h-40",
        )}
      >
        {loading && has ? (
          <Skeleton className="size-full" />
        ) : has && src ? (
          // eslint-disable-next-line @next/next/no-img-element -- API-owned draft stream (preview token); not optimizable.
          <img
            src={src}
            alt={t(`site.images.kinds.${kind}`)}
            className={cn(
              "size-full",
              kind === "cover" ? "object-cover" : "object-contain p-3",
            )}
          />
        ) : (
          <div className="text-muted-foreground flex flex-col items-center gap-1 p-4 text-center text-xs">
            <ImageOff className="size-6" />
            {kind === "logo"
              ? t("site.images.logo_fallback")
              : t("site.images.none")}
          </div>
        )}
      </div>
      {canManage ? (
        <div className="flex flex-wrap gap-2">
          <Input
            ref={input}
            type="file"
            accept={ACCEPT}
            className="hidden"
            aria-label={t(`site.images.kinds.${kind}`)}
            onChange={(e) => {
              pick(e.target.files?.[0]);
              e.target.value = "";
            }}
          />
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={upload.isPending}
            onClick={() => input.current?.click()}
          >
            <ImageUp className="size-4" />
            {has ? t("site.images.replace") : t("site.images.upload")}
          </Button>
          {has ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={remove.isPending}
              onClick={() => setConfirmRemove(true)}
            >
              <Trash2 className="size-4" />
              {t("site.images.remove")}
            </Button>
          ) : null}
        </div>
      ) : null}
      <ConfirmDialog
        open={confirmRemove}
        title={t("site.images.remove_title")}
        description={t("site.images.remove_description")}
        confirmLabel={t("site.images.remove")}
        cancelLabel={t("common.cancel")}
        variant="destructive"
        isPending={remove.isPending}
        onCancel={() => setConfirmRemove(false)}
        onConfirm={() =>
          remove.mutate(
            { kind, version: settings.version },
            { onSettled: () => setConfirmRemove(false) },
          )
        }
      />
    </div>
  );
}

/**
 * Görseller: logo, favicon and cover of the draft (shown through the
 * preview token; published with the next publish). SVG is refused.
 */
export function SiteImagesTab({
  settings,
  canManage,
}: {
  settings: SiteSettings;
  canManage: boolean;
}) {
  const { t } = useLocale();
  const preview = useSitePreview(true);
  const paths = preview.data?.assets;
  return (
    <FormSection
      title={t("site.images.title")}
      description={t("site.images.description")}
    >
      <div className="grid gap-4 md:grid-cols-3">
        {KINDS.map((kind) => (
          <ImageCard
            key={kind}
            kind={kind}
            settings={settings}
            src={resolveLogoSrc(assertServiceMediaURL(paths?.[kind]))}
            loading={preview.isLoading}
            canManage={canManage}
          />
        ))}
      </div>
    </FormSection>
  );
}
