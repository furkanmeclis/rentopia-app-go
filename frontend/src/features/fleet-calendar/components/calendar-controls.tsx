"use client";

import { parseISO } from "date-fns";
import {
  Check,
  ChevronLeft,
  ChevronRight,
  CirclePlus,
  GripVertical,
  Search,
  X,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "@/components/ui/command";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  CALENDAR_ZOOMS,
  LEGEND_ENTRIES,
  ZOOM_LABEL_KEYS,
} from "@/features/fleet-calendar/constants";
import {
  type CalendarWindow,
  toDayValue,
} from "@/features/fleet-calendar/lib/timeline";
import type { CalendarZoom } from "@/features/fleet-calendar/types";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** "14 Ekim 2026, Çarşamba" / "12 – 18 Eki 2026" / "Ekim 2026". */
export function windowLabel(
  zoom: CalendarZoom,
  window: CalendarWindow,
  locale: "tr" | "en",
) {
  if (zoom === "day") return datetime(window.from, "d MMMM yyyy, EEEE", locale);
  if (zoom === "month") return datetime(window.from, "LLLL yyyy", locale);
  const last = new Date(window.to.getTime() - 1);
  return `${datetime(window.from, "d MMM", locale)} – ${datetime(last, "d MMM yyyy", locale)}`;
}

/** Prev / today / next, a day picker to jump and the zoom switch. */
export function CalendarNavigation({
  zoom,
  window,
  anchor,
  onAnchorChange,
  onShift,
  onZoomChange,
}: {
  zoom: CalendarZoom;
  window: CalendarWindow;
  anchor: Date;
  onAnchorChange: (anchor: Date) => void;
  onShift: (direction: -1 | 1) => void;
  onZoomChange: (zoom: CalendarZoom) => void;
}) {
  const { t, locale } = useLocale();
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="flex items-center gap-1">
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="size-9"
          aria-label={t("fleet_calendar.nav.previous")}
          onClick={() => onShift(-1)}
        >
          <ChevronLeft className="size-4" />
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-9"
          onClick={() => onAnchorChange(new Date())}
        >
          {t("fleet_calendar.nav.today")}
        </Button>
        <Button
          type="button"
          size="icon"
          variant="outline"
          className="size-9"
          aria-label={t("fleet_calendar.nav.next")}
          onClick={() => onShift(1)}
        >
          <ChevronRight className="size-4" />
        </Button>
      </div>
      <DatePicker
        className="h-9 w-44"
        value={toDayValue(anchor)}
        onChange={(value) => {
          if (value) onAnchorChange(parseISO(value));
        }}
        placeholder={t("fleet_calendar.nav.jump")}
      />
      <h2
        className="font-display min-w-0 grow text-base font-semibold capitalize"
        aria-live="polite"
      >
        {windowLabel(zoom, window, locale)}
      </h2>
      <ToggleGroup
        type="single"
        variant="outline"
        size="sm"
        value={zoom}
        onValueChange={(value) => {
          if (value) onZoomChange(value as CalendarZoom);
        }}
        aria-label={t("fleet_calendar.zoom.label")}
      >
        {CALENDAR_ZOOMS.map((value) => (
          <ToggleGroupItem key={value} value={value} className="px-3">
            {t(ZOOM_LABEL_KEYS[value])}
          </ToggleGroupItem>
        ))}
      </ToggleGroup>
    </div>
  );
}

/**
 * Colour legend. Reservation bars include the return branch's preparation
 * buffer; bars with a provisional plate can be dragged onto another vehicle
 * row ("plaka değişimi").
 */
