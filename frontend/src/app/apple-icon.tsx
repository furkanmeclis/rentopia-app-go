import { ImageResponse } from "next/og";

import { RENTOPYA_PATHS } from "@/components/brand/rentopya-paths";
import { brand } from "@/config/brand";

export const size = { width: 180, height: 180 };
export const contentType = "image/png";

/**
 * Apple touch icon — the brand kit app icon: white "R" on the indigo → ink
 * gradient square (iOS rounds the corners itself).
 */
export default function AppleIcon() {
  return new ImageResponse(
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        backgroundImage: `linear-gradient(to top, ${brand.colors.ink}, ${brand.colors.primary})`,
      }}
    >
      <svg width="180" height="180" viewBox="0 0 850 850">
        <path
          fill={brand.colors.white}
          fillRule="evenodd"
          d={RENTOPYA_PATHS.icon.d}
          transform={RENTOPYA_PATHS.icon.transform}
        />
      </svg>
    </div>,
    { ...size },
  );
}
