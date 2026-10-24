"use client";

import { useState } from "react";

import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { cn } from "@/lib/utils";

/**
 * Vehicle brand logos (API-owned URLs, never storage links).
 *
 * BrandMark shows the compact badge (emblem) next to names in tables, cards
 * and pickers and falls back to the brand initial. BrandWordmark shows the
 * text-only logo in headers and falls back to the brand name.
 */

function withVariant(
  url: string | null | undefined,
  variant: "badge" | "wordmark",
) {
  if (!url) return undefined;
  const sep = url.includes("?") ? "&" : "?";
  return resolveLogoSrc(`${url}${sep}variant=${variant}`);
}

export function BrandMark({
  name,
  logoUrl,
  className,
  size = "sm",
}: {
  name?: string | null;
  /** `logo_url` or `logo_badge_url` from the API. */
  logoUrl?: string | null;
  className?: string;
  size?: "sm" | "md" | "lg";
}) {
  const [failed, setFailed] = useState(false);
  const label = name?.trim() ?? "";
  const src = withVariant(logoUrl, "badge");
  if (!src && !label) return null;
  const box = size === "lg" ? "size-12" : size === "md" ? "size-8" : "size-6";
  return (
    <span
      title={label || undefined}
      className={cn(
        "grid shrink-0 place-items-center overflow-hidden rounded-md border bg-white",
        box,
        className,
      )}
    >
      {src && !failed ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={src}
          alt={label}
          className="size-full object-contain p-0.5"
          onError={() => setFailed(true)}
        />
      ) : (
        <span className="text-[10px] font-semibold text-neutral-600">
          {label.charAt(0).toLocaleUpperCase("tr-TR")}
        </span>
      )}
    </span>
  );
}

export function BrandWordmark({
  name,
  logoUrl,
  wordmarkUrl,
  className,
  height = 24,
}: {
  name?: string | null;
  /** `logo_url` (the wordmark variant is requested from it). */
  logoUrl?: string | null;
  /** `logo_wordmark_url` when the API returns it; null hides the image. */
  wordmarkUrl?: string | null;
  className?: string;
  height?: number;
}) {
  const [failed, setFailed] = useState(false);
  const label = name?.trim() ?? "";
  const src =
    wordmarkUrl === null
      ? undefined
      : wordmarkUrl
        ? resolveLogoSrc(wordmarkUrl)
        : withVariant(logoUrl, "wordmark");
  if (src && !failed) {
    return (
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={src}
        alt={label}
        title={label || undefined}
        style={{ height, maxWidth: height * 6 }}
        className={cn("w-auto object-contain object-left", className)}
        onError={() => setFailed(true)}
      />
    );
  }
  if (!label) return null;
  return <span className={cn("font-medium", className)}>{label}</span>;
}
