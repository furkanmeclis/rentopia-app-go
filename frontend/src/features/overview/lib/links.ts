import { routes } from "@/config/routes";

type Query = Record<string, string | undefined>;

function withQuery(path: string, query: Query): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value) search.set(key, value);
  }
  const q = search.toString();
  return q ? `${path}?${q}` : path;
}

/**
 * "Tümünü gör" targets: the full list pages with the overview's filters
 * applied (read by `useUrlListDefaults` on the list page).
 */
export const overviewLinks = {
  todayOperations: (
    slug: string,
    branch: string | undefined,
    day: { start: string; end: string },
  ) =>
    withQuery(routes.tenant.operationTasks(slug), {
      task_type: "handover,return",
      branch,
      due_from: day.start,
      due_to: day.end,
      sort: "due_at",
    }),
  overdueReturns: (slug: string, branch: string | undefined) =>
    withQuery(routes.tenant.rentals.root(slug), {
      status: "active",
      branch,
      sort: "planned_return_at",
    }),
  approvals: (slug: string) => routes.tenant.reservations.approvals(slug),
  awaitingPayment: (slug: string, branch: string | undefined) =>
    withQuery(routes.tenant.reservations.root(slug), {
      status: "awaiting_payment",
      branch,
      sort: "starts_at",
    }),
  resolutionTasks: (slug: string) =>
    withQuery(routes.tenant.reservations.tasks(slug), { status: "open" }),
  integrationJobs: (slug: string) => routes.tenant.settings.integrations(slug),
  documents: (slug: string, branch: string | undefined) =>
    withQuery(routes.tenant.reservations.root(slug), {
      status: "confirmed",
      branch,
      sort: "starts_at",
    }),
};
