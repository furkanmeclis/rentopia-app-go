"use client";

import { ImageOff } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { GuaranteeBadgeChip } from "@/features/fleet/components/products/guarantee-badge";
import type { GuaranteeBadge } from "@/features/fleet/types";
import { apiConfig } from "@/config/api";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { cn } from "@/lib/utils";

export type ProductCardPreviewProps = {
  title: string;
  /** "Renault Clio veya benzeri", "Fiat Egea Cross 1.4 Fire (model garantili)". */
  displayText: string;
  badge: GuaranteeBadge;
  /** API-owned `public_url` of the cover image (used as is). */
  imageUrl?: string | null;
  /** Attribute chips (transmission, fuel, seats …). */
  chips?: string[];
  /** Small caption (segment name). */
  caption?: string;
  className?: string;
};

/**
 * How a rentable product looks on the website / a channel: cover image,
 * title, the guarantee line with its "veya benzeri" / "model garantili"
 * badge and attribute chips. Pure presentation, reused by the site later.
 */
export function ProductCardPreview({
  title,
  displayText,
  badge,
  imageUrl,
  chips = [],
  caption,
  className,
}: ProductCardPreviewProps) {
  const src = imageUrl ? mediaSrc(imageUrl) : null;
  return (
    <article
      className={cn(
        "bg-card text-card-foreground w-full max-w-sm overflow-hidden rounded-xl border shadow-sm",
        className,
      )}
    >
      <div className="bg-muted relative aspect-[16/10] w-full">
        {src ? (
          // eslint-disable-next-line @next/next/no-img-element -- API-owned media stream
          <img
            src={src}
            alt={title}
            className="size-full object-cover"
            loading="lazy"
          />
        ) : (
          <div className="text-muted-foreground flex size-full items-center justify-center">
            <ImageOff className="size-8" aria-hidden />
          </div>
        )}
        <div className="absolute start-3 top-3">
          <GuaranteeBadgeChip badge={badge} />
        </div>
      </div>
      <div className="space-y-2 p-4">
        {caption ? (
          <p className="text-muted-foreground text-xs font-medium tracking-wide uppercase">
            {caption}
          </p>
        ) : null}
        <h3 className="font-display text-lg leading-tight font-semibold">
          {title || "—"}
        </h3>
        <p className="text-sm">{displayText}</p>
        {chips.length ? (
          <div className="flex flex-wrap gap-1.5 pt-1">
            {chips.map((chip) => (
              <Badge key={chip} variant="outline" className="font-normal">
                {chip}
              </Badge>
            ))}
          </div>
        ) : null}
      </div>
    </article>
  );
}

/** API media paths go through the same-origin BFF (`/api/v1/...`). */
export function mediaSrc(publicUrl: string): string | null {
  const url = assertServiceMediaURL(publicUrl);
  if (!url) return null;
  if (url.startsWith("/v1/")) {
    return `${apiConfig.baseUrl.replace(/\/$/, "")}${url}`;
  }
  return url;
}
