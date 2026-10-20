"use client";

import type { ReactNode } from "react";
import { MotionConfig, motion, type HTMLMotionProps } from "motion/react";

export const EASE_OUT = [0.22, 1, 0.36, 1] as const;

/**
 * Landing-wide motion settings: with `prefers-reduced-motion` motion drops
 * transforms and layout animation, so content simply appears in place.
 */
export function LandingMotion({ children }: { children: ReactNode }) {
  return (
    <MotionConfig reducedMotion="user" transition={{ ease: EASE_OUT }}>
      {children}
    </MotionConfig>
  );
}

type RevealProps = HTMLMotionProps<"div"> & {
  delay?: number;
  /** Distance in px the block rises while fading in. */
  y?: number;
};

/** Fades and lifts its children in once, when they scroll into view. */
export function Reveal({ delay = 0, y = 28, children, ...props }: RevealProps) {
  return (
    <motion.div
      initial={{ opacity: 0, y }}
      whileInView={{ opacity: 1, y: 0 }}
      viewport={{ once: true, margin: "0px 0px -12% 0px" }}
      transition={{ duration: 0.7, delay, ease: EASE_OUT }}
      {...props}
    >
      {children}
    </motion.div>
  );
}

/** A mockup plate that lifts slightly under the pointer. */
export function HoverLift({ children, ...props }: HTMLMotionProps<"div">) {
  return (
    <motion.div
      whileHover={{ y: -6, scale: 1.01 }}
      transition={{ type: "spring", stiffness: 300, damping: 24 }}
      {...props}
    >
      {children}
    </motion.div>
  );
}
