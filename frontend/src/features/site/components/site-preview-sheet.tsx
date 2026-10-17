"use client";

import { ExternalLink, Monitor, RefreshCw, Smartphone } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { useSitePreview } from "@/features/site/hooks/use-site";
import { useDebounce } from "@/hooks/use-debounce";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type Device = "desktop" | "mobile";

/**
 * The site app itself on its own address with a short-lived draft token
 * (POST /v1/tenant/site/preview-token). The site renders the saved draft,
 * shows a preview banner and never books; only this panel's origin may
 * frame it. A new settings version (save, upload, restore) reloads the
 * frame after a short pause, so a burst of writes reloads it once.
 */
export function SitePreviewFrame({
  enabled,
  version,
  className,
}: {
  enabled: boolean;
  /** Settings version: a new one reloads the frame (debounced). */
  version: number;
  className?: string;
}) {
  const { t } = useLocale();
  const preview = useSitePreview(enabled);
  const [device, setDevice] = useState<Device>("desktop");
  const [nonce, setNonce] = useState(0);
  const settledVersion = useDebounce(version, 800);
  const url = preview.data?.url;

  return (
    <div className={cn("flex min-h-0 flex-col gap-3", className)}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          value={device}
          onValueChange={(v) => v && setDevice(v as Device)}
          aria-label={t("site.preview.device")}
        >
          <ToggleGroupItem
            value="desktop"
            aria-label={t("site.preview.desktop")}
          >
            <Monitor className="size-4" />
          </ToggleGroupItem>
          <ToggleGroupItem value="mobile" aria-label={t("site.preview.mobile")}>
            <Smartphone className="size-4" />
          </ToggleGroupItem>
        </ToggleGroup>
        <div className="flex gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              setNonce((n) => n + 1);
              void preview.refetch();
            }}
          >
            <RefreshCw className="size-4" />
            {t("site.preview.reload")}
          </Button>
          {url ? (
            <Button type="button" size="sm" variant="outline" asChild>
              <a href={url} target="_blank" rel="noreferrer">
                <ExternalLink className="size-4" />
                {t("site.preview.new_tab")}
              </a>
            </Button>
          ) : null}
        </div>
      </div>
      <div className="bg-muted/40 flex min-h-0 flex-1 justify-center overflow-hidden rounded-lg border">
        {preview.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            description={t("site.preview.failed")}
            onRetry={() => void preview.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : !url ? (
          <Skeleton className="size-full" />
        ) : (
          <iframe
            key={`${url}-${settledVersion}-${nonce}`}
            src={url}
            title={t("site.preview.title")}
            sandbox="allow-scripts allow-same-origin allow-forms allow-popups"
            referrerPolicy="no-referrer"
            className={cn(
              "bg-background h-full border-0",
              device === "mobile" ? "w-[390px] max-w-full" : "w-full",
            )}
          />
        )}
      </div>
    </div>
  );
}

/** Live preview docked next to the editor on wide screens. */
export function SiteLivePreview({
  version,
  dirty,
}: {
  version: number;
  dirty: boolean;
}) {
  const { t } = useLocale();
  return (
    <aside
      aria-label={t("site.preview.title")}
      className="border-border bg-card sticky top-4 flex h-[calc(100dvh-6rem)] min-h-[32rem] flex-col gap-3 rounded-xl border p-4"
    >
      <div className="space-y-0.5">
        <h2 className="font-display text-base font-semibold tracking-tight">
          {t("site.preview.title")}
        </h2>
        <p
          className={cn(
            "text-xs",
            dirty
              ? "text-amber-700 dark:text-amber-400"
              : "text-muted-foreground",
          )}
        >
          {dirty ? t("site.preview.unsaved") : t("site.preview.live_hint")}
        </p>
      </div>
      <SitePreviewFrame enabled version={version} className="flex-1" />
    </aside>
  );
}

/** Preview in a sheet (narrow screens). */
export function SitePreviewSheet({
  open,
  onOpenChange,
  version,
  dirty,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Settings version: a new one reloads the frame. */
  version: number;
  dirty: boolean;
}) {
  const { t } = useLocale();
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-4 sm:max-w-[min(1280px,96vw)]"
      >
        <SheetHeader>
          <SheetTitle>{t("site.preview.title")}</SheetTitle>
          <SheetDescription>
            {dirty ? t("site.preview.unsaved") : t("site.preview.description")}
          </SheetDescription>
        </SheetHeader>
        <SitePreviewFrame enabled={open} version={version} className="flex-1" />
      </SheetContent>
    </Sheet>
  );
}
