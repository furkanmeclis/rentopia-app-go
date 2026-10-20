"use client";

import { ArrowUpRight } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { Card } from "@/components/common/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type OverviewPanelProps = {
  title: string;
  description?: string;
  count?: number;
  countTone?: "secondary" | "warning" | "danger";
  link?: { href: string; label: string };
  className?: string;
  children: ReactNode;
};

/** Section shell of the overview: title row, optional count and link. */
export function OverviewPanel({
  title,
  description,
  count,
  countTone = "secondary",
  link,
  className,
  children,
}: OverviewPanelProps) {
  return (
    <Card className={cn("flex min-w-0 flex-col", className)}>
      <div className="flex items-start justify-between gap-3 px-5 pt-4 pb-3">
        <div className="min-w-0 space-y-0.5">
          <h2 className="flex items-center gap-2 text-base font-semibold tracking-tight">
            {title}
            {count != null ? (
              <Badge
                variant={count > 0 ? countTone : "secondary"}
                className="tabular-nums"
              >
                {count.toLocaleString()}
              </Badge>
            ) : null}
          </h2>
          {description ? (
            <p className="text-muted-foreground text-sm">{description}</p>
          ) : null}
        </div>
        {link ? (
          <Button
            asChild
            variant="ghost"
            size="sm"
            className="text-muted-foreground -mr-2 shrink-0"
          >
            <Link href={link.href}>
              {link.label}
              <ArrowUpRight className="size-3.5" />
            </Link>
          </Button>
        ) : null}
      </div>
      <div className="flex flex-1 flex-col px-5 pb-5">{children}</div>
    </Card>
  );
}

/** Skeleton rows matching `PanelRow`. */
export function PanelRowsSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <ul className="space-y-3 py-1" aria-busy>
      {Array.from({ length: rows }, (_, i) => (
        <li key={i} className="flex items-center gap-3">
          <Skeleton className="size-9 shrink-0 rounded-full" />
          <span className="flex-1 space-y-1.5">
            <Skeleton className="h-3.5 w-2/5" />
            <Skeleton className="h-3 w-3/5" />
          </span>
          <Skeleton className="h-5 w-14 rounded-md" />
        </li>
      ))}
    </ul>
  );
}

/** Quiet empty / error note inside a panel. */
export function PanelNote({
  title,
  description,
  action,
  icon,
  tone = "default",
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  icon?: ReactNode;
  tone?: "default" | "danger";
}) {
  return (
    <div
      className={cn(
        "flex flex-1 flex-col items-center justify-center gap-1.5 rounded-xl border border-dashed px-4 py-8 text-center",
        tone === "danger" && "border-destructive/40",
      )}
    >
      {icon ? (
        <span
          className={cn(
            "mb-1 inline-flex size-9 items-center justify-center rounded-full",
            tone === "danger"
              ? "bg-destructive/10 text-destructive"
              : "bg-muted text-muted-foreground",
          )}
        >
          {icon}
        </span>
      ) : null}
      <p className="text-sm font-medium">{title}</p>
      {description ? (
        <p className="text-muted-foreground max-w-xs text-sm">{description}</p>
      ) : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}

/** Panel error with retry. */
export function PanelError({
  description,
  onRetry,
}: {
  description: string;
  onRetry: () => void;
}) {
  const { t } = useLocale();
  return (
    <PanelNote
      tone="danger"
      title={t("common.error_generic")}
      description={description}
      action={
        <Button variant="outline" size="sm" onClick={onRetry}>
          {t("common.retry")}
        </Button>
      }
    />
  );
}
