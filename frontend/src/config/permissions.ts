/**
 * Permission catalog aligned with platform RBAC seed permissions.
 */
export const Permission = {
  PlatformUsersRead: "platform.users.read",
  PlatformUsersWrite: "platform.users.write",
  PlatformUsersExport: "platform.users.export",
  PlatformUsersImport: "platform.users.import",
  PlatformRolesRead: "platform.roles.read",
  PlatformRolesWrite: "platform.roles.write",
  PlatformRolesExport: "platform.roles.export",
  PlatformRolesImport: "platform.roles.import",
  PlatformOrganizationsRead: "platform.organizations.read",
  PlatformOrganizationsWrite: "platform.organizations.write",
  PlatformOrganizationsExport: "platform.organizations.export",
  PlatformOrganizationsBulkActivate: "platform.organizations.bulk.activate",
  PlatformOrganizationsBulkSuspend: "platform.organizations.bulk.suspend",
  PlatformCatalogRead: "platform.catalog.read",
  PlatformCatalogWrite: "platform.catalog.write",
  PlatformNotificationsRead: "platform.notifications.read",
  PlatformNotificationsReadAll: "platform.notifications.read_all",
  PlatformNotificationsExport: "platform.notifications.export",
  PlatformSettingsRead: "platform.settings.read",
  PlatformSettingsWrite: "platform.settings.write",
  PlatformActivityRead: "platform.activity.read",
  PlatformImportsRead: "platform.imports.read",
  PlatformExportsRead: "platform.exports.read",
  PlatformStorageRead: "platform.storage.read",
  PlatformStorageWrite: "platform.storage.write",
  PlatformLogsRead: "platform.logs.read",
  PlatformLogsWrite: "platform.logs.write",
  PlatformUsersBulkDisable: "platform.users.bulk.disable",
  PlatformUsersBulkEnable: "platform.users.bulk.enable",
  PlatformUsersImpersonate: "platform.users.impersonate",
  PlatformRolesBulkDelete: "platform.roles.bulk.delete",
  PlatformBulkRead: "platform.bulk.read",
  PlatformAccessRead: "platform.access.read",
  PlatformAccessWrite: "platform.access.write",
  PlatformIntegrationsGitHubRead: "platform.integrations.github.read",
  PlatformIntegrationsGitHubWrite: "platform.integrations.github.write",
  PlatformIntegrationsGoogleRead: "platform.integrations.google.read",
  PlatformIntegrationsGoogleWrite: "platform.integrations.google.write",
  PlatformIntegrationsFacebookRead: "platform.integrations.facebook.read",
  PlatformIntegrationsFacebookWrite: "platform.integrations.facebook.write",
  PlatformIntegrationsAppleRead: "platform.integrations.apple.read",
  PlatformIntegrationsAppleWrite: "platform.integrations.apple.write",
  PlatformAuthSettingsRead: "platform.auth.settings.read",
  PlatformAuthSettingsWrite: "platform.auth.settings.write",

  AuthSession: "auth.session",

  NotificationsRead: "notifications.read",
  NotificationsManage: "notifications.manage",

  TenantSettingsRead: "tenant.settings.read",
  TenantSettingsWrite: "tenant.settings.write",
  TenantMembersRead: "tenant.members.read",
  TenantMembersWrite: "tenant.members.write",
  TenantRolesRead: "tenant.roles.read",
  TenantRolesWrite: "tenant.roles.write",
  TenantBranchesRead: "tenant.branches.read",
  TenantBranchesWrite: "tenant.branches.write",
  TenantBranchesExport: "tenant.branches.export",
  TenantBranchesImport: "tenant.branches.import",
  TenantBranchesBulkActivate: "tenant.branches.bulk.activate",
  TenantBranchesBulkDeactivate: "tenant.branches.bulk.deactivate",
  TenantFleetRead: "tenant.fleet.read",
  TenantFleetWrite: "tenant.fleet.write",
  TenantVehiclesRead: "tenant.vehicles.read",
  TenantVehiclesWrite: "tenant.vehicles.write",
  TenantVehiclesExport: "tenant.vehicles.export",
  TenantVehiclesImport: "tenant.vehicles.import",
  TenantVehiclesBulk: "tenant.vehicles.bulk",
  TenantBlocksRead: "tenant.blocks.read",
  TenantBlocksWrite: "tenant.blocks.write",
  TenantTransfersRead: "tenant.transfers.read",
  TenantTransfersWrite: "tenant.transfers.write",
  TenantOneWayRead: "tenant.one_way.read",
  TenantOneWayWrite: "tenant.one_way.write",
  TenantPricingRead: "tenant.pricing.read",
  TenantPricingWrite: "tenant.pricing.write",
  TenantPricingActivate: "tenant.pricing.activate",
  TenantAvailabilityRead: "tenant.availability.read",
  TenantAvailabilityHold: "tenant.availability.hold",
  TenantCapacityRulesRead: "tenant.capacity_rules.read",
  TenantCapacityRulesWrite: "tenant.capacity_rules.write",
  TenantCustomersRead: "tenant.customers.read",
  TenantCustomersWrite: "tenant.customers.write",
  TenantCustomersReveal: "tenant.customers.reveal",
  TenantCustomersExport: "tenant.customers.export",
  TenantCustomersImport: "tenant.customers.import",
  TenantCustomersAnonymize: "tenant.customers.anonymize",
  TenantReservationsRead: "tenant.reservations.read",
  TenantReservationsWrite: "tenant.reservations.write",
  TenantReservationsCancel: "tenant.reservations.cancel",
  TenantReservationsConfirm: "tenant.reservations.confirm",
  TenantReservationsExport: "tenant.reservations.export",
  TenantReservationsOverrideEligibility:
    "tenant.reservations.override_eligibility",
  TenantQuotesRead: "tenant.quotes.read",
  TenantQuotesWrite: "tenant.quotes.write",
  TenantResolutionTasksRead: "tenant.resolution_tasks.read",
  TenantResolutionTasksWrite: "tenant.resolution_tasks.write",
  TenantRentalsRead: "tenant.rentals.read",
  TenantRentalsWrite: "tenant.rentals.write",
  TenantRentalsClose: "tenant.rentals.close",
  TenantFieldTasksRead: "tenant.field_tasks.read",
  TenantFieldTasksAssign: "tenant.field_tasks.assign",
  TenantFieldTasksExecute: "tenant.field_tasks.execute",
  TenantContractsRead: "tenant.contracts.read",
  TenantContractsWrite: "tenant.contracts.write",
  TenantContractsActivate: "tenant.contracts.activate",
  TenantExtrasRead: "tenant.extras.read",
  TenantExtrasWrite: "tenant.extras.write",
  TenantCampaignsRead: "tenant.campaigns.read",
  TenantCampaignsWrite: "tenant.campaigns.write",
  TenantPriceOverride: "tenant.price.override",
  TenantDiscountApply: "tenant.discount.apply",
  TenantPaymentCollect: "tenant.payment.collect",
  TenantPaymentRefund: "tenant.payment.refund",
  TenantDamageApprove: "tenant.damage.approve",
  TenantSignatureWaive: "tenant.signature.waive",
  TenantDocumentDownload: "tenant.document.download",
  TenantReportExport: "tenant.report.export",
  TenantIntegrationsManage: "tenant.integrations.manage",
  TenantPaymentsRead: "tenant.payments.read",
  TenantPaymentsCreateLink: "tenant.payments.create_link",
  TenantReservationsApprove: "tenant.reservations.approve",
  TenantTollsRead: "tenant.tolls.read",
  TenantTollsExport: "tenant.tolls.export",
  TenantTollsImport: "tenant.tolls.import",
  TenantTollsReview: "tenant.tolls.review",
  TenantKabisRead: "tenant.kabis.read",
  TenantKabisRetry: "tenant.kabis.retry",
  TenantKabisReconcile: "tenant.kabis.reconcile",
} as const;

