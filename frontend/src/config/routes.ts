export const routes = {
  public: {
    root: "/",
    health: "/health",
    share: {
      slug: (slug: string) => `/share/${slug}`,
      signed: (token: string) => `/share/s/${token}`,
    },
  },
  guest: {
    login: "/platform/login",
    register: "/platform/register",
    forgotPassword: "/platform/forgot-password",
    resetPassword: "/platform/reset-password",
    verifyEmail: "/platform/verify-email",
  },
  /**
   * Shell-independent pages for every signed-in user. `/` only routes the
   * user to their home (see `defaultHomeForUser`).
   */
  account: {
    home: "/",
    profile: {
      root: "/profile",
      password: "/profile/password",
      preferences: "/profile/preferences",
    },
    organizations: "/organizations",
    noAccess: "/no-access",
    /** Public staff invitation page (`/v1/invitations/{token}`). */
    invite: (token: string) => `/invite/${encodeURIComponent(token)}`,
  },
  /**
   * Organization (tenant) shell `/t/{slug}`. The slug in the URL picks the
   * organization; API calls carry it as `X-Organization` (see
   * `lib/api/tenant-request.ts`).
   */
  tenant: {
    root: (slug: string) => `/t/${encodeURIComponent(slug)}`,
    home: (slug: string) => `/t/${encodeURIComponent(slug)}`,
    branches: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/branches`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/branches/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/branches/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/branches/${uuid}/edit`,
    },
    exports: {
      /**
       * Downloads a finished export of the organization (the action link of
       * the "export ready" notification; the API needs `X-Organization`).
       */
      download: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/exports/${uuid}/download`,
    },
    fleet: {
      segments: fleetRoutes("segments"),
      groups: fleetRoutes("groups"),
      products: fleetRoutes("products"),
      vehicles: fleetRoutes("vehicles"),
      /** Upcoming / overdue documents and maintenance ("Operasyon özeti"). */
      alerts: (slug: string) => `/t/${encodeURIComponent(slug)}/fleet/alerts`,
      /** Fleet calendar: vehicles × time with blocks, transfers, maintenance. */
      calendar: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/fleet/calendar`,
      transfers: fleetRoutes("transfers"),
      oneWayRules: fleetRoutes("one-way-rules"),
      /** Channel capacity rules (availability engine). */
      capacityRules: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/fleet/capacity-rules`,
    },
    /** Pricing: rate plans (+ matrix), date rules, price preview. */
    pricing: {
      plans: {
        root: (slug: string) =>
          `/t/${encodeURIComponent(slug)}/pricing/rate-plans`,
        detail: (slug: string, uuid: string) =>
          `/t/${encodeURIComponent(slug)}/pricing/rate-plans/${uuid}`,
        matrix: (slug: string) =>
          `/t/${encodeURIComponent(slug)}/pricing/rate-plans/matrix`,
      },
      rules: {
        root: (slug: string) =>
          `/t/${encodeURIComponent(slug)}/pricing/date-rules`,
        detail: (slug: string, uuid: string) =>
          `/t/${encodeURIComponent(slug)}/pricing/date-rules/${uuid}`,
      },
      preview: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/pricing/preview`,
      /** Extras ("Ek hizmetler"): catalog, prices, stock, allocations. */
      extras: {
        root: (slug: string) => `/t/${encodeURIComponent(slug)}/pricing/extras`,
        detail: (slug: string, uuid: string) =>
          `/t/${encodeURIComponent(slug)}/pricing/extras/${uuid}`,
      },
      /** Campaigns and coupons ("Kampanyalar"). */
      campaigns: {
        root: (slug: string) =>
          `/t/${encodeURIComponent(slug)}/pricing/campaigns`,
        detail: (slug: string, uuid: string) =>
          `/t/${encodeURIComponent(slug)}/pricing/campaigns/${uuid}`,
      },
    },
    /** Availability search and holds ("Müsaitlik"). */
    availability: (slug: string) =>
      `/t/${encodeURIComponent(slug)}/availability`,
    settings: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/settings`,
      members: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/settings/members`,
      roles: {
        root: (slug: string) => `/t/${encodeURIComponent(slug)}/settings/roles`,
        create: (slug: string) =>
          `/t/${encodeURIComponent(slug)}/settings/roles/create`,
        detail: (slug: string, uuid: string) =>
          `/t/${encodeURIComponent(slug)}/settings/roles/${uuid}`,
      },
    },
  },
  platform: {
    root: "/platform",
    home: "/platform",
    /**
     * System Settings shell (`/platform/settings/*`). Platform admin pages
     * live here behind one grouped sub-sidebar (`settingsNav`). The legacy
     * top-level URLs redirect permanently via `next.config.ts`.
     */
    settings: {
      root: "/platform/settings",
      system: "/platform/settings/system",
      environment: "/platform/settings/environment",
      general: "/platform/settings/general",
      maintenance: "/platform/settings/maintenance",
      integrations: "/platform/settings/integrations",
      roles: "/platform/settings/roles",
      activity: "/platform/settings/activity",
      logs: "/platform/settings/logs",
      storage: "/platform/settings/storage",
      access: "/platform/settings/access",
      auth: "/platform/settings/auth",
      imports: "/platform/settings/imports",
      exports: "/platform/settings/exports",
    },
    roles: {
      root: "/platform/settings/roles",
      create: "/platform/settings/roles/create",
      detail: (uuid: string) => `/platform/settings/roles/${uuid}`,
      edit: (uuid: string) => `/platform/settings/roles/${uuid}/edit`,
    },
    organizations: {
      root: "/platform/organizations",
      create: "/platform/organizations/create",
      detail: (uuid: string) => `/platform/organizations/${uuid}`,
      edit: (uuid: string) => `/platform/organizations/${uuid}/edit`,
    },
    users: {
      root: "/platform/users",
      create: "/platform/users/create",
      detail: (uuid: string) => `/platform/users/${uuid}`,
      edit: (uuid: string) => `/platform/users/${uuid}/edit`,
    },
    notifications: {
      root: "/platform/notifications",
      detail: (uuid: string) => `/platform/notifications/${uuid}`,
    },
    /** Vehicle catalog (TSB kasko list): tabs brands / models / types / unresolved queue / imports. */
    catalog: {
      root: "/platform/catalog",
      brands: "/platform/catalog/brands",
      models: "/platform/catalog/models",
      types: "/platform/catalog/types",
      typeDetail: (uuid: string) => `/platform/catalog/types/${uuid}`,
      unresolved: "/platform/catalog/unresolved",
      imports: "/platform/catalog/imports",
      importDetail: (uuid: string) => `/platform/catalog/imports/${uuid}`,
    },
    imports: {
      root: "/platform/settings/imports",
      detail: (uuid: string) => `/platform/settings/imports/${uuid}`,
    },
    exports: {
      root: "/platform/settings/exports",
      detail: (uuid: string) => `/platform/settings/exports/${uuid}`,
    },
    access: {
      root: "/platform/settings/access",
    },
    authSettings: {
      root: "/platform/settings/auth",
    },
    integrations: {
      root: "/platform/settings/integrations",
      github: "/platform/settings/integrations?tab=github",
      google: "/platform/settings/integrations?tab=google",
      facebook: "/platform/settings/integrations?tab=facebook",
      apple: "/platform/settings/integrations?tab=apple",
    },
    activity: {
      root: "/platform/settings/activity",
    },
    logs: {
      root: "/platform/settings/logs",
    },
    storage: {
      root: "/platform/settings/storage",
    },
    profile: {
      root: "/platform/profile",
      password: "/platform/profile/password",
      preferences: "/platform/profile/preferences",
    },
  },
  errors: {
    unauthorized: "/unauthorized",
    forbidden: "/forbidden",
  },
} as const;

type NestedRoutes<T> = T extends string
  ? T
  : T extends (...args: never[]) => infer R
    ? R extends string
      ? R
      : never
    : T extends Record<string, unknown>
      ? NestedRoutes<T[keyof T]>
      : never;

export type AppRoute = NestedRoutes<typeof routes>;

/** `/t/{slug}/fleet/{section}` list / create / detail / edit routes. */
function fleetRoutes(
  section:
    | "segments"
    | "groups"
    | "products"
    | "vehicles"
    | "transfers"
    | "one-way-rules",
) {
  const base = (slug: string) =>
    `/t/${encodeURIComponent(slug)}/fleet/${section}`;
  return {
    root: base,
    create: (slug: string) => `${base(slug)}/create`,
    detail: (slug: string, uuid: string) => `${base(slug)}/${uuid}`,
    edit: (slug: string, uuid: string) => `${base(slug)}/${uuid}/edit`,
  };
}
