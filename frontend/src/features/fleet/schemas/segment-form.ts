import { z } from "zod";

import { CODE_PATTERN } from "@/features/fleet/constants";
import type { Segment, SegmentCreateRequest } from "@/features/fleet/types";

type Translate = (key: string) => string;

export function createSegmentFormSchema(t: Translate) {
  return z.object({
    code: z
      .string()
      .trim()
      .min(1, t("fleet.validation.code_required"))
      .regex(CODE_PATTERN, t("fleet.validation.code_format")),
    name: z
      .string()
      .trim()
      .min(1, t("fleet.validation.name_required"))
      .max(255),
    description: z.string().max(2000),
    sort_order: z
      .string()
      .trim()
      .refine((v) => {
        const n = Number(v);
        return v !== "" && Number.isInteger(n) && n >= 0 && n <= 100000;
      }, t("fleet.validation.sort_order")),
    is_active: z.boolean(),
  });
}

export type SegmentFormValues = z.infer<
  ReturnType<typeof createSegmentFormSchema>
>;

export function segmentFormDefaults(segment?: Segment): SegmentFormValues {
  return {
    code: segment?.code ?? "",
    name: segment?.name ?? "",
    description: segment?.description ?? "",
    sort_order: String(segment?.sort_order ?? 0),
    is_active: segment?.is_active ?? true,
  };
}

export function segmentFormToRequest(
  values: SegmentFormValues,
): SegmentCreateRequest {
  return {
    code: values.code.trim().toUpperCase(),
    name: values.name.trim(),
    description: values.description.trim(),
    sort_order: Number(values.sort_order),
    is_active: values.is_active,
  };
}
