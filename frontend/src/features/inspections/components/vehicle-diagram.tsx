"use client";

import { type Zone, zoneLabel } from "@/features/inspections/constants";
import type { InspectionDamage } from "@/features/inspections/types";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type Shape =
  | {
      zone: Zone;
      kind: "rect";
      x: number;
      y: number;
      w: number;
      h: number;
      r?: number;
    }
  | {
      zone: Zone;
      kind: "ellipse";
      cx: number;
      cy: number;
      rx: number;
      ry: number;
    };

/**
 * Top view of a car (front up). Each panel is a zone the API knows; the
 * body spans x 30-170, the sides are 30-55 / 145-170.
 */
const SHAPES: Shape[] = [
  { zone: "front_bumper", kind: "rect", x: 40, y: 6, w: 120, h: 22, r: 10 },
  { zone: "hood", kind: "rect", x: 55, y: 30, w: 90, h: 82, r: 6 },
  { zone: "windshield", kind: "rect", x: 58, y: 114, w: 84, h: 30, r: 6 },
  { zone: "roof", kind: "rect", x: 58, y: 146, w: 84, h: 110, r: 4 },
  { zone: "rear_window", kind: "rect", x: 58, y: 258, w: 84, h: 28, r: 6 },
  { zone: "trunk", kind: "rect", x: 55, y: 288, w: 90, h: 62, r: 6 },
  { zone: "rear_bumper", kind: "rect", x: 40, y: 352, w: 120, h: 22, r: 10 },
  { zone: "left_front_fender", kind: "rect", x: 30, y: 30, w: 23, h: 82, r: 6 },
  { zone: "left_front_door", kind: "rect", x: 30, y: 114, w: 23, h: 84 },
  { zone: "left_rear_door", kind: "rect", x: 30, y: 200, w: 23, h: 78 },
  { zone: "left_rear_fender", kind: "rect", x: 30, y: 280, w: 23, h: 70, r: 6 },
  {
    zone: "right_front_fender",
    kind: "rect",
    x: 147,
    y: 30,
    w: 23,
    h: 82,
    r: 6,
  },
  { zone: "right_front_door", kind: "rect", x: 147, y: 114, w: 23, h: 84 },
  { zone: "right_rear_door", kind: "rect", x: 147, y: 200, w: 23, h: 78 },
  {
    zone: "right_rear_fender",
    kind: "rect",
    x: 147,
    y: 280,
    w: 23,
    h: 70,
    r: 6,
  },
  { zone: "left_mirror", kind: "rect", x: 10, y: 120, w: 18, h: 14, r: 4 },
  { zone: "right_mirror", kind: "rect", x: 172, y: 120, w: 18, h: 14, r: 4 },
  { zone: "left_front_wheel", kind: "ellipse", cx: 22, cy: 72, rx: 7, ry: 22 },
  {
    zone: "right_front_wheel",
    kind: "ellipse",
    cx: 178,
    cy: 72,
    rx: 7,
    ry: 22,
  },
  { zone: "left_rear_wheel", kind: "ellipse", cx: 22, cy: 312, rx: 7, ry: 22 },
  {
    zone: "right_rear_wheel",
    kind: "ellipse",
    cx: 178,
    cy: 312,
    rx: 7,
    ry: 22,
  },
];

function center(s: Shape): [number, number] {
  return s.kind === "rect" ? [s.x + s.w / 2, s.y + s.h / 2] : [s.cx, s.cy];
}

/**
 * SVG vehicle diagram: tap a zone to mark a damage. Zones with findings of
 * this form show their count; findings of the handover record (baseline)
 * are outlined so a return can tell new damage from old.
 */
export function VehicleDiagram({
  damages,
  baseline = [],
  selected,
  onSelect,
  disabled = false,
  className,
}: {
  damages: InspectionDamage[];
  baseline?: InspectionDamage[];
  selected?: string | null;
  onSelect?: (zone: Zone) => void;
  disabled?: boolean;
  className?: string;
}) {
  const { t } = useLocale();
  const count = (list: InspectionDamage[], zone: string) =>
    list.filter((d) => d.zone === zone).length;
  return (
    <div className={cn("flex flex-col items-center gap-3", className)}>
      <svg
        viewBox="0 0 200 380"
        className="h-auto w-full max-w-[260px] touch-manipulation"
        role="group"
        aria-label={t("inspections.diagram.label")}
      >
        {SHAPES.map((s) => {
          const own = count(damages, s.zone);
          const old = count(baseline, s.zone);
          const active = selected === s.zone;
          const fill = own
            ? "fill-destructive/30"
            : old
              ? "fill-amber-500/20"
              : "fill-muted";
          const common = {
            className: cn(
              "stroke-foreground/40 transition-colors",
              fill,
              active && "stroke-primary stroke-[3]",
              !disabled && "hover:fill-primary/20 cursor-pointer",
            ),
            strokeWidth: 1.2,
            role: "button" as const,
            tabIndex: disabled ? -1 : 0,
            "aria-label": zoneLabel(t, s.zone),
            "data-zone": s.zone,
            onClick: () => !disabled && onSelect?.(s.zone),
            onKeyDown: (e: React.KeyboardEvent) => {
              if (!disabled && (e.key === "Enter" || e.key === " ")) {
                e.preventDefault();
                onSelect?.(s.zone);
              }
            },
          };
          const [cx, cy] = center(s);
          return (
            <g key={s.zone}>
              {s.kind === "rect" ? (
                <rect
                  x={s.x}
                  y={s.y}
                  width={s.w}
                  height={s.h}
                  rx={s.r ?? 2}
                  {...common}
                />
              ) : (
                <ellipse cx={s.cx} cy={s.cy} rx={s.rx} ry={s.ry} {...common} />
              )}
              {own > 0 ? (
                <g pointerEvents="none">
                  <circle cx={cx} cy={cy} r={8} className="fill-destructive" />
                  <text
                    x={cx}
                    y={cy + 3.5}
                    textAnchor="middle"
                    className="fill-white text-[10px] font-semibold"
                  >
                    {own}
                  </text>
                </g>
              ) : old > 0 ? (
                <circle
                  cx={cx}
                  cy={cy}
                  r={5}
                  pointerEvents="none"
                  className="fill-none stroke-amber-600"
                  strokeWidth={2}
                />
              ) : null}
            </g>
          );
        })}
      </svg>
      <div className="text-muted-foreground flex flex-wrap justify-center gap-x-4 gap-y-1 text-xs">
        <span className="flex items-center gap-1">
          <span className="bg-destructive inline-block size-2.5 rounded-full" />
          {t("inspections.diagram.this_form")}
        </span>
        {baseline.length > 0 ? (
          <span className="flex items-center gap-1">
            <span className="inline-block size-2.5 rounded-full border-2 border-amber-600" />
            {t("inspections.diagram.handover")}
          </span>
        ) : null}
      </div>
    </div>
  );
}
