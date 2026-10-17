import Image, { type StaticImageData } from "next/image";

import egm from "@/assets/brand-icons/egm.svg";
import gib from "@/assets/brand-icons/gib.svg";
import hgs from "@/assets/brand-icons/hgs.png";
import iyzico from "@/assets/brand-icons/iyzico.svg";
import netgsm from "@/assets/brand-icons/netgsm.svg";
import obilet from "@/assets/brand-icons/obilet.svg";
import whatsapp from "@/assets/brand-icons/whatsapp.svg";
import yolcu360 from "@/assets/brand-icons/yolcu360.svg";
import { cn } from "@/lib/utils";

/**
 * Official logos of external services (sources and usage notes in
 * assets/brand-icons/SOURCES.md). Keyed by integration provider code; the
 * fake providers show the service they stand in for.
 */
const PROVIDER_LOGOS: Record<string, { src: StaticImageData; alt: string }> = {
  iyzico: { src: iyzico, alt: "iyzico" },
  netgsm: { src: netgsm, alt: "Netgsm" },
  hgs_fake: { src: hgs, alt: "HGS" },
  kabis: { src: egm, alt: "Emniyet Genel Müdürlüğü" },
  kabis_fake: { src: egm, alt: "Emniyet Genel Müdürlüğü" },
  einvoice: { src: gib, alt: "Gelir İdaresi Başkanlığı" },
  einvoice_fake: { src: gib, alt: "Gelir İdaresi Başkanlığı" },
  whatsapp: { src: whatsapp, alt: "WhatsApp" },
  yolcu360: { src: yolcu360, alt: "Yolcu360" },
  yolcu360_fake: { src: yolcu360, alt: "Yolcu360" },
  obilet: { src: obilet, alt: "obilet" },
  obilet_fake: { src: obilet, alt: "obilet" },
};

export function hasBrandLogo(provider: string | null | undefined): boolean {
  return Boolean(provider && PROVIDER_LOGOS[provider]);
}

/**
 * The provider's logo on a light chip, so it keeps its own colours in dark
 * mode as the brand guides ask. Renders nothing for an unknown provider.
 */
export function BrandLogo({
  provider,
  className,
}: {
  provider: string | null | undefined;
  className?: string;
}) {
  const logo = provider ? PROVIDER_LOGOS[provider] : undefined;
  if (!logo) return null;
  return (
    <span
      className={cn(
        "inline-flex h-8 w-14 shrink-0 items-center justify-center rounded-md border bg-white p-1",
        className,
      )}
    >
      <Image
        src={logo.src}
        alt={logo.alt}
        unoptimized
        className="h-auto max-h-full w-auto max-w-full object-contain"
      />
    </span>
  );
}
