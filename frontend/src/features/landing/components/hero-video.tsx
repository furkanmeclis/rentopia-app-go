"use client";

import { useEffect, useRef } from "react";

import { useLocale } from "@/providers/locale-provider";

import { LANDING_HERO_VIDEO } from "../config";
import { usePrefersReducedMotion } from "../lib/use-reduced-motion";
import { FleetCalendarMock } from "../mockups/fleet-calendar-mock";
import { BrowserFrame } from "../mockups/site-mock";

/**
 * The product loop inside browser chrome. A static fleet-calendar mock sits
 * under the video, so the frame still reads when the files are missing or
 * still loading (a video without a loadable source or poster is transparent).
 * With reduced motion the video stays on its first frame.
 */
export function HeroVideo() {
  const { t } = useLocale();
  const ref = useRef<HTMLVideoElement>(null);
  const reducedMotion = usePrefersReducedMotion();

  useEffect(() => {
    const video = ref.current;
    if (!video) return;
    if (reducedMotion) {
      video.pause();
    } else {
      void video.play().catch(() => {
        // Autoplay can be refused (data saver); the poster stays.
      });
    }
  }, [reducedMotion]);

  return (
    <BrowserFrame address={t("landing.hero.frame_title")}>
      <div className="relative aspect-video bg-[#F6F7FC]">
        <div className="absolute inset-0 grid place-items-center overflow-hidden p-4 sm:p-8 lg:p-12">
          <FleetCalendarMock compact className="w-full max-w-3xl" />
        </div>
        <video
          ref={ref}
          className="absolute inset-0 size-full object-cover"
          autoPlay
          muted
          loop
          playsInline
          preload="metadata"
          poster={LANDING_HERO_VIDEO.poster}
          aria-label={t("landing.hero.video_label")}
        >
          <source src={LANDING_HERO_VIDEO.mp4} type="video/mp4" />
          <source src={LANDING_HERO_VIDEO.webm} type="video/webm" />
        </video>
      </div>
    </BrowserFrame>
  );
}
