import * as React from "react";

import { cn } from "@/lib/cn";

/**
 * Form controls. The site's forms post to server actions and work without
 * JavaScript, so selects and date fields are the native controls
 * (accessible, with the platform pickers on phones). The classes are the
 * semantic contract (`rt-*`) each theme stylesheet draws.
 */
const field = "rt-input";

export function Input({
  className,
  ...props
}: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input className={cn(field, className)} {...props} />;
}

export function Select({
  className,
  ...props
}: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={cn(field, "rt-select", className)} {...props} />;
}

export function Textarea({
  className,
  ...props
}: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea className={cn(field, "rt-textarea", className)} {...props} />
  );
}

export function Label({
  className,
  ...props
}: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return <label className={cn("rt-label", className)} {...props} />;
}

export function Checkbox({
  className,
  ...props
}: Omit<React.InputHTMLAttributes<HTMLInputElement>, "type">) {
  return (
    <input
      type="checkbox"
      className={cn("rt-checkbox", className)}
      {...props}
    />
  );
}

/** Label + control + error, wired with aria attributes. */
export function Field({
  id,
  label,
  error,
  hint,
  children,
  className,
}: {
  id: string;
  label: string;
  error?: string;
  hint?: string;
  children: React.ReactElement<{
    id?: string;
    "aria-invalid"?: boolean;
    "aria-describedby"?: string;
  }>;
  className?: string;
}) {
  const describedBy = [error ? `${id}-error` : "", hint ? `${id}-hint` : ""]
    .filter(Boolean)
    .join(" ");
  return (
    <div className={cn("rt-field", className)}>
      <Label htmlFor={id}>{label}</Label>
      {React.cloneElement(children, {
        id,
        "aria-invalid": error ? true : undefined,
        "aria-describedby": describedBy || undefined,
      })}
      {hint ? (
        <p id={`${id}-hint`} className="rt-field-hint">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={`${id}-error`} className="rt-field-error" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
}
