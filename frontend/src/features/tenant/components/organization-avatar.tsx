"use client";

import { Building2 } from "lucide-react";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { resolveLogoSrc } from "@/lib/branding/logo-url";
import { assertServiceMediaURL } from "@/lib/media/urls";
import { cn } from "@/lib/utils";

type OrganizationAvatarVariant = "tile" | "wide";

/**
 * Organization logo from the API-owned `logo_url` (never an object key),
 * falling back to a building icon.
 *
 * - `tile` (default): square tile for lists, the switcher and detail headers.
 * - `wide`: keeps the logo's own aspect ratio (most company logos are
 *   landscape) up to a max width; used by the sidebar header. Collapses to a
 *   square tile when the sidebar is in icon mode.
 *
 * Uploaded logos are artwork drawn for light backgrounds, so the image sits
 * on a light plate in every theme; the fallback uses theme tokens.
 */
export function OrganizationAvatar({
  logoUrl,
  variant = "tile",
  className,
}: {
  logoUrl?: string | null;
  variant?: OrganizationAvatarVariant;
  className?: string;
}) {
  const src = resolveLogoSrc(assertServiceMediaURL(logoUrl));
  const wide = variant === "wide";
  return (
    <Avatar
      className={cn(
        "ring-border rounded-lg ring-1",
        wide
          ? "h-8 w-auto max-w-28 min-w-8 group-data-[collapsible=icon]:w-8 group-data-[collapsible=icon]:max-w-8"
          : "size-8",
        className,
      )}
    >
      {src ? (
        <AvatarImage
          src={src}
          alt=""
          decoding="async"
          draggable={false}
          className={cn(
            "bg-white object-contain p-0.5",
            wide && "aspect-auto w-auto max-w-full px-1",
          )}
        />
      ) : null}
      <AvatarFallback className="bg-sidebar-primary text-sidebar-primary-foreground rounded-none">
        <Building2 className="size-4" />
      </AvatarFallback>
    </Avatar>
  );
}
