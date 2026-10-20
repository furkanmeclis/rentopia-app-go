"use client";

import { AlertTriangle, CheckCircle2, OctagonAlert } from "lucide-react";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ATTENTION_VISIBLE } from "@/features/platform-overview/constants";
import {
  OverviewPanel,
  PanelNote,
  PanelRowsSkeleton,
} from "@/features/platform-overview/components/overview-panel";
import type {
  AttentionCategory,
  AttentionFailure,
  AttentionItem,
  AttentionSource,
} from "@/features/platform-overview/hooks/use-attention";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const SOURCE_LABEL_KEYS: Record<AttentionSource, string> = {
  organizations: "dashboard.attention.sources.organizations",
  issues: "dashboard.attention.sources.issues",
  domains: "dashboard.attention.sources.domains",
};

type AttentionPanelProps = {
  items: AttentionItem[];
  total: number;
  categories: AttentionCategory[];
  failures: AttentionFailure[];
  isLoading: boolean;
  className?: string;
};

/**
 * One list of what needs a person, critical first: access windows,
 * suspended organizations, open connection issues and custom domains.
 * Every row opens the page where it is fixed.
 */
export function AttentionPanel({
  items,
  total,
  categories,
  failures,
  isLoading,
  className,
}: AttentionPanelProps) {
  const { t } = useLocale();
  const visible = items.slice(0, ATTENTION_VISIBLE);
  const hasDanger = items.some((item) => item.tone === "danger");

  return (
    <OverviewPanel
      title={t("dashboard.attention.title")}
      description={t("dashboard.attention.description")}
      count={isLoading ? undefined : total}
      countTone={hasDanger ? "danger" : "warning"}
      className={className}
    >
      {failures.length > 0 ? (
        <ul className="mb-3 space-y-2">
          {failures.map((failure) => (
            <li
              key={failure.source}
              className="border-destructive/30 bg-destructive/5 flex flex-wrap items-center justify-between gap-2 rounded-xl border px-3 py-2 text-sm"
            >
              <span className="text-destructive flex items-center gap-2">
                <AlertTriangle className="size-4 shrink-0" />
                {t("dashboard.attention.source_failed", {
                  source: t(SOURCE_LABEL_KEYS[failure.source]),
                })}
              </span>
              <Button variant="outline" size="sm" onClick={failure.retry}>
                {t("common.retry")}
              </Button>
            </li>
          ))}
        </ul>
      ) : null}

      {isLoading && items.length === 0 ? (
        <PanelRowsSkeleton rows={4} />
      ) : items.length === 0 ? (
        <PanelNote
          icon={<CheckCircle2 className="size-5" />}
          title={
            failures.length > 0
              ? t("dashboard.attention.empty_partial_title")
              : t("dashboard.attention.empty_title")
          }
          description={
            failures.length > 0
              ? t("dashboard.attention.empty_partial_description")
              : t("dashboard.attention.empty_description")
          }
        />
      ) : (
        <ul className="divide-border -mx-2 divide-y">
          {visible.map((item) => (
            <li key={item.key}>
              <Link
                href={item.href}
                className="hover:bg-muted/60 focus-visible:ring-ring flex items-center gap-3 rounded-xl px-2 py-2.5 outline-none focus-visible:ring-2"
              >
                <span
                  aria-hidden
                  className={cn(
                    "inline-flex size-9 shrink-0 items-center justify-center rounded-full",
                    item.tone === "danger"
                      ? "bg-destructive/10 text-destructive"
                      : "bg-amber-500/10 text-amber-600 dark:text-amber-400",
                  )}
                >
                  {item.tone === "danger" ? (
                    <OctagonAlert className="size-4" />
                  ) : (
                    <AlertTriangle className="size-4" />
                  )}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">
                    {item.title}
                  </span>
                  {item.meta ? (
                    <span className="text-muted-foreground block truncate text-xs">
                      {item.meta}
                    </span>
                  ) : null}
                </span>
                <Badge
                  variant={item.tone}
                  className="shrink-0 whitespace-nowrap"
                >
                  {item.badge}
                </Badge>
              </Link>
            </li>
          ))}
        </ul>
      )}

      {categories.length > 0 ? (
        <div className="mt-auto flex flex-wrap items-center gap-2 pt-4">
          {items.length > visible.length ? (
            <span className="text-muted-foreground mr-1 text-xs">
              {t("dashboard.attention.more", {
                count: items.length - visible.length,
              })}
            </span>
          ) : null}
          {categories.map((category) => (
            <Button
              key={category.source}
              asChild
              variant="outline"
              size="sm"
              className="h-8"
            >
              <Link href={category.href}>
                {t(SOURCE_LABEL_KEYS[category.source])}
                <span className="text-muted-foreground tabular-nums">
                  {category.count.toLocaleString()}
                </span>
              </Link>
            </Button>
          ))}
        </div>
      ) : null}
    </OverviewPanel>
  );
}
