import * as React from "react";

import { cn } from "@/lib/cn";

export function Card({ className, ...props }: React.ComponentProps<"div">) {
  return (
    <div
      className={cn(
        "bg-background text-foreground border-border flex flex-col gap-4 rounded-[calc(var(--radius)*1.5)] border p-5 shadow-sm",
        className,
      )}
      {...props}
    />
  );
}

export function CardTitle({ className, ...props }: React.ComponentProps<"h2">) {
  return (
    <h2
      className={cn("text-lg leading-tight font-semibold", className)}
      {...props}
    />
  );
}

export function Alert({
  className,
  tone = "info",
  ...props
}: React.ComponentProps<"div"> & { tone?: "info" | "error" | "success" }) {
  return (
    <div
      role={tone === "error" ? "alert" : "status"}
      className={cn(
        "rounded-[var(--radius)] border px-4 py-3 text-sm",
        tone === "error" && "border-red-300 bg-red-50 text-red-900",
        tone === "success" &&
          "border-emerald-300 bg-emerald-50 text-emerald-900",
        tone === "info" && "border-border bg-muted text-foreground",
        className,
      )}
      {...props}
    />
  );
}

export function Badge({ className, ...props }: React.ComponentProps<"span">) {
  return (
    <span
      className={cn(
        "bg-muted text-foreground inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium",
        className,
      )}
      {...props}
    />
  );
}
