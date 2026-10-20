import { brand, type BrandTone } from "@/config/brand";
import { cn } from "@/lib/utils";

import { RentopyaLogoSvg } from "./brand-svg";

type AppWordmarkProps = {
  title?: string;
  tone?: BrandTone;
  className?: string;
};

/** Horizontal Rentopya lockup for auth, payment and approval pages. */
export function AppWordmark({
  className,
  title = brand.productName,
  tone = "auto",
}: AppWordmarkProps) {
  return (
    <RentopyaLogoSvg
      title={title}
      tone={tone}
      className={cn("h-8 w-auto", className)}
    />
  );
}
