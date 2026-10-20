"use client";

import { MeshGradient } from "@paper-design/shaders-react";
import { useSyncExternalStore } from "react";

import { cn } from "@/lib/utils";

const REDUCED_MOTION = "(prefers-reduced-motion: reduce)";

function subscribe(onChange: () => void) {
  const query = window.matchMedia(REDUCED_MOTION);
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
}

function usePrefersReducedMotion() {
  return useSyncExternalStore(
    subscribe,
    () => window.matchMedia(REDUCED_MOTION).matches,
    () => true,
  );
}

/**
 * Animated Rentopya mesh gradient behind the guest pages: ink, brand indigo
 * and two lighter indigo tones (docs/brand.md). It holds still when the
 * visitor prefers reduced motion; the ink background shows until WebGL draws.
 */
export function AuthBackdrop({ className }: { className?: string }) {
  const reducedMotion = usePrefersReducedMotion();

  return (
    <div
      aria-hidden
      className={cn("pointer-events-none bg-[#141527]", className)}
    >
      <MeshGradient
        className="size-full"
        colors={["#141527", "#2E358C", "#5A62C4", "#1D2160", "#8E95EC"]}
        distortion={0.85}
        swirl={0.35}
        grainMixer={0.12}
        grainOverlay={0.06}
        speed={reducedMotion ? 0 : 0.35}
        maxPixelCount={1920 * 1080}
      />
    </div>
  );
}
