"use client";

import type { ReactNode } from "react";
import { Controller, get, useFormContext, useWatch } from "react-hook-form";

import { FormFieldShell } from "@/components/forms/form-field";
import { Button } from "@/components/ui/button";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/** Swatches offered in the color popover (any #rrggbb can be typed). */
const SWATCHES = [
  "#0f172a",
  "#334155",
  "#64748b",
  "#e2e8f0",
  "#f8fafc",
  "#ffffff",
  "#1d4ed8",
  "#2563eb",
  "#0891b2",
  "#059669",
  "#16a34a",
  "#65a30d",
  "#ca8a04",
  "#f59e0b",
  "#ea580c",
  "#dc2626",
  "#db2777",
  "#9333ea",
  "#7c3aed",
  "#4f46e5",
  "#0b1220",
  "#1e293b",
  "#94a3b8",
  "#60a5fa",
];

const HEX = /^#[0-9a-fA-F]{6}$/;

/**
 * `#rrggbb` color field: a swatch that opens a palette (popover) plus the
 * hex value as text. Built from shadcn primitives (no native color input).
 */
export function ColorField({
  name,
  label,
  disabled,
}: {
  name: string;
  label: string;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const {
    control,
    formState: { errors },
  } = useFormContext();
  const error = get(errors, name)?.message as string | undefined;
  return (
    <FormFieldShell name={name} label={label} error={error}>
      <Controller
        name={name}
        control={control}
        render={({ field }) => {
          const value = String(field.value ?? "");
          return (
            <InputGroup>
              <InputGroupAddon align="inline-start">
                <Popover>
                  <PopoverTrigger asChild>
                    <Button
                      type="button"
                      variant="outline"
                      size="icon"
                      disabled={disabled}
                      aria-label={t("site.theme.pick_color", { label })}
                      className="size-6 shrink-0 rounded p-0"
                      style={{
                        backgroundColor: HEX.test(value)
                          ? value
                          : "transparent",
                      }}
                    />
                  </PopoverTrigger>
                  <PopoverContent className="w-64" align="start">
                    <div className="grid grid-cols-6 gap-2">
                      {SWATCHES.map((swatch) => (
                        <Button
                          key={swatch}
                          type="button"
                          variant="outline"
                          size="icon"
                          aria-label={swatch}
                          aria-pressed={value.toLowerCase() === swatch}
                          className={cn(
                            "size-8 rounded p-0 transition-transform hover:scale-110",
                            value.toLowerCase() === swatch &&
                              "ring-ring ring-2 ring-offset-2",
                          )}
                          style={{ backgroundColor: swatch }}
                          onClick={() => field.onChange(swatch)}
                        />
                      ))}
                    </div>
                  </PopoverContent>
                </Popover>
              </InputGroupAddon>
              <InputGroupInput
                id={name}
                value={value}
                onChange={(e) => field.onChange(e.target.value)}
                onBlur={field.onBlur}
                placeholder="#1d4ed8"
                maxLength={7}
                spellCheck={false}
                className="font-mono"
                disabled={disabled}
                aria-invalid={Boolean(error)}
              />
            </InputGroup>
          );
        }}
      />
    </FormFieldShell>
  );
}

export type ChoiceOption = {
  value: string;
  label: string;
  description?: string;
  preview?: ReactNode;
};

/** Visual choice cards (layout variants, mode): one radio group. */
export function ChoiceCards({
  name,
  label,
  description,
  options,
  disabled,
  columns = 4,
}: {
  name: string;
  label: string;
  description?: string;
  options: ChoiceOption[];
  disabled?: boolean;
  columns?: 2 | 3 | 4;
}) {
  const {
    control,
    formState: { errors },
  } = useFormContext();
  const error = get(errors, name)?.message as string | undefined;
  return (
    <FormFieldShell
      name={name}
      label={label}
      description={description}
      error={error}
    >
      <Controller
        name={name}
        control={control}
        render={({ field }) => (
          <RadioGroup
            value={field.value}
            onValueChange={field.onChange}
            disabled={disabled}
            className={cn(
              "grid gap-3 sm:grid-cols-2",
              columns === 3 && "lg:grid-cols-3",
              columns === 4 && "lg:grid-cols-4",
            )}
            aria-invalid={Boolean(error)}
          >
            {options.map((option) => {
              const id = `${name}-${option.value}`;
              const selected = field.value === option.value;
              return (
                <Label
                  key={option.value}
                  htmlFor={id}
                  className={cn(
                    "bg-card hover:bg-accent/40 flex cursor-pointer flex-col items-stretch gap-2 rounded-lg border p-3 font-normal transition-colors",
                    selected && "border-primary ring-primary/30 ring-2",
                    disabled && "cursor-not-allowed opacity-60",
                  )}
                >
                  {option.preview ? (
                    <div
                      aria-hidden
                      className="bg-muted/50 flex h-20 overflow-hidden rounded-md border p-2"
                    >
                      {option.preview}
                    </div>
                  ) : null}
                  <span className="flex items-start gap-2">
                    <RadioGroupItem
                      id={id}
                      value={option.value}
                      className="mt-0.5"
                    />
                    <span className="grid gap-0.5">
                      <span className="text-sm font-medium">
                        {option.label}
                      </span>
                      {option.description ? (
                        <span className="text-muted-foreground text-xs">
                          {option.description}
                        </span>
                      ) : null}
                    </span>
                  </span>
                </Label>
              );
            })}
          </RadioGroup>
        )}
      />
    </FormFieldShell>
  );
}

/** "12 / 70" counter of a text field (SEO, hero). */
export function LengthHint({ name, max }: { name: string; max: number }) {
  const { t } = useLocale();
  const value = useWatch({ name }) as string | undefined;
  const length = (value ?? "").length;
  return (
    <span
      className={cn(
        "tabular-nums",
        length > max ? "text-destructive" : "text-muted-foreground",
      )}
    >
      {t("site.length", { length, max })}
    </span>
  );
}
