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
    <div className="mx-auto max-w-xl space-y-4 px-4 py-20 text-center">
      <h1 className="text-2xl font-bold">
        Bir sorun oluştu / Something went wrong
      </h1>
      <p className="text-muted-foreground">
        Lütfen biraz sonra tekrar deneyin. Please try again shortly.
      </p>
      <Button onClick={reset}>Tekrar dene / Try again</Button>
    </div>
  );
}
