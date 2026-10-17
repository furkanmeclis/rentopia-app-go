import { cva, type VariantProps } from "class-variance-authority";
import * as React from "react";

import { cn } from "@/lib/cn";

/**
 * Button styles. The classes are the site's semantic contract (`rt-*`);
 * each theme stylesheet (src/themes/{id}) draws them from the theme tokens.
 */
export const buttonVariants = cva("rt-button", {
  variants: {
    variant: {
      default: "rt-button--primary",
      accent: "rt-button--accent",
      outline: "rt-button--outline",
      ghost: "rt-button--ghost",
      link: "rt-button--link",
    },
    size: {
      default: "",
      sm: "rt-button--sm",
      lg: "rt-button--lg",
    },
  },
  defaultVariants: { variant: "default", size: "default" },
});

export type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> &
  VariantProps<typeof buttonVariants>;

export function Button({
  className,
  variant,
  size,
  type,
  ...props
}: ButtonProps) {
  return (
    <button
      type={type ?? "button"}
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  );
}