export type PermissionSlug = (typeof Permission)[keyof typeof Permission];

export const ALL_PERMISSIONS: readonly PermissionSlug[] = Object.values(
  Permission,
).sort((a, b) => a.localeCompare(b));

const knownPermissionSet = new Set<string>(ALL_PERMISSIONS);

export function isKnownPermission(slug: string): slug is PermissionSlug {
  return knownPermissionSet.has(slug);
}

export const permissions = {
  users: {
    read: Permission.PlatformUsersRead,
    write: Permission.PlatformUsersWrite,
    export: Permission.PlatformUsersExport,
    import: Permission.PlatformUsersImport,
    bulkDisable: Permission.PlatformUsersBulkDisable,
    bulkEnable: Permission.PlatformUsersBulkEnable,
    impersonate: Permission.PlatformUsersImpersonate,
  },
  roles: {
    read: Permission.PlatformRolesRead,
    write: Permission.PlatformRolesWrite,
    export: Permission.PlatformRolesExport,
    import: Permission.PlatformRolesImport,
    bulkDelete: Permission.PlatformRolesBulkDelete,
  },
  organizations: {
    read: Permission.PlatformOrganizationsRead,
    write: Permission.PlatformOrganizationsWrite,
    export: Permission.PlatformOrganizationsExport,
    bulkActivate: Permission.PlatformOrganizationsBulkActivate,
    bulkSuspend: Permission.PlatformOrganizationsBulkSuspend,
  },
  catalog: {
    read: Permission.PlatformCatalogRead,
    write: Permission.PlatformCatalogWrite,
  },
  notifications: {
    read: Permission.NotificationsRead,
    manage: Permission.NotificationsManage,
    platformRead: Permission.PlatformNotificationsRead,
    platformReadAll: Permission.PlatformNotificationsReadAll,
    export: Permission.PlatformNotificationsExport,
  },
  settings: {
    read: Permission.PlatformSettingsRead,
    write: Permission.PlatformSettingsWrite,
  },
  activity: {
    read: Permission.PlatformActivityRead,
  },
  imports: {
    read: Permission.PlatformImportsRead,
  },
  exports: {
    read: Permission.PlatformExportsRead,
  },
  storage: {
    read: Permission.PlatformStorageRead,
    write: Permission.PlatformStorageWrite,
  },
  logs: {
    read: Permission.PlatformLogsRead,
    write: Permission.PlatformLogsWrite,
  },
  bulk: {
    read: Permission.PlatformBulkRead,
  },
  access: {
    read: Permission.PlatformAccessRead,
    write: Permission.PlatformAccessWrite,
  },
  integrations: {
    github: {
      read: Permission.PlatformIntegrationsGitHubRead,
      write: Permission.PlatformIntegrationsGitHubWrite,
    },
    google: {
      read: Permission.PlatformIntegrationsGoogleRead,
      write: Permission.PlatformIntegrationsGoogleWrite,
    },
    facebook: {
      read: Permission.PlatformIntegrationsFacebookRead,
      write: Permission.PlatformIntegrationsFacebookWrite,
    },
    apple: {
      read: Permission.PlatformIntegrationsAppleRead,
      write: Permission.PlatformIntegrationsAppleWrite,
    },
  },
  authSettings: {
    read: Permission.PlatformAuthSettingsRead,
    write: Permission.PlatformAuthSettingsWrite,
  },
  auth: {
    session: Permission.AuthSession,
  },
  tenant: {
    settings: {
      read: Permission.TenantSettingsRead,
      write: Permission.TenantSettingsWrite,
    },
    members: {
      read: Permission.TenantMembersRead,
      write: Permission.TenantMembersWrite,
    },
    roles: {
      read: Permission.TenantRolesRead,
      write: Permission.TenantRolesWrite,
    },
    branches: {
      read: Permission.TenantBranchesRead,
      write: Permission.TenantBranchesWrite,
      export: Permission.TenantBranchesExport,
      import: Permission.TenantBranchesImport,
      bulkActivate: Permission.TenantBranchesBulkActivate,
      bulkDeactivate: Permission.TenantBranchesBulkDeactivate,
    },
    /** Segments, rental groups and rentable products (one slug pair). */
    fleet: {
      read: Permission.TenantFleetRead,
      write: Permission.TenantFleetWrite,
    },
    /** Physical vehicles; every bulk action shares `bulk`. */
    vehicles: {
      read: Permission.TenantVehiclesRead,
      write: Permission.TenantVehiclesWrite,
      export: Permission.TenantVehiclesExport,
      import: Permission.TenantVehiclesImport,
      bulk: Permission.TenantVehiclesBulk,
    },
    /** Vehicle usage blocks (maintenance, damage, …); "Bakıma al" bulk = write. */
    blocks: {
      read: Permission.TenantBlocksRead,
      write: Permission.TenantBlocksWrite,
    },
    /** Transfers between branches; export needs read. */
    transfers: {
      read: Permission.TenantTransfersRead,
      write: Permission.TenantTransfersWrite,
    },
    /** One-way rules; export needs read, bulk activate / deactivate write. */
    oneWay: {
      read: Permission.TenantOneWayRead,
      write: Permission.TenantOneWayWrite,
    },
    /**
     * Rate plans, date rules and the price preview. `write` edits drafts
     * only; `activate` puts a draft live (separate so sales staff cannot
     * change prices).
     */
    pricing: {
      read: Permission.TenantPricingRead,
      write: Permission.TenantPricingWrite,
      activate: Permission.TenantPricingActivate,
    },
    /** Availability engine: capacity search (read) and holds ("opsiyon"). */
    availability: {
      read: Permission.TenantAvailabilityRead,
      hold: Permission.TenantAvailabilityHold,
    },
    /** Channel capacity rules (sales limit, guaranteed quota, safety margin). */
    capacityRules: {
      read: Permission.TenantCapacityRulesRead,
      write: Permission.TenantCapacityRulesWrite,
    },
    /**
     * Customers and drivers: read shows masked identity / contact values;
     * `reveal` (plaintext, with a reason), `export`, `import` and
     * `anonymize` are granted one by one.
     */
    customers: {
      read: Permission.TenantCustomersRead,
      write: Permission.TenantCustomersWrite,
      reveal: Permission.TenantCustomersReveal,
      export: Permission.TenantCustomersExport,
      import: Permission.TenantCustomersImport,
      anonymize: Permission.TenantCustomersAnonymize,
    },
    /**
     * Reservations: `cancel` (cancel, no-show), `confirm` (approve,
     * confirm), `export` and the driver eligibility override are granted
     * one by one.
     */
    reservations: {
      read: Permission.TenantReservationsRead,
      write: Permission.TenantReservationsWrite,
      cancel: Permission.TenantReservationsCancel,
      confirm: Permission.TenantReservationsConfirm,
      export: Permission.TenantReservationsExport,
      overrideEligibility: Permission.TenantReservationsOverrideEligibility,
      /** Staff approval queue: accept / reject requests and reviews. */
      approve: Permission.TenantReservationsApprove,
    },
    /** Quotes (priced searches; no capacity). */
    quotes: {
      read: Permission.TenantQuotesRead,
      write: Permission.TenantQuotesWrite,
    },
    /** Resolution tasks of broken vehicle plans. */
    resolutionTasks: {
      read: Permission.TenantResolutionTasksRead,
      write: Permission.TenantResolutionTasksWrite,
    },
    /**
     * Rentals and operation tasks: `close` finishes the return check,
     * cancels before the handover and decides charges (damage approvals
     * need `damage.approve`).
     */
    rentals: {
      read: Permission.TenantRentalsRead,
      write: Permission.TenantRentalsWrite,
      close: Permission.TenantRentalsClose,
    },
    /**
     * Field tasks: `read` = the assignment board, `assign` = plan, assign
     * (bulk), re-plan and cancel, `execute` = "my tasks" (start, report an
     * issue, complete the member's own tasks).
     */
    fieldTasks: {
      read: Permission.TenantFieldTasksRead,
      assign: Permission.TenantFieldTasksAssign,
      execute: Permission.TenantFieldTasksExecute,
    },
    /**
     * Contract templates and documents: `activate` makes a template
     * version active; preparing documents and recording approvals is
     * `contracts.write` or `rentals.write`; PDFs need `document.download`.
     */
    contracts: {
      read: Permission.TenantContractsRead,
      write: Permission.TenantContractsWrite,
      activate: Permission.TenantContractsActivate,
    },
    /**
     * Extras catalog, per-product prices and branch stock. Equipment holds
     * use `availability.hold`.
     */
    extras: {
      read: Permission.TenantExtrasRead,
      write: Permission.TenantExtrasWrite,
    },
    /** Campaigns and coupon codes; using a code at sale time needs none. */
    campaigns: {
      read: Permission.TenantCampaignsRead,
      write: Permission.TenantCampaignsWrite,
    },
    price: {
      override: Permission.TenantPriceOverride,
    },
    discount: {
      apply: Permission.TenantDiscountApply,
    },
    /**
     * Payments: `collect` records a collection at the branch, `refund`
     * refunds through the provider (TEC-689 slugs reused).
     */
    payment: {
      collect: Permission.TenantPaymentCollect,
      refund: Permission.TenantPaymentRefund,
    },
    /** Payments list / detail and pay-by-link links (TEC-707). */
    payments: {
      read: Permission.TenantPaymentsRead,
      createLink: Permission.TenantPaymentsCreateLink,
    },
    /** Provider accounts (iyzico …): connect, test, disable (TEC-676). */
    integrations: {
      manage: Permission.TenantIntegrationsManage,
    },
    /**
     * HGS / OGS toll passes (TEC-725): list and review queue (`read`),
     * export, file import and "fetch now" (`import`), assign / unassign /
     * ignore / rematch with a reason (`review`). Vehicle tags need
     * `vehicles.write`, the processing fee `settings.write`.
     */
    tolls: {
      read: Permission.TenantTollsRead,
      export: Permission.TenantTollsExport,
      import: Permission.TenantTollsImport,
      review: Permission.TenantTollsReview,
    },
    /**
     * KABİS notifications (TEC-726): the screen and the rental chip,
     * retry (status check first), mark manual / reconcile. The KABİS
     * account is an integration account (`integrations.manage`).
     */
    kabis: {
      read: Permission.TenantKabisRead,
      retry: Permission.TenantKabisRetry,
      reconcile: Permission.TenantKabisReconcile,
    },
    damage: {
      approve: Permission.TenantDamageApprove,
    },
    signature: {
      waive: Permission.TenantSignatureWaive,
    },
    document: {
      download: Permission.TenantDocumentDownload,
    },
    report: {
      export: Permission.TenantReportExport,
    },
  },
} as const;