export function CalendarLegend({ className }: { className?: string }) {
  const { t } = useLocale();
  return (
    <ul
      className={cn(
        "text-muted-foreground flex flex-wrap items-center gap-x-4 gap-y-2 text-xs",
        className,
      )}
      aria-label={t("fleet_calendar.legend.title")}
    >
      {LEGEND_ENTRIES.map((entry) => (
        <li key={entry.id}>
          <span className="inline-flex items-center gap-1.5">
            <span
              aria-hidden
              className={cn(
                "inline-block h-3 w-5 rounded-sm border",
                entry.id === "maintenance" && "size-3 rounded-full border-2",
                entry.swatch,
              )}
            />
            {t(entry.labelKey)}
          </span>
        </li>
      ))}
      <li>
        <Tooltip>
          <TooltipTrigger asChild>
            <span tabIndex={0} className="inline-flex items-center gap-1">
              <GripVertical className="size-3.5" aria-hidden />
              {t("fleet_calendar.legend.drag")}
            </span>
          </TooltipTrigger>
          <TooltipContent className="max-w-xs">
            {t("fleet_calendar.legend.drag_hint")}
          </TooltipContent>
        </Tooltip>
      </li>
    </ul>
  );
}

export type FacetOption = { value: string; label: string; count?: number };

/** Multi-select facet (same look as the DataTable toolbar filters). */
export function FacetFilter({
  title,
  options,
  value,
  onChange,
}: {
  title: string;
  options: FacetOption[];
  value: string[];
  onChange: (value: string[]) => void;
}) {
  const { t } = useLocale();
  const selected = new Set(value);
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-9 border-dashed"
        >
          <CirclePlus className="size-3.5" />
          {title}
          {selected.size > 0 ? (
            <>
              <Separator orientation="vertical" className="mx-1 h-4" />
              <Badge
                variant="secondary"
                className="rounded-sm px-1 font-normal"
              >
                {selected.size > 2
                  ? t("table.faceted_selected", { count: selected.size })
                  : options
                      .filter((o) => selected.has(o.value))
                      .map((o) => o.label)
                      .join(", ")}
              </Badge>
            </>
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-60 p-0" align="start">
        <Command>
          <CommandInput placeholder={title} />
          <CommandList>
            <CommandEmpty>{t("table.faceted_empty")}</CommandEmpty>
            <CommandGroup>
              {options.map((option) => {
                const isSelected = selected.has(option.value);
                return (
                  <CommandItem
                    key={option.value}
                    value={`${option.label} ${option.value}`}
                    onSelect={() => {
                      const next = new Set(selected);
                      if (isSelected) next.delete(option.value);
                      else next.add(option.value);
                      onChange(Array.from(next));
                    }}
                  >
                    <div
                      className={cn(
                        "border-primary flex size-4 items-center justify-center rounded-sm border",
                        isSelected
                          ? "bg-primary text-primary-foreground"
                          : "opacity-50 [&_svg]:invisible",
                      )}
                    >
                      <Check className="size-3" />
                    </div>
                    <span className="truncate">{option.label}</span>
                    {option.count != null ? (
                      <span className="text-muted-foreground ms-auto font-mono text-xs">
                        {option.count}
                      </span>
                    ) : null}
                  </CommandItem>
                );
              })}
            </CommandGroup>
            {selected.size > 0 ? (
              <>
                <CommandSeparator />
                <CommandGroup>
                  <CommandItem
                    onSelect={() => onChange([])}
                    className="justify-center text-center"
                  >
                    {t("table.clear_filter")}
                  </CommandItem>
                </CommandGroup>
              </>
            ) : null}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

/** Plate search box with a clear button. */
export function PlateSearch({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useLocale();
  return (
    <div className="relative w-full sm:w-56">
      <Search
        className="text-muted-foreground absolute start-2.5 top-1/2 size-4 -translate-y-1/2"
        aria-hidden
      />
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={t("fleet_calendar.filters.plate")}
        aria-label={t("fleet_calendar.filters.plate")}
        className="h-9 ps-8 pe-8"
      />
      {value ? (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="absolute end-0.5 top-1/2 size-8 -translate-y-1/2"
          aria-label={t("form.clear")}
          onClick={() => onChange("")}
        >
          <X className="size-4" />
        </Button>
      ) : null}
    </div>
  );
}
