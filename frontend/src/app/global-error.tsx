"use client";

import * as Sentry from "@sentry/nextjs";
import { useEffect } from "react";

import { RentopyaIconSvg } from "@/components/brand/brand-svg";
import { brand } from "@/config/brand";

/** Root error boundary (replaces the root layout when it throws). */
export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    Sentry.captureException(error);
  }, [error]);

  return (
    <html lang="tr">
      <body
        style={{
          fontFamily: "system-ui, sans-serif",
          display: "grid",
          placeItems: "center",
          minHeight: "100vh",
          margin: 0,
        }}
      >
        <div style={{ textAlign: "center" }}>
          <RentopyaIconSvg
            title={brand.name}
            width={48}
            height={48}
            style={{ borderRadius: 12 }}
          />
          <h1 style={{ fontSize: 20 }}>
            Bir şeyler ters gitti / Something went wrong
          </h1>
          <button type="button" onClick={reset} style={{ marginTop: 16 }}>
            Tekrar dene / Try again
          </button>
        </div>
      </body>
    </html>
  );
}
