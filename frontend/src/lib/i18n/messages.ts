import type { AppLocale } from "@/config/i18n";

import enAccess from "@/locales/en/access.json";
import enActivity from "@/locales/en/activity.json";
import enAuth from "@/locales/en/auth.json";
import enAvailability from "@/locales/en/availability.json";
import enBranches from "@/locales/en/branches.json";
import enFleet from "@/locales/en/fleet.json";
import enBranding from "@/locales/en/branding.json";
import enBulk from "@/locales/en/bulk.json";
import enCatalog from "@/locales/en/catalog.json";
import enChart from "@/locales/en/chart.json";
import enCommon from "@/locales/en/common.json";
import enDashboard from "@/locales/en/dashboard.json";
import enEditor from "@/locales/en/editor.json";
import enEntity from "@/locales/en/entity.json";
import enExports from "@/locales/en/exports.json";
import enForm from "@/locales/en/form.json";
import enImports from "@/locales/en/imports.json";
import enIntegrations from "@/locales/en/integrations.json";
import enLogs from "@/locales/en/logs.json";
import enLayout from "@/locales/en/layout.json";
import enNotifications from "@/locales/en/notifications.json";
import enInvitations from "@/locales/en/invitations.json";
import enOrganizations from "@/locales/en/organizations.json";
import enPermissions from "@/locales/en/permissions.json";
import enRoles from "@/locales/en/roles.json";
import enSearch from "@/locales/en/search.json";
import enErrors from "@/locales/en/errors.json";
import enRealtime from "@/locales/en/realtime.json";
import enSettings from "@/locales/en/settings.json";
import enStepup from "@/locales/en/stepup.json";
import enStorage from "@/locales/en/storage.json";
import enTable from "@/locales/en/table.json";
import enOverview from "@/locales/en/overview.json";
import enTenant from "@/locales/en/tenant.json";
import enTenantMembers from "@/locales/en/tenant_members.json";
import enTenantRoles from "@/locales/en/tenant_roles.json";
import enTenantSettings from "@/locales/en/tenant_settings.json";
import enPayments from "@/locales/en/payments.json";
import enFinance from "@/locales/en/finance.json";
import enSuppliers from "@/locales/en/suppliers.json";
import enPublicApi from "@/locales/en/public_api.json";
import enSite from "@/locales/en/site.json";
import enNotificationEngine from "@/locales/en/notification_engine.json";
import enSiteLeads from "@/locales/en/site_leads.json";
import enWhatsapp from "@/locales/en/whatsapp.json";
import enUsers from "@/locales/en/users.json";
import enVehicleRecords from "@/locales/en/vehicle_records.json";
import enOneWayRules from "@/locales/en/one_way_rules.json";
import enPricing from "@/locales/en/pricing.json";
import enExtras from "@/locales/en/extras.json";
import enCampaigns from "@/locales/en/campaigns.json";
import enTransfers from "@/locales/en/transfers.json";
import enFleetCalendar from "@/locales/en/fleet_calendar.json";
import enVehicleBlocks from "@/locales/en/vehicle_blocks.json";
import enVehicles from "@/locales/en/vehicles.json";
import enCustomers from "@/locales/en/customers.json";
import enReservations from "@/locales/en/reservations.json";
import enRentals from "@/locales/en/rentals.json";
import enTolls from "@/locales/en/tolls.json";
import enInvoices from "@/locales/en/invoices.json";
import enKabis from "@/locales/en/kabis.json";
import enChannels from "@/locales/en/channels.json";
import enAgreements from "@/locales/en/agreements.json";
import enDeposits from "@/locales/en/deposits.json";
import enProfitability from "@/locales/en/profitability.json";
import enReports from "@/locales/en/reports.json";
import enPlatformHealth from "@/locales/en/platform_health.json";
import enContracts from "@/locales/en/contracts.json";
import enInspections from "@/locales/en/inspections.json";
import enFieldTasks from "@/locales/en/field_tasks.json";
import enCustomerPortal from "@/locales/en/customer_portal.json";
import trAccess from "@/locales/tr/access.json";
import trActivity from "@/locales/tr/activity.json";
import trAuth from "@/locales/tr/auth.json";
import trAvailability from "@/locales/tr/availability.json";
import trBranches from "@/locales/tr/branches.json";
import trFleet from "@/locales/tr/fleet.json";
import trBranding from "@/locales/tr/branding.json";
import trBulk from "@/locales/tr/bulk.json";
import trCatalog from "@/locales/tr/catalog.json";
import trChart from "@/locales/tr/chart.json";
import trCommon from "@/locales/tr/common.json";
import trDashboard from "@/locales/tr/dashboard.json";
import trEditor from "@/locales/tr/editor.json";
import trEntity from "@/locales/tr/entity.json";
import trExports from "@/locales/tr/exports.json";
import trForm from "@/locales/tr/form.json";
import trImports from "@/locales/tr/imports.json";
import trIntegrations from "@/locales/tr/integrations.json";
import trLogs from "@/locales/tr/logs.json";
import trLayout from "@/locales/tr/layout.json";
import trNotifications from "@/locales/tr/notifications.json";
import trInvitations from "@/locales/tr/invitations.json";
import trOrganizations from "@/locales/tr/organizations.json";
import trPermissions from "@/locales/tr/permissions.json";
import trRoles from "@/locales/tr/roles.json";
import trSearch from "@/locales/tr/search.json";
import trErrors from "@/locales/tr/errors.json";
import trRealtime from "@/locales/tr/realtime.json";
import trSettings from "@/locales/tr/settings.json";
import trStepup from "@/locales/tr/stepup.json";
import trStorage from "@/locales/tr/storage.json";
import trTable from "@/locales/tr/table.json";
import trOverview from "@/locales/tr/overview.json";
import trTenant from "@/locales/tr/tenant.json";
import trTenantMembers from "@/locales/tr/tenant_members.json";
import trTenantRoles from "@/locales/tr/tenant_roles.json";
import trTenantSettings from "@/locales/tr/tenant_settings.json";
import trPayments from "@/locales/tr/payments.json";
import trFinance from "@/locales/tr/finance.json";
import trSuppliers from "@/locales/tr/suppliers.json";
import trPublicApi from "@/locales/tr/public_api.json";
import trSite from "@/locales/tr/site.json";
import trNotificationEngine from "@/locales/tr/notification_engine.json";
import trSiteLeads from "@/locales/tr/site_leads.json";
import trWhatsapp from "@/locales/tr/whatsapp.json";
import trUsers from "@/locales/tr/users.json";
import trVehicleRecords from "@/locales/tr/vehicle_records.json";
import trOneWayRules from "@/locales/tr/one_way_rules.json";
import trPricing from "@/locales/tr/pricing.json";
import trExtras from "@/locales/tr/extras.json";
import trCampaigns from "@/locales/tr/campaigns.json";
import trTransfers from "@/locales/tr/transfers.json";
import trFleetCalendar from "@/locales/tr/fleet_calendar.json";
import trVehicleBlocks from "@/locales/tr/vehicle_blocks.json";
import trVehicles from "@/locales/tr/vehicles.json";
import trCustomers from "@/locales/tr/customers.json";
import trReservations from "@/locales/tr/reservations.json";
import trRentals from "@/locales/tr/rentals.json";
import trTolls from "@/locales/tr/tolls.json";
import trInvoices from "@/locales/tr/invoices.json";
import trKabis from "@/locales/tr/kabis.json";
import trChannels from "@/locales/tr/channels.json";
import trAgreements from "@/locales/tr/agreements.json";
import trDeposits from "@/locales/tr/deposits.json";
import trProfitability from "@/locales/tr/profitability.json";
import trReports from "@/locales/tr/reports.json";
import trPlatformHealth from "@/locales/tr/platform_health.json";
import trContracts from "@/locales/tr/contracts.json";
import trInspections from "@/locales/tr/inspections.json";
import trFieldTasks from "@/locales/tr/field_tasks.json";
import trCustomerPortal from "@/locales/tr/customer_portal.json";

