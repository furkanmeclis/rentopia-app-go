"use client";

import ErrorPage from "@/app/error";

export default function EmbedError(props: { error: Error; reset: () => void }) {
  return (
    <div data-widget-error>
      <ErrorPage {...props} />
    </div>
  );
}
