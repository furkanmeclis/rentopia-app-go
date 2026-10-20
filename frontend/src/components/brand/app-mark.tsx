import type { BrandTone } from "@/config/brand";
import { brand } from "@/config/brand";
import { cn } from "@/lib/utils";

import { RentopyaMarkSvg } from "./brand-svg";

type AppMarkProps = {
  title?: string;
  tone?: BrandTone;
  className?: string;
};

/** The Rentopya "R" glyph (indigo on light, white on dark with `auto`). */
export function AppMark({ className, title = brand.name, tone }: AppMarkProps) {
  return (
    <RentopyaMarkSvg
      title={title}
      tone={tone}
      className={cn("size-4", className)}
    />
  );
}
