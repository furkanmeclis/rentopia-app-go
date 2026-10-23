"use client";

import * as Sentry from "@sentry/nextjs";
import { useEffect } from "react";

/**
 * Root error boundary (replaces the root layout when it throws). Server
 * errors are reported by instrumentation.ts (onRequestError); this reports
 * what the browser sees when the error tracker runs there. Bilingual: no
 * site context here.
 */
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
          <h1 style={{ fontSize: 20 }}>
            Bir sorun oluştu / Something went wrong
          </h1>
          <button type="button" onClick={reset} style={{ marginTop: 16 }}>
            Tekrar dene / Try again
          </button>
        </div>
      </body>
    </html>
  );
}
