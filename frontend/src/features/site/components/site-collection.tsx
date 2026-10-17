"use client";

import { ArrowDown, ArrowUp, Plus, Trash2 } from "lucide-react";
import type { ReactNode } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/**
 * Shared chrome of the editor's repeatable items (FAQ, slides, advantages,
 * highlights, menus): numbered card with up / down / remove, an empty
 * state, the list error and an add button that stops at the limit.
 */
export function CollectionList({
  count,
  max,
  emptyTitle,
  emptyDescription,
  addLabel,
  onAdd,
  error,
  disabled,
  children,
}: {
  count: number;
  max: number;
  emptyTitle: string;
  emptyDescription?: string;
  addLabel: string;
  onAdd: () => void;
  error?: string;
  disabled?: boolean;
  children: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <div className="space-y-4">
      {count === 0 ? (
        <EmptyState title={emptyTitle} description={emptyDescription} />
      ) : (
        <ol className="space-y-4">{children}</ol>
      )}
      {error ? <p className="text-destructive text-sm">{error}</p> : null}
      {!disabled ? (
        <div className="flex flex-wrap items-center gap-3">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={count >= max}
            onClick={onAdd}
          >
            <Plus className="size-4" />
            {addLabel}
          </Button>
          <span className="text-muted-foreground text-xs tabular-nums">
            {t("site.collection.count", { count, max })}
          </span>
        </div>
      ) : null}
    </div>
  );
}

export function CollectionItem({
  title,
  index,
  count,
  onMove,
  onRemove,
  canRemove = true,
  disabled,
  error,
  extra,
  children,
}: {
  title: string;
  index: number;
  count: number;
  onMove: (from: number, to: number) => void;
  onRemove: () => void;
  canRemove?: boolean;
  disabled?: boolean;
  error?: string;
  /** Controls next to the title (e.g. an enabled switch). */
  extra?: ReactNode;
  children: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <li className="bg-muted/30 space-y-3 rounded-lg border p-4">
      <div className="flex items-center justify-between gap-2">
        <span className="text-muted-foreground text-sm font-medium tabular-nums">
          {title}
        </span>
        <div className="flex items-center gap-1">
          {extra}
          {!disabled ? (
            <>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                disabled={index === 0}
                aria-label={t("site.collection.up")}
                onClick={() => onMove(index, index - 1)}
              >
                <ArrowUp className="size-4" />
              </Button>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                disabled={index === count - 1}
                aria-label={t("site.collection.down")}
                onClick={() => onMove(index, index + 1)}
              >
                <ArrowDown className="size-4" />
              </Button>
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                disabled={!canRemove}
                aria-label={t("site.collection.remove")}
                onClick={onRemove}
              >
                <Trash2 className="size-4" />
              </Button>
            </>
          ) : null}
        </div>
      </div>
      {error ? <p className="text-destructive text-sm">{error}</p> : null}
      {children}
    </li>
  );
}
