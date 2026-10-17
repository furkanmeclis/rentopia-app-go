import * as React from "react";

import { cn } from "@/lib/cn";

/** Surfaces of the semantic class contract (`rt-*`, styled per theme). */
export function Card({ className, ...props }: React.ComponentProps<"div">) {
  return <div className={cn("rt-card", className)} {...props} />;
}

export function CardTitle({ className, ...props }: React.ComponentProps<"h2">) {
  return <h2 className={cn("rt-card-title", className)} {...props} />;
}

export function Alert({
  className,
  tone = "info",
  ...props
}: React.ComponentProps<"div"> & { tone?: "info" | "error" | "success" }) {
  return (
    <div
      role={tone === "error" ? "alert" : "status"}
      className={cn("rt-alert", `rt-alert--${tone}`, className)}
      {...props}
    />
  );
}

export function Badge({ className, ...props }: React.ComponentProps<"span">) {
  return <span className={cn("rt-badge", className)} {...props} />;
}
