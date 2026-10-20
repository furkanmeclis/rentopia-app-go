import { useId, type SVGProps } from "react";

import type { BrandTone } from "@/config/brand";
import { cn } from "@/lib/utils";

import { RENTOPYA_PATHS } from "./rentopya-paths";

type BrandSvgProps = Omit<SVGProps<SVGSVGElement>, "children"> & {
  title: string;
  tone?: BrandTone;
};

/**
 * Fill classes per tone. `tone` names the background the art sits on:
 * `light` → colourful kit (indigo mark, ink letters), `dark` → white kit,
 * `auto` → colourful in light mode, white in dark mode.
 */
const MARK_FILL: Record<BrandTone, string> = {
  auto: "fill-[#2E358C] dark:fill-white",
  light: "fill-[#2E358C]",
  dark: "fill-white",
};
const WORD_FILL: Record<BrandTone, string> = {
  auto: "fill-[#141527] dark:fill-white",
  light: "fill-[#141527]",
  dark: "fill-white",
};

function Svg({
  viewBox,
  title,
  className,
  children,
  ...props
}: Omit<BrandSvgProps, "tone"> & {
  viewBox: string;
  children: React.ReactNode;
}) {
  return (
    <svg
      viewBox={viewBox}
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label={title}
      className={cn("shrink-0", className)}
      {...props}
    >
      <title>{title}</title>
      {children}
    </svg>
  );
}

/** Horizontal lockup: "R" mark + "rentopya" letters. */
export function RentopyaLogoSvg({ tone = "auto", ...props }: BrandSvgProps) {
  const { mark, word } = RENTOPYA_PATHS.logo;
  return (
    <Svg viewBox="97 122 1342 306" {...props}>
      <path className={MARK_FILL[tone]} fillRule="evenodd" {...mark} />
      <path className={WORD_FILL[tone]} fillRule="evenodd" {...word} />
    </Svg>
  );
}

/** Stacked lockup: mark above the letters. */
export function RentopyaStackedSvg({ tone = "auto", ...props }: BrandSvgProps) {
  const { mark, word } = RENTOPYA_PATHS.stacked;
  return (
    <Svg viewBox="78 56 669 501" {...props}>
      <path className={MARK_FILL[tone]} fillRule="evenodd" {...mark} />
      <path className={WORD_FILL[tone]} fillRule="evenodd" {...word} />
    </Svg>
  );
}

/** Letters only ("rentopya"). */
export function RentopyaWordSvg({ tone = "auto", ...props }: BrandSvgProps) {
  return (
    <Svg viewBox="436 194 1003 224" {...props}>
      <path
        className={WORD_FILL[tone]}
        fillRule="evenodd"
        {...RENTOPYA_PATHS.logo.word}
      />
    </Svg>
  );
}

/** The "R" glyph alone, square frame. */
export function RentopyaMarkSvg({ tone = "auto", ...props }: BrandSvgProps) {
  return (
    <Svg viewBox="97 122 308 308" {...props}>
      <path
        className={MARK_FILL[tone]}
        fillRule="evenodd"
        {...RENTOPYA_PATHS.logo.mark}
      />
    </Svg>
  );
}

/**
 * App icon: white "R" on the indigo → ink gradient square. Same art on any
 * background, so it takes no tone.
 */
export function RentopyaIconSvg(props: Omit<BrandSvgProps, "tone">) {
  const gradientId = `rentopya-icon-${useId().replace(/[^a-zA-Z0-9_-]/g, "")}`;
  return (
    <Svg viewBox="0 0 850 850" {...props}>
      <defs>
        <linearGradient
          id={gradientId}
          x1="425"
          y1="850"
          x2="425"
          y2="0"
          gradientUnits="userSpaceOnUse"
        >
          <stop offset="0" stopColor="#141527" />
          <stop offset="1" stopColor="#2E358C" />
        </linearGradient>
      </defs>
      <rect width="850" height="850" fill={`url(#${gradientId})`} />
      <path fill="#FFFFFF" fillRule="evenodd" {...RENTOPYA_PATHS.icon} />
    </Svg>
  );
}
