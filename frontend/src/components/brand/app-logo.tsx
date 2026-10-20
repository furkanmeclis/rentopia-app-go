import { brand, type BrandTone, type BrandVariant } from "@/config/brand";
import { cn } from "@/lib/utils";

import {
  RentopyaIconSvg,
  RentopyaLogoSvg,
  RentopyaMarkSvg,
  RentopyaStackedSvg,
  RentopyaWordSvg,
} from "./brand-svg";

type AppLogoProps = {
  /**
   * `logo` → horizontal lockup, `stacked` → vertical lockup, `wordmark` →
   * letters only, `mark` → "R" glyph, `icon` → app icon tile.
   */
  variant?: BrandVariant;
  /**
   * Background the logo sits on: `light` → colourful, `dark` → white,
   * `auto` → follows the colour mode. Ignored by `icon`.
   */
  tone?: BrandTone;
  className?: string;
  title?: string;
};

/** Rentopya logo in any of the brand kit's variants. */
export function AppLogo({
  variant = "logo",
  tone = "auto",
  className,
  title = brand.name,
}: AppLogoProps) {
  switch (variant) {
    case "icon":
      return (
        <RentopyaIconSvg
          title={title}
          className={cn("size-8 rounded-lg", className)}
        />
      );
    case "mark":
      return (
        <RentopyaMarkSvg
          title={title}
          tone={tone}
          className={cn("size-4", className)}
        />
      );
    case "wordmark":
      return (
        <RentopyaWordSvg
          title={title}
          tone={tone}
          className={cn("h-5 w-auto", className)}
        />
      );
    case "stacked":
      return (
        <RentopyaStackedSvg
          title={title}
          tone={tone}
          className={cn("h-20 w-auto", className)}
        />
      );
    default:
      return (
        <RentopyaLogoSvg
          title={title}
          tone={tone}
          className={cn("h-7 w-auto", className)}
        />
      );
  }
}
