"use client";

import { Link2, TriangleAlert } from "lucide-react";
import { useState } from "react";

import { PermissionGuard } from "@/components/common/permission-guard";
import { AsyncCombobox } from "@/components/ui/async-combobox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { permissions } from "@/config/permissions";
import { useBrandModelOptions } from "@/features/catalog/components/model-dialogs";
import { usePatchType } from "@/features/catalog/hooks/use-catalog";
import type { CatalogType } from "@/features/catalog/services/catalog.service";
import { useLocale } from "@/providers/locale-provider";

type TypeModelAssignProps = {
  type: Pick<CatalogType, "uuid" | "raw_name" | "brand" | "model">;
  /** Compact = table cell (model badge + small trigger). */
  compact?: boolean;
};

/**
 * Inline model assignment for a catalog type: pick a model of the same
 * brand and optionally store a prefix rule so later imports (and the other
 * unresolved / auto types of the brand) follow it.
 */
export function TypeModelAssign({ type, compact }: TypeModelAssignProps) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const [modelUuid, setModelUuid] = useState(type.model?.uuid ?? "");
  const [saveRule, setSaveRule] = useState(false);
  const [prefix, setPrefix] = useState("");
  const patch = usePatchType();
  const { loadOptions, options } = useBrandModelOptions(
    open ? type.brand.uuid : undefined,
  );

  const selectedLabel =
    options.find((o) => o.value === modelUuid)?.label ?? type.model?.name ?? "";

  const label = type.model ? (
    <Badge
      variant={type.model.source === "auto" ? "outline" : "secondary"}
      className="max-w-48 truncate font-normal"
      title={type.model.name}
    >
      {type.model.name}
    </Badge>
  ) : (
    <Badge variant="warning" className="gap-1 font-normal">
      <TriangleAlert className="size-3" />
      {t("catalog.types.unresolved")}
    </Badge>
  );

  return (
    <div
      className="flex items-center gap-1"
      onClick={(event) => event.stopPropagation()}
    >
      {label}
      <PermissionGuard permission={permissions.catalog.write}>
        <Popover
          open={open}
          onOpenChange={(next) => {
            setOpen(next);
            if (next) {
              setModelUuid(type.model?.uuid ?? "");
              setSaveRule(false);
              setPrefix("");
            }
          }}
        >
          <PopoverTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size={compact ? "icon" : "sm"}
              className={compact ? "size-7" : undefined}
              aria-label={t("catalog.types.assign_model")}
              title={t("catalog.types.assign_model")}
            >
              <Link2 className="size-4" />
              {compact ? null : t("catalog.types.assign_model")}
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-80 space-y-3" align="start">
            <div className="space-y-1">
              <p className="text-sm font-medium">
                {t("catalog.types.assign_model")}
              </p>
              <p className="text-muted-foreground text-xs break-words">
                {type.raw_name}
              </p>
            </div>
            <AsyncCombobox
              value={modelUuid}
              onValueChange={(value) => {
                setModelUuid(value);
                const name = options.find((o) => o.value === value)?.label;
                if (name && !prefix) setPrefix(name);
              }}
              loadOptions={loadOptions}
              options={options}
              placeholder={t("catalog.picker.model_placeholder")}
              searchPlaceholder={t("catalog.picker.search")}
              emptyText={t("catalog.picker.empty")}
              clearable
            />
            <div className="flex items-start gap-2">
              <Checkbox
                id={`save-rule-${type.uuid}`}
                checked={saveRule}
                disabled={!modelUuid}
                onCheckedChange={(checked) => setSaveRule(checked === true)}
              />
              <div className="space-y-1">
                <Label htmlFor={`save-rule-${type.uuid}`}>
                  {t("catalog.types.save_rule")}
                </Label>
                <p className="text-muted-foreground text-xs">
                  {t("catalog.types.save_rule_hint")}
                </p>
              </div>
            </div>
            {saveRule ? (
              <div className="space-y-1">
                <Label htmlFor={`rule-prefix-${type.uuid}`}>
                  {t("catalog.types.rule_prefix")}
                </Label>
                <Input
                  id={`rule-prefix-${type.uuid}`}
                  value={prefix}
                  placeholder={selectedLabel}
                  onChange={(event) => setPrefix(event.target.value)}
                />
              </div>
            ) : null}
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setOpen(false)}
              >
                {t("form.cancel")}
              </Button>
              <Button
                type="button"
                size="sm"
                disabled={patch.isPending}
                onClick={async () => {
                  await patch.mutateAsync({
                    uuid: type.uuid,
                    body: {
                      model_uuid: modelUuid || null,
                      ...(saveRule && modelUuid
                        ? { save_rule: true, rule_prefix: prefix.trim() }
                        : {}),
                    },
                  });
                  setOpen(false);
                }}
              >
                {t("form.save")}
              </Button>
            </div>
          </PopoverContent>
        </Popover>
      </PermissionGuard>
    </div>
  );
}
