/**
 * Feedback widget loaded from https://issue.technowide.tr/embed.js (root
 * layout). Before the script runs, `window.LinearWidget` may be a plain
 * `{ q: [...] }` queue that the script drains on load.
 */
type LinearWidgetUser = { id: string; name?: string; email?: string };

type LinearWidgetContext = Record<string, string | number | boolean | null>;

type LinearWidgetCall =
  ["setUser", LinearWidgetUser] | ["setContext", LinearWidgetContext];

interface LinearWidgetApi {
  setUser?: (user: LinearWidgetUser) => void;
  setContext?: (context: LinearWidgetContext) => void;
  q?: LinearWidgetCall[];
}

interface Window {
  LinearWidget?: LinearWidgetApi;
}
