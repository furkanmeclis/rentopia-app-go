package rbac

// Role slugs (seeded).
const (
	RoleSuperAdmin = "super_admin"
	RoleCMSUser    = "cms_user"
)

// Permission slugs (seeded).
const (
	PermPlatformUsersRead                 = "platform.users.read"
	PermPlatformUsersWrite                = "platform.users.write"
	PermPlatformUsersExport               = "platform.users.export"
	PermPlatformUsersImport               = "platform.users.import"
	PermPlatformUsersBulkDisable          = "platform.users.bulk.disable"
	PermPlatformUsersBulkEnable           = "platform.users.bulk.enable"
	PermPlatformUsersImpersonate          = "platform.users.impersonate"
	PermPlatformRolesRead                 = "platform.roles.read"
	PermPlatformRolesWrite                = "platform.roles.write"
	PermPlatformRolesExport               = "platform.roles.export"
	PermPlatformRolesImport               = "platform.roles.import"
	PermPlatformRolesBulkDelete           = "platform.roles.bulk.delete"
	PermPlatformOrganizationsRead         = "platform.organizations.read"
	PermPlatformOrganizationsWrite        = "platform.organizations.write"
	PermPlatformOrganizationsExport       = "platform.organizations.export"
	PermPlatformOrganizationsBulkActivate = "platform.organizations.bulk.activate"
	PermPlatformOrganizationsBulkSuspend  = "platform.organizations.bulk.suspend"
	PermPlatformCatalogRead               = "platform.catalog.read"
	PermPlatformCatalogWrite              = "platform.catalog.write"
	PermPlatformBulkRead                  = "platform.bulk.read"
	PermPlatformNotificationsRead         = "platform.notifications.read"
	PermPlatformNotificationsReadAll      = "platform.notifications.read_all"
	PermPlatformNotificationsExport       = "platform.notifications.export"
	PermPlatformSettingsRead              = "platform.settings.read"
	PermPlatformSettingsWrite             = "platform.settings.write"
	PermPlatformActivityRead              = "platform.activity.read"
	PermPlatformImportsRead               = "platform.imports.read"
	PermPlatformExportsRead               = "platform.exports.read"
	PermPlatformStorageRead               = "platform.storage.read"
	PermPlatformStorageWrite              = "platform.storage.write"
	PermPlatformLogsRead                  = "platform.logs.read"
	PermPlatformLogsWrite                 = "platform.logs.write"
	PermPlatformAccessRead                = "platform.access.read"
	PermPlatformAccessWrite               = "platform.access.write"
	PermPlatformIntegrationsGitHubRead    = "platform.integrations.github.read"
	PermPlatformIntegrationsGitHubWrite   = "platform.integrations.github.write"
	PermPlatformIntegrationsGoogleRead    = "platform.integrations.google.read"
	PermPlatformIntegrationsGoogleWrite   = "platform.integrations.google.write"
	PermPlatformIntegrationsFacebookRead  = "platform.integrations.facebook.read"
	PermPlatformIntegrationsFacebookWrite = "platform.integrations.facebook.write"
	PermPlatformIntegrationsAppleRead     = "platform.integrations.apple.read"
	PermPlatformIntegrationsAppleWrite    = "platform.integrations.apple.write"
	PermPlatformAuthSettingsRead          = "platform.auth.settings.read"
	PermPlatformAuthSettingsWrite         = "platform.auth.settings.write"
	PermAuthSession                       = "auth.session"
	PermNotificationsRead                 = "notifications.read"
	PermNotificationsManage               = "notifications.manage"
	PermTenantSettingsRead                = "tenant.settings.read"
	PermTenantSettingsWrite               = "tenant.settings.write"
	PermTenantMembersRead                 = "tenant.members.read"
	PermTenantMembersWrite                = "tenant.members.write"
	PermTenantRolesRead                   = "tenant.roles.read"
	PermTenantRolesWrite                  = "tenant.roles.write"
	PermTenantBranchesRead                = "tenant.branches.read"
	PermTenantBranchesWrite               = "tenant.branches.write"
	PermTenantBranchesExport              = "tenant.branches.export"
	PermTenantBranchesImport              = "tenant.branches.import"
	PermTenantBranchesBulkActivate        = "tenant.branches.bulk.activate"
	PermTenantBranchesBulkDeactivate      = "tenant.branches.bulk.deactivate"
	PermTenantFleetRead                   = "tenant.fleet.read"
	PermTenantFleetWrite                  = "tenant.fleet.write"
	PermTenantVehiclesRead                = "tenant.vehicles.read"
	PermTenantVehiclesWrite               = "tenant.vehicles.write"
	PermTenantVehiclesExport              = "tenant.vehicles.export"
	PermTenantVehiclesImport              = "tenant.vehicles.import"
	PermTenantVehiclesBulk                = "tenant.vehicles.bulk"
	PermTenantBlocksRead                  = "tenant.blocks.read"
	PermTenantBlocksWrite                 = "tenant.blocks.write"
	PermTenantTransfersRead               = "tenant.transfers.read"
	PermTenantTransfersWrite              = "tenant.transfers.write"
	PermTenantOneWayRead                  = "tenant.one_way.read"
	PermTenantOneWayWrite                 = "tenant.one_way.write"
	// Pricing (TEC-700 / TEC-701): rate plans, date rules, preview. Writes
	// only touch drafts; activation is a separate slug.
	PermTenantPricingRead        = "tenant.pricing.read"
	PermTenantPricingWrite       = "tenant.pricing.write"
	PermTenantPricingActivate    = "tenant.pricing.activate"
	PermTenantAvailabilityRead   = "tenant.availability.read"
	PermTenantAvailabilityHold   = "tenant.availability.hold"
	PermTenantCapacityRulesRead  = "tenant.capacity_rules.read"
	PermTenantCapacityRulesWrite = "tenant.capacity_rules.write"
	// Customers and drivers (TEC-705). Read shows masked identity / contact
	// values; reveal, export, import and anonymize are granted one by one.
	PermTenantCustomersRead      = "tenant.customers.read"
	PermTenantCustomersWrite     = "tenant.customers.write"
	PermTenantCustomersReveal    = "tenant.customers.reveal"
	PermTenantCustomersExport    = "tenant.customers.export"
	PermTenantCustomersImport    = "tenant.customers.import"
	PermTenantCustomersAnonymize = "tenant.customers.anonymize"
	// Reservations, quotes and resolution tasks (TEC-704 / TEC-706 /
	// TEC-698). Cancel, confirm, export and the driver eligibility override
	// are granted one by one.
	PermTenantReservationsRead                = "tenant.reservations.read"
	PermTenantReservationsWrite               = "tenant.reservations.write"
	PermTenantReservationsCancel              = "tenant.reservations.cancel"
	PermTenantReservationsConfirm             = "tenant.reservations.confirm"
	PermTenantReservationsExport              = "tenant.reservations.export"
	PermTenantReservationsOverrideEligibility = "tenant.reservations.override_eligibility"
	PermTenantQuotesRead                      = "tenant.quotes.read"
	PermTenantQuotesWrite                     = "tenant.quotes.write"
	PermTenantResolutionTasksRead             = "tenant.resolution_tasks.read"
	PermTenantResolutionTasksWrite            = "tenant.resolution_tasks.write"
	// Rentals and operation tasks (TEC-709): close = finish the return check
	// and decide charges; contracts (TEC-710): activate = make a template
	// version the active one. PDF downloads use PermTenantDocumentDownload.
	PermTenantRentalsRead       = "tenant.rentals.read"
	PermTenantRentalsWrite      = "tenant.rentals.write"
	PermTenantRentalsClose      = "tenant.rentals.close"
	PermTenantContractsRead     = "tenant.contracts.read"
	PermTenantContractsWrite    = "tenant.contracts.write"
	PermTenantContractsActivate = "tenant.contracts.activate"
	// Extras catalog, prices and branch stock (TEC-702). Equipment holds
	// use PermTenantAvailabilityHold.
	PermTenantExtrasRead  = "tenant.extras.read"
	PermTenantExtrasWrite = "tenant.extras.write"
	// Campaigns and coupons (TEC-703).
	PermTenantCampaignsRead  = "tenant.campaigns.read"
	PermTenantCampaignsWrite = "tenant.campaigns.write"
	// Sensitive actions of later tenant modules (TEC-689), separate from the
	// resource read/write slugs so roles can grant them one by one.
	PermTenantPriceOverride    = "tenant.price.override"
	PermTenantDiscountApply    = "tenant.discount.apply"
	PermTenantPaymentCollect   = "tenant.payment.collect"
	PermTenantPaymentRefund    = "tenant.payment.refund"
	PermTenantDamageApprove    = "tenant.damage.approve"
	PermTenantSignatureWaive   = "tenant.signature.waive"
	PermTenantDocumentDownload = "tenant.document.download"
	PermTenantReportExport     = "tenant.report.export"
	// Integrations, payments and staff approval (TEC-676 / TEC-707 /
	// TEC-708, migration 000080). Collecting at the branch and refunds keep
	// PermTenantPaymentCollect / PermTenantPaymentRefund.
	PermTenantIntegrationsManage  = "tenant.integrations.manage"
	PermTenantPaymentsRead        = "tenant.payments.read"
	PermTenantPaymentsCreateLink  = "tenant.payments.create_link"
	PermTenantReservationsApprove = "tenant.reservations.approve"
	// Toll passes (TEC-725, migration 000401). Vehicle tags use
	// PermTenantVehiclesWrite, the processing fee PermTenantSettingsWrite.
	PermTenantTollsRead   = "tenant.tolls.read"
	PermTenantTollsExport = "tenant.tolls.export"
	PermTenantTollsImport = "tenant.tolls.import"
	PermTenantTollsReview = "tenant.tolls.review"
)

// Tenant template role keys seeded by migrations 000030 and 000041.
const (
	TenantRoleOwner            = "owner"
	TenantRoleAdmin            = "admin"
	TenantRoleMember           = "member"
	TenantRoleMerkezYoneticisi = "merkez_yoneticisi"
	TenantRoleSubeYoneticisi   = "sube_yoneticisi"
	TenantRoleRezervasyonSatis = "rezervasyon_satis"
	TenantRoleMuhasebe         = "muhasebe"
	TenantRoleTeslimIade       = "teslim_iade"
)

// IsSystemRole reports whether slug is a protected system role.
func IsSystemRole(slug string) bool {
	return slug == RoleSuperAdmin
}
