"use client";

import dynamic from "next/dynamic";

import { cn } from "@/lib/utils";

import { usePrefersReducedMotion } from "../lib/use-reduced-motion";

// WebGL shaders load on the client only, after the page is interactive; the
// ink background underneath is what the server renders and what shows when
// WebGL is unavailable.
const MeshGradient = dynamic(
  () => import("@paper-design/shaders-react").then((m) => m.MeshGradient),
  { ssr: false },
);
const GrainGradient = dynamic(
  () => import("@paper-design/shaders-react").then((m) => m.GrainGradient),
  { ssr: false },
);

const INK = "#141527";

/**
 * Hero band: the brand mesh (ink, indigo, the two lighter indigos), slow and
 * dark enough for white text. Holds still for reduced motion.
 */
export function HeroShader({ className }: { className?: string }) {
  const reducedMotion = usePrefersReducedMotion();
  return (
    <div
      aria-hidden
      className={cn("pointer-events-none bg-[#141527]", className)}
    >
      <MeshGradient
        className="size-full"
        colors={[INK, "#2E358C", INK, "#5A62C4", "#1D2160"]}
        distortion={0.7}
        swirl={0.25}
        grainMixer={0.1}
        grainOverlay={0.05}
        speed={reducedMotion ? 0 : 0.22}
        maxPixelCount={1600 * 900}
      />
    </div>
  );
}

/** Closing band: a grainy indigo wave, static unless motion is welcome. */
export function ClosingShader({ className }: { className?: string }) {
  const reducedMotion = usePrefersReducedMotion();
  return (
    <div
      aria-hidden
      className={cn("pointer-events-none bg-[#141527]", className)}
    >
      <GrainGradient
        className="size-full"
        colorBack={INK}
        colors={["#2E358C", "#5A62C4", "#8E95EC"]}
        softness={0.7}
        intensity={0.35}
        noise={0.25}
        shape="wave"
        speed={reducedMotion ? 0 : 0.18}
        maxPixelCount={1600 * 900}
      />
    </div>
  );
}
