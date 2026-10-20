import { useId, type SVGProps } from "react";

import type { AppLocale } from "@/config/i18n";
import { cn } from "@/lib/utils";

type FlagProps = SVGProps<SVGSVGElement>;

/**
 * Türkiye flag (1200×800 artwork), framed as a square centred on the crescent
 * and star so it reads correctly inside the round badge.
 */
export function TurkeyFlag({ className, ...props }: FlagProps) {
  return (
    <svg viewBox="95 0 800 800" aria-hidden className={className} {...props}>
      <rect width="1200" height="800" fill="#E30A17" />
      <circle cx="425" cy="400" r="200" fill="#fff" />
      <circle cx="475" cy="400" r="160" fill="#E30A17" />
      <path
        fill="#fff"
        d="M583.334 400 764.235 458.779 652.431 304.894v190.212L764.235 341.221z"
      />
    </svg>
  );
}

/** United Kingdom flag (60×30 artwork), used for English. */
export function UnitedKingdomFlag({ className, ...props }: FlagProps) {
  const id = useId().replace(/[^a-zA-Z0-9_-]/g, "");
  return (
    <svg
      viewBox="0 0 60 30"
      preserveAspectRatio="xMidYMid slice"
      aria-hidden
      className={className}
      {...props}
    >
      <clipPath id={`uk-s-${id}`}>
        <path d="M0 0v30h60V0z" />
      </clipPath>
      <clipPath id={`uk-t-${id}`}>
        <path d="M30 15h30v15zv15H0zH0V0zV0h30z" />
      </clipPath>
      <g clipPath={`url(#uk-s-${id})`}>
        <path d="M0 0v30h60V0z" fill="#012169" />
        <path d="M0 0l60 30m0-30L0 30" stroke="#fff" strokeWidth="6" />
        <path
          d="M0 0l60 30m0-30L0 30"
          clipPath={`url(#uk-t-${id})`}
          stroke="#C8102E"
          strokeWidth="4"
        />
        <path d="M30 0v30M0 15h60" stroke="#fff" strokeWidth="10" />
        <path d="M30 0v30M0 15h60" stroke="#C8102E" strokeWidth="6" />
      </g>
    </svg>
  );
}

const FLAGS: Record<AppLocale, (props: FlagProps) => React.JSX.Element> = {
  tr: TurkeyFlag,
  en: UnitedKingdomFlag,
};

/** Round flag badge for a UI locale. */
export function LocaleFlag({
  locale,
  className,
}: {
  locale: AppLocale;
  className?: string;
}) {
  const Flag = FLAGS[locale];
  return (
    <span
      className={cn(
        "ring-border inline-flex size-5 shrink-0 overflow-hidden rounded-full ring-1",
        className,
      )}
    >
      <Flag className="size-full" />
    </span>
  );
}
