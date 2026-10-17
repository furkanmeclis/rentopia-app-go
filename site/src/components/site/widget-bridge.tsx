"use client";

import { usePathname, useSearchParams } from "next/navigation";
import { useEffect } from "react";

import { allowedParent } from "@/lib/widget";

export function WidgetBridge({ domains }: { domains: string[] }) {
  const params = useSearchParams();
  const pathname = usePathname();
  const parent = allowedParent(params.get("parent"), domains);
  const widgetId = (params.get("widgetId") || "").slice(0, 100);
  useEffect(() => {
    if (!parent || !widgetId || window.parent === window) return;
    const send = (type: string, payload: unknown) =>
      window.parent.postMessage(
        { source: "rentopia-widget", v: 1, type, widgetId, payload },
        parent,
      );
    const root =
      document.querySelector<HTMLElement>("[data-widget-root]") ||
      document.body;
    let errorSent = false;
    const resize = () =>
      send("resize", {
        height: Math.ceil(root.getBoundingClientRect().height),
      });
    send("ready", {});
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(root);
    let completedSent = "";
    let paymentSent = "";
    const events = () => {
      if (!errorSent && root.querySelector("[data-widget-error]")) {
        errorSent = true;
        send("error", { code: "RENDER_FAILED" });
      }
      const completed = root.querySelector<HTMLElement>(
        "[data-widget-reference]",
      );
      const reference = completed?.dataset.widgetReference;
      if (reference && reference !== completedSent) {
        completedSent = reference;
        send("completed", {
          reference,
          status: completed.dataset.widgetStatus,
        });
      }
      const payment = root.querySelector<HTMLAnchorElement>(
        "[data-widget-payment]",
      );
      if (payment && payment.href !== paymentSent) {
        paymentSent = payment.href;
        send("navigate", {
          url: payment.href,
          target: "_top",
          reason: "payment",
        });
      }
    };
    events();
    const changes = new MutationObserver(events);
    changes.observe(root, { childList: true, subtree: true });
    const receive = (event: MessageEvent) => {
      if (event.origin !== parent || event.source !== window.parent) return;
      const msg = event.data;
      if (
        !msg ||
        msg.source !== "rentopia-host" ||
        msg.v !== 1 ||
        msg.widgetId !== widgetId ||
        !["init", "filters"].includes(msg.type)
      )
        return;
      // Filters only change the search form; they cannot navigate or supply credentials.
      if (pathname !== "/embed" && pathname !== "/embed/") return;
      const payload = msg.payload;
      if (!payload || typeof payload !== "object") return;
      for (const name of [
        "pickup",
        "return",
        "pickup_date",
        "return_date",
        "pickup_time",
        "return_time",
        "subgroup",
      ]) {
        const value = payload[name];
        const input = document.querySelector<
          HTMLInputElement | HTMLSelectElement
        >(`[name="${name}"]`);
        if (input && typeof value === "string" && value.length <= 100)
          input.value = value;
      }
    };
    window.addEventListener("message", receive);
    return () => {
      observer.disconnect();
      changes.disconnect();
      window.removeEventListener("message", receive);
    };
  }, [parent, widgetId, pathname, params]);
  return null;
}
