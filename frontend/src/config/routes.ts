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
    customers: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/customers`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/customers/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/customers/${uuid}`,
      edit: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/customers/${uuid}/edit`,
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
    /** Reservations ("Rezervasyonlar"): list, wizard, detail, quotes, tasks. */
    reservations: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/reservations`,
      create: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/create`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/${uuid}`,
      quotes: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/quotes`,
      tasks: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/tasks`,
      /** Staff approval queue ("Onay kuyruğu", TEC-708). */
      approvals: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/approvals`,
      /** Requests from the customer portal ("Müşteri talepleri", TEC-715). */
      customerRequests: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/reservations/requests`,
    },
    /** Rentals ("Kiralamalar"): list and detail. */
    rentals: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/rentals`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/rentals/${uuid}`,
    },
    /** Finance ("Finans", TEC-717 / TEC-720). */
    finance: {
      accounts: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/accounts`,
      account: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/finance/accounts/${uuid}`,
      cashAccounts: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/cash-accounts`,
      cashAccount: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/finance/cash-accounts/${uuid}`,
      movements: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/movements`,
      expenses: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/expenses`,
      categories: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/expense-categories`,
      reports: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/reports`,
      /** Suppliers, their invoices / payments and due payables (TEC-721). */
      suppliers: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/suppliers`,
      supplier: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/finance/suppliers/${uuid}`,
      supplierInvoices: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/supplier-invoices`,
      supplierPayments: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/supplier-payments`,
      payables: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/finance/payables`,
    },
    /** HGS / OGS toll passes and their review queue (TEC-725). */
    kabis: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/kabis`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/kabis/${uuid}`,
    },
    tolls: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/tolls`,
      review: (slug: string) => `/t/${encodeURIComponent(slug)}/tolls/review`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/tolls/${uuid}`,
    },
    /** Mobile handover / return ("Saha görevleri", TEC-712 / TEC-713). */
    field: {
      /** "Görevlerim": the member's own field tasks (today, tomorrow). */
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/field`,
      /** One field task (phone layout). */
      task: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/field/my/${uuid}`,
      /** "Görev panosu": the assignment board and the staff timeline. */
      board: (slug: string) => `/t/${encodeURIComponent(slug)}/field/board`,
      handover: (slug: string, rentalUuid: string) =>
        `/t/${encodeURIComponent(slug)}/field/handover/${rentalUuid}`,
      return: (slug: string, rentalUuid: string) =>
        `/t/${encodeURIComponent(slug)}/field/return/${rentalUuid}`,
    },
    /** Handover / return forms and the review queue. */
    inspections: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/inspections`,
      detail: (slug: string, uuid: string) =>
        `/t/${encodeURIComponent(slug)}/inspections/${uuid}`,
    },
    /** Operation tasks of the branches ("Operasyon görevleri"). */
    operationTasks: (slug: string) =>
      `/t/${encodeURIComponent(slug)}/operation-tasks`,
    /** Availability search and holds ("Müsaitlik"). */
    availability: (slug: string) =>
      `/t/${encodeURIComponent(slug)}/availability`,
    settings: {
      root: (slug: string) => `/t/${encodeURIComponent(slug)}/settings`,
      members: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/settings/members`,
      /** Provider accounts ("Entegrasyonlar", TEC-676). */
      integrations: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/settings/integrations`,
      roles: {
        root: (slug: string) => `/t/${encodeURIComponent(slug)}/settings/roles`,
        create: (slug: string) =>
          `/t/${encodeURIComponent(slug)}/settings/roles/create`,
        detail: (slug: string, uuid: string) =>
          `/t/${encodeURIComponent(slug)}/settings/roles/${uuid}`,
      },
      /** Inspection templates ("Teslim / iade formları"). */
      inspectionTemplates: (slug: string) =>
        `/t/${encodeURIComponent(slug)}/settings/inspection-templates`,
      /** Contract templates ("Sözleşme şablonları"). */
      contractTemplates: {
        root: (slug: string) =>
          `/t/${encodeURIComponent(slug)}/settings/contract-templates`,
        create: (slug: string) =>
          `/t/${encodeURIComponent(slug)}/settings/contract-templates/create`,
        detail: (slug: string, uuid: string) =>
          `/t/${encodeURIComponent(slug)}/settings/contract-templates/${uuid}`,
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
