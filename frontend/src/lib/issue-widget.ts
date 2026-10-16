import pkg from "../../package.json";

/**
 * Feedback widget (Linear issue requests). The script builds its own button
 * and iframe; its styles live in Shadow DOM / the iframe. Origin, Linear team
 * and project mapping and the API key stay on the widget server.
 */
export const ISSUE_WIDGET_SRC = "https://issue.technowide.tr/embed.js";

export const ISSUE_WIDGET_RELEASE = pkg.version;

export const ISSUE_WIDGET_ENVIRONMENT =
  process.env.NODE_ENV === "production" ? "staging" : "development";

/**
 * Calls the widget API, or queues the call when the script has not run yet
 * (it drains `window.LinearWidget.q` on load).
 */
export function callIssueWidget(call: LinearWidgetCall) {
  if (typeof window === "undefined") return;
  const widget = window.LinearWidget;
  const [method, arg] = call;
  const fn = widget?.[method] as ((value: typeof arg) => void) | undefined;
  if (typeof fn === "function") {
    fn.call(widget, arg);
    return;
  }
  const target = widget ?? (window.LinearWidget = {});
  (target.q ??= []).push(call);
}