export type MessageDictionary = Record<string, string>;

const catalogs: Record<AppLocale, Record<string, MessageDictionary>> = {
  tr: {
    common: trCommon,
    layout: trLayout,
    editor: trEditor,
    table: trTable,
    form: trForm,
    chart: trChart,
    auth: trAuth,
    availability: trAvailability,
    branding: trBranding,
    bulk: trBulk,
    catalog: trCatalog,
    entity: trEntity,
    permissions: trPermissions,
    realtime: trRealtime,
    roles: trRoles,
    search: trSearch,
    errors: trErrors,
    users: trUsers,
    vehicles: trVehicles,
    customers: trCustomers,
    reservations: trReservations,
    rentals: trRentals,
    tolls: trTolls,
    invoices: trInvoices,
    kabis: trKabis,
    channels: trChannels,
    agreements: trAgreements,
    deposits: trDeposits,
    profitability: trProfitability,
    reports: trReports,
    platform_health: trPlatformHealth,
    contracts: trContracts,
    inspections: trInspections,
    field_tasks: trFieldTasks,
    customer_portal: trCustomerPortal,
    vehicle_records: trVehicleRecords,
    vehicle_blocks: trVehicleBlocks,
    transfers: trTransfers,
    fleet_calendar: trFleetCalendar,
    one_way_rules: trOneWayRules,
    pricing: trPricing,
    extras: trExtras,
    campaigns: trCampaigns,
    notifications: trNotifications,
    dashboard: trDashboard,
    activity: trActivity,
    exports: trExports,
    imports: trImports,
    logs: trLogs,
    settings: trSettings,
    storage: trStorage,
    access: trAccess,
    integrations: trIntegrations,
    stepup: trStepup,
    invitations: trInvitations,
    organizations: trOrganizations,
    tenant: trTenant,
    overview: trOverview,
    branches: trBranches,
    fleet: trFleet,
    tenant_members: trTenantMembers,
    tenant_roles: trTenantRoles,
    tenant_settings: trTenantSettings,
    payments: trPayments,
    finance: trFinance,
    suppliers: trSuppliers,
    public_api: trPublicApi,
    site: trSite,
    notification_engine: trNotificationEngine,
    site_leads: trSiteLeads,
    whatsapp: trWhatsapp,
  },
  en: {
    common: enCommon,
    layout: enLayout,
    editor: enEditor,
    table: enTable,
    form: enForm,
    chart: enChart,
    auth: enAuth,
    availability: enAvailability,
    branding: enBranding,
    bulk: enBulk,
    catalog: enCatalog,
    entity: enEntity,
    permissions: enPermissions,
    realtime: enRealtime,
    roles: enRoles,
    search: enSearch,
    errors: enErrors,
    users: enUsers,
    vehicles: enVehicles,
    customers: enCustomers,
    reservations: enReservations,
    rentals: enRentals,
    tolls: enTolls,
    invoices: enInvoices,
    kabis: enKabis,
    channels: enChannels,
    agreements: enAgreements,
    deposits: enDeposits,
    profitability: enProfitability,
    reports: enReports,
    platform_health: enPlatformHealth,
    contracts: enContracts,
    inspections: enInspections,
    field_tasks: enFieldTasks,
    customer_portal: enCustomerPortal,
    vehicle_records: enVehicleRecords,
    vehicle_blocks: enVehicleBlocks,
    transfers: enTransfers,
    fleet_calendar: enFleetCalendar,
    one_way_rules: enOneWayRules,
    pricing: enPricing,
    extras: enExtras,
    campaigns: enCampaigns,
    notifications: enNotifications,
    dashboard: enDashboard,
    activity: enActivity,
    exports: enExports,
    imports: enImports,
    logs: enLogs,
    settings: enSettings,
    storage: enStorage,
    access: enAccess,
    integrations: enIntegrations,
    stepup: enStepup,
    invitations: enInvitations,
    organizations: enOrganizations,
    tenant: enTenant,
    overview: enOverview,
    branches: enBranches,
    fleet: enFleet,
    tenant_members: enTenantMembers,
    tenant_roles: enTenantRoles,
    tenant_settings: enTenantSettings,
    payments: enPayments,
    finance: enFinance,
    suppliers: enSuppliers,
    public_api: enPublicApi,
    site: enSite,
    notification_engine: enNotificationEngine,
    site_leads: enSiteLeads,
    whatsapp: enWhatsapp,
  },
};
export function loadMessages(locale: AppLocale) {
  return catalogs[locale];
}

