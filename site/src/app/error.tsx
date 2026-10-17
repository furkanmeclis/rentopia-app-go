"use client";

import { Button } from "@/components/ui/button";

/** Rendering failed (the API was unreachable, …). Bilingual: no context here. */
export default function ErrorPage({
  reset,
}: {
  error: Error;
  reset: () => void;
}) {
  return (
    <div className="rt-container rt-narrow">
      <div className="rt-empty">
        <h1>Bir sorun oluştu / Something went wrong</h1>
        <p>Lütfen biraz sonra tekrar deneyin. Please try again shortly.</p>
        <Button onClick={reset}>Tekrar dene / Try again</Button>
      </div>
    </div>
  );
}
