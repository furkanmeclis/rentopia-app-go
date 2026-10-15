"use client";

import { Braces } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  PLACEHOLDER_GROUP_LABEL_KEYS,
  placeholderLabel,
} from "@/features/contracts/constants";
import { usePlaceholders } from "@/features/contracts/hooks/use-contracts";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Alan ekle": the placeholder catalog grouped (company, renter, vehicle,
 * dates, amounts, policies…); picking one inserts `{{group.key}}` at the
 * caret. Table placeholders render server-built HTML tables.
 */
export function PlaceholderPicker({
  onInsert,
  disabled = false,
}: {
  onInsert: (token: string) => void;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const query = usePlaceholders();
  const [open, setOpen] = useState(false);
  const groups = query.data?.groups ?? [];
  const items = query.data?.items ?? [];
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" size="sm" variant="outline" disabled={disabled}>
          <Braces className="size-4" />
          {t("contracts.editor.insert_placeholder")}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-96 p-0" align="start">
        <Command>
          <CommandInput
            placeholder={t("contracts.editor.search_placeholder")}
          />
          <CommandList className="max-h-96">
            <CommandEmpty>{t("contracts.editor.no_placeholder")}</CommandEmpty>
            {groups.map((group) => (
              <CommandGroup
                key={group}
                heading={
                  PLACEHOLDER_GROUP_LABEL_KEYS[group]
                    ? t(PLACEHOLDER_GROUP_LABEL_KEYS[group])
                    : group
                }
              >
                {items
                  .filter((p) => p.group === group)
                  .map((p) => (
                    <CommandItem
                      key={p.key}
                      value={`${p.key} ${placeholderLabel(t, p.key)}`}
                      onSelect={() => {
                        onInsert(`{{${p.key}}}`);
                        setOpen(false);
                      }}
                    >
                      <span className="flex min-w-0 flex-1 flex-col">
                        <span className="flex items-center gap-1.5 text-sm">
                          {placeholderLabel(t, p.key)}
                          {p.html ? (
                            <Badge
                              variant="outline"
                              className="px-1 py-0 text-[10px]"
                            >
                              {t("contracts.editor.table_badge")}
                            </Badge>
                          ) : null}
                        </span>
                        <span className="text-muted-foreground truncate font-mono text-xs">
                          {`{{${p.key}}}`}
                          {p.sample ? ` · ${p.sample}` : ""}
                        </span>
                      </span>
                    </CommandItem>
                  ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