export function translate(
  locale: AppLocale,
  key: string,
  params?: Record<string, string | number>,
  fallbackLocale: AppLocale = "en",
): string {
  const [ns, ...rest] = key.split(".");
  const path = rest.join(".");
  const primary = catalogs[locale]?.[ns]?.[path];
  const fallback = catalogs[fallbackLocale]?.[ns]?.[path];
  if (primary === undefined)
    recordMissingKey(locale, key, fallback !== undefined);
  let text = primary ?? fallback ?? key;

  if (params) {
    for (const [k, v] of Object.entries(params)) {
      text = text.replace(new RegExp(`{{\\s*${k}\\s*}}`, "g"), String(v));
    }
  }

  return text;
}

/** Simple plural helper: key_one / key_other */
export function translatePlural(
  locale: AppLocale,
  key: string,
  count: number,
  params?: Record<string, string | number>,
) {
  const suffix = count === 1 ? "one" : "other";
  return translate(locale, `${key}_${suffix}`, { count, ...params });
}

/**
 * Development aid: remembers keys the active locale could not resolve so a
 * crawler (or `window.__i18nMissing` in devtools) can list them. No-op in
 * production builds.
 */
function recordMissingKey(
  locale: AppLocale,
  key: string,
  hasFallback: boolean,
) {
  if (process.env.NODE_ENV === "production" || typeof window === "undefined")
    return;
  const w = window as unknown as { __i18nMissing?: Record<string, string> };
  w.__i18nMissing ??= {};
  w.__i18nMissing[`${locale}:${key}`] = hasFallback ? "fallback" : "missing";
}
