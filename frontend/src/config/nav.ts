import {
  Activity,
  ArrowLeftRight,
  Baby,
  BadgePercent,
  Banknote,
  Bell,
  BellRing,
  Boxes,
  Building2,
  Calculator,
  Camera,
  CalendarCheck,
  CalendarClock,
  CalendarDays,
  CalendarPlus,
  CalendarRange,
  CarFront,
  ClipboardCheck,
  ClipboardList,
  ScanSearch,
  Smartphone,
  ChartColumn,
  TrendingUp,
  Coins,
  HandCoins,
  Handshake,
  Cpu,
  Download,
  Landmark,
  Receipt,
  Wallet,
  FilePen,
  FileText,
  Gauge,
  HardDrive,
  KeyRound,
  Layers,
  ListChecks,
  LayoutDashboard,
  LogIn,
  NotebookPen,
  Palette,
  MessageCircle,
  Plug,
  ReceiptText,
  Route,
  ScrollText,
  Settings2,
  Share2,
  ScanLine,
  Shield,
  ShieldCheck,
  SlidersHorizontal,
  Sparkles,
  Store,
  Tags,
  TicketPercent,
  Truck,
  Upload,
  Users,
} from "lucide-react";

import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { defineNav } from "@/features/nav-engine";
import type { NavCatalog } from "@/features/nav-engine";
import { usersNavItem } from "@/features/users/nav";
import { customerRequestsNavItem } from "@/features/customer-portal/nav";
import { TollReviewNavAdornment } from "@/features/tolls/nav";
import { InvoicesNavAdornment } from "@/features/invoices/nav";
import { KabisNavAdornment } from "@/features/kabis/nav";
import { PayablesNavAdornment } from "@/features/suppliers/nav";
import { MyFieldTasksNavAdornment } from "@/features/field-tasks/nav";
import {
  ApprovalsNavAdornment,
  ResolutionTasksNavAdornment,
} from "@/features/overview/nav";

const integrationReadPermissions = [
  permissions.integrations.google.read,
  permissions.integrations.github.read,
  permissions.integrations.facebook.read,
  permissions.integrations.apple.read,
];

/**
 * System Settings sub-sidebar (`/platform/settings/*`). Rendered by
 * `features/settings` (not by `NavEngine`), but uses the same catalog shape
 * and `permission` / `anyPermission` gating.
 */
export const settingsNav = defineNav({
  id: "settings",
  groups: [
    {
      id: "system",
      labelKey: "settings.subnav.group_system",
      icon: Activity,
      defaultOpen: true,
      items: [
        {
          id: "system-overview",
          titleKey: "settings.subnav.system_overview",
          href: routes.platform.settings.root,
          icon: Activity,
          permission: permissions.settings.read,
        },
        {
          id: "system-environment",
          titleKey: "settings.subnav.system_environment",
          href: routes.platform.settings.environment,
          icon: Cpu,
          permission: permissions.settings.read,
        },
        {
          id: "logs",
          titleKey: "layout.nav_logs",
          href: routes.platform.settings.logs,
          icon: ScrollText,
          permission: permissions.logs.read,
        },
        {
          id: "activity",
          titleKey: "layout.nav_activity",
          href: routes.platform.settings.activity,
          icon: Activity,
          permission: permissions.activity.read,
        },
      ],
    },
    {
      id: "general",
      labelKey: "settings.subnav.group_general",
      icon: Settings2,
      defaultOpen: true,
      items: [
        {
          id: "general-branding",
          titleKey: "settings.subnav.general_branding",
          href: routes.platform.settings.general,
          icon: Palette,
          permission: permissions.settings.read,
        },
      ],
    },
    {
      id: "security",
      labelKey: "settings.subnav.group_security",
      icon: Shield,
      defaultOpen: true,
      items: [
        {
          id: "roles",
          titleKey: "layout.nav_roles",
          href: routes.platform.settings.roles,
          icon: Shield,
          permission: permissions.roles.read,
        },
        {
          id: "auth-settings",
          titleKey: "settings.subnav.auth_methods",
          href: routes.platform.settings.auth,
          icon: LogIn,
          permission: permissions.authSettings.read,
        },
        {
          id: "access-policy",
          titleKey: "settings.subnav.access_policy",
          href: routes.platform.settings.access,
          icon: KeyRound,
          permission: permissions.access.read,
        },
      ],
    },
    {
      id: "integrations",
      labelKey: "settings.subnav.group_integrations",
      icon: Share2,
      defaultOpen: true,
      items: [
        {
          id: "oauth-integrations",
          titleKey: "settings.subnav.oauth_providers",
          href: routes.platform.settings.integrations,
          icon: Share2,
          anyPermission: integrationReadPermissions,
        },
        {
          id: "storage",
          titleKey: "settings.subnav.storage",
          href: routes.platform.settings.storage,
          icon: HardDrive,
          permission: permissions.storage.read,
        },
      ],
    },
    {
      id: "io",
      labelKey: "settings.subnav.group_maintenance",
      icon: Upload,
      defaultOpen: true,
      items: [
        {
          id: "imports",
          titleKey: "layout.nav_imports",
          href: routes.platform.settings.imports,
          icon: Upload,
          permission: permissions.imports.read,
        },
        {
          id: "exports",
          titleKey: "layout.nav_exports",
          href: routes.platform.settings.exports,
          icon: Download,
          permission: permissions.exports.read,
        },
        {
          id: "purge-rules",
          titleKey: "settings.subnav.purge_rules",
          href: routes.platform.settings.maintenance,
          icon: Sparkles,
          permission: permissions.logs.read,
        },
      ],
    },
  ],
});

/** Every slug that unlocks at least one settings subpage (OR-gate for the shell). */
function catalogPermissions(catalog: NavCatalog): string[] {
  const slugs = new Set<string>();
  for (const group of catalog.groups) {
    for (const item of group.items) {
      if (typeof item.permission === "string") slugs.add(item.permission);
      else if (item.permission) item.permission.forEach((p) => slugs.add(p));
      item.anyPermission?.forEach((p) => slugs.add(p));
    }
  }
  return [...slugs].sort();
}

export const settingsNavPermissions = catalogPermissions(settingsNav);

export const platformNav = defineNav({
  id: "platform",
  groups: [
    {
      id: "general",
      labelKey: "layout.nav_platform",
      icon: LayoutDashboard,
      defaultOpen: true,
      items: [
        {
          id: "home",
          titleKey: "layout.home",
          href: routes.platform.home,
          icon: LayoutDashboard,
        },
      ],
    },
    {
      id: "manage",
      labelKey: "layout.section_manage",
      icon: Shield,
      defaultOpen: true,
      items: [
        {
          id: "organizations",
          titleKey: "layout.nav_organizations",
          href: routes.platform.organizations.root,
          icon: Building2,
          permission: permissions.organizations.read,
        },
        usersNavItem,
        {
          id: "catalog",
          titleKey: "layout.nav_catalog",
          href: routes.platform.catalog.root,
          icon: CarFront,
          permission: permissions.catalog.read,
        },
        {
          id: "notifications",
          titleKey: "layout.nav_notifications",
          href: routes.platform.notifications.root,
          icon: Bell,
          permission: permissions.notifications.platformRead,
        },
        {
          id: "settings",
          titleKey: "layout.nav_settings",
          href: routes.platform.settings.root,
          icon: Settings2,
          anyPermission: settingsNavPermissions,
        },
      ],
    },
  ],
});

/**
 * Organization shell (`/t/{slug}`) sidebar. Built per slug because every href
 * carries the organization. Items gate on `tenant.*` permissions, which the
 * tenant shell resolves for the current organization only.
 */
export function tenantNav(slug: string): NavCatalog {
  return defineNav({
    id: "tenant",
    groups: [
      {
        id: "general",
        labelKey: "layout.section_organization",
        icon: Building2,
        defaultOpen: true,
        items: [
          {
            id: "home",
            titleKey: "layout.home",
            href: routes.tenant.home(slug),
            icon: LayoutDashboard,
          },
        ],
      },
      {
        id: "operations",
        labelKey: "layout.section_operations",
        icon: Store,
        defaultOpen: true,
        items: [
          {
            id: "branches",
            titleKey: "layout.nav_branches",
            href: routes.tenant.branches.root(slug),
            icon: Store,
            permission: permissions.tenant.branches.read,
          },
          {
            id: "availability",
            titleKey: "layout.nav_availability",
            href: routes.tenant.availability(slug),
            icon: CalendarCheck,
            permission: permissions.tenant.availability.read,
          },
        ],
      },
      {
        id: "reservations",
        labelKey: "layout.section_reservations",
        icon: NotebookPen,
        defaultOpen: true,
        items: [
          {
            id: "reservations",
            titleKey: "layout.nav_reservations",
            href: routes.tenant.reservations.root(slug),
            icon: NotebookPen,
            permission: permissions.tenant.reservations.read,
          },
          {
            id: "reservations-create",
            titleKey: "layout.nav_reservations_create",
            href: routes.tenant.reservations.create(slug),
            icon: CalendarPlus,
            permission: permissions.tenant.reservations.write,
          },
          {
            id: "reservations-quotes",
            titleKey: "layout.nav_reservations_quotes",
            href: routes.tenant.reservations.quotes(slug),
            icon: ReceiptText,
            permission: permissions.tenant.quotes.read,
          },
          {
            id: "reservations-approvals",
            titleKey: "layout.nav_reservations_approvals",
            href: routes.tenant.reservations.approvals(slug),
            icon: ListChecks,
            permission: permissions.tenant.reservations.approve,
            Adornment: ApprovalsNavAdornment,
          },
          {
            id: "reservations-tasks",
            titleKey: "layout.nav_reservations_tasks",
            href: routes.tenant.reservations.tasks(slug),
            icon: ClipboardList,
            permission: permissions.tenant.resolutionTasks.read,
            Adornment: ResolutionTasksNavAdornment,
          },
          customerRequestsNavItem(slug),
        ],
      },
      {
        id: "rentals",
        labelKey: "layout.section_rentals",
        icon: KeyRound,
        defaultOpen: true,
        items: [
          {
            id: "rentals",
            titleKey: "layout.nav_rentals",
            href: routes.tenant.rentals.root(slug),
            icon: KeyRound,
            permission: permissions.tenant.rentals.read,
          },
          {
            id: "operation-tasks",
            titleKey: "layout.nav_operation_tasks",
            href: routes.tenant.operationTasks(slug),
            icon: ClipboardCheck,
            permission: permissions.tenant.rentals.read,
          },
          {
            id: "my-field-tasks",
            titleKey: "layout.nav_my_field_tasks",
            href: routes.tenant.field.root(slug),
            icon: Smartphone,
            permission: permissions.tenant.fieldTasks.execute,
            Adornment: MyFieldTasksNavAdornment,
          },
          {
            id: "field-task-board",
            titleKey: "layout.nav_field_task_board",
            href: routes.tenant.field.board(slug),
            icon: ClipboardList,
            permission: permissions.tenant.fieldTasks.read,
          },
          {
            id: "inspections",
            titleKey: "layout.nav_inspections",
            href: routes.tenant.inspections.root(slug),
            icon: ScanSearch,
            permission: permissions.tenant.rentals.read,
          },
          {
            id: "tolls",
            titleKey: "layout.nav_tolls",
            href: routes.tenant.tolls.root(slug),
            icon: Route,
            permission: permissions.tenant.tolls.read,
          },
          {
            id: "tolls-review",
            titleKey: "layout.nav_tolls_review",
            href: routes.tenant.tolls.review(slug),
            icon: ScanLine,
            permission: permissions.tenant.tolls.read,
            Adornment: TollReviewNavAdornment,
          },
          {
            id: "kabis",
            titleKey: "layout.nav_kabis",
            href: routes.tenant.kabis.root(slug),
            icon: ShieldCheck,
            permission: permissions.tenant.kabis.read,
            Adornment: KabisNavAdornment,
          },
        ],
      },
      {
        id: "finance",
        labelKey: "layout.section_finance",
        icon: Wallet,
        defaultOpen: true,
        items: [
          {
            id: "finance-accounts",
            titleKey: "layout.nav_finance_accounts",
            href: routes.tenant.finance.accounts(slug),
            icon: Coins,
            permission: permissions.tenant.finance.accounts.read,
          },
          {
            id: "finance-collections",
            titleKey: "layout.nav_finance_collections",
            href: routes.tenant.finance.collections(slug),
            icon: HandCoins,
            permission: permissions.tenant.finance.accounts.read,
          },
          {
            id: "finance-invoices",
            titleKey: "layout.nav_finance_invoices",
            href: routes.tenant.invoices.root(slug),
            icon: FileText,
            permission: permissions.tenant.invoices.read,
            Adornment: InvoicesNavAdornment,
          },
          {
            id: "finance-cash-accounts",
            titleKey: "layout.nav_finance_cash_accounts",
            href: routes.tenant.finance.cashAccounts(slug),
            icon: Landmark,
            permission: permissions.tenant.finance.cash.read,
          },
          {
            id: "finance-movements",
            titleKey: "layout.nav_finance_movements",
            href: routes.tenant.finance.movements(slug),
            icon: ArrowLeftRight,
            permission: permissions.tenant.finance.cash.read,
          },
          {
            id: "finance-deposits",
            titleKey: "layout.nav_finance_deposits",
            href: routes.tenant.finance.deposits(slug),
            icon: ShieldCheck,
            permission: permissions.tenant.finance.deposits.read,
          },
          {
            id: "finance-expenses",
            titleKey: "layout.nav_finance_expenses",
            href: routes.tenant.finance.expenses(slug),
            icon: Receipt,
            permission: permissions.tenant.finance.expenses.read,
          },
          {
            id: "finance-categories",
            titleKey: "layout.nav_finance_categories",
            href: routes.tenant.finance.categories(slug),
            icon: Tags,
            permission: permissions.tenant.finance.expenses.read,
          },
          {
            id: "finance-suppliers",
            titleKey: "layout.nav_finance_suppliers",
            href: routes.tenant.finance.suppliers(slug),
            icon: Truck,
            permission: permissions.tenant.suppliers.read,
          },
          {
            id: "finance-supplier-invoices",
            titleKey: "layout.nav_finance_supplier_invoices",
            href: routes.tenant.finance.supplierInvoices(slug),
            icon: FileText,
            permission: permissions.tenant.finance.expenses.read,
          },
          {
            id: "finance-supplier-payments",
            titleKey: "layout.nav_finance_supplier_payments",
            href: routes.tenant.finance.supplierPayments(slug),
            icon: Banknote,
            permission: permissions.tenant.finance.cash.read,
          },
          {
            id: "finance-payables",
            titleKey: "layout.nav_finance_payables",
            href: routes.tenant.finance.payables(slug),
            icon: CalendarClock,
            permission: permissions.tenant.finance.expenses.read,
            Adornment: PayablesNavAdornment,
          },
          {
            id: "finance-reports",
            titleKey: "layout.nav_finance_reports",
            href: routes.tenant.finance.reports(slug),
            icon: ChartColumn,
            permission: permissions.tenant.finance.reports.read,
          },
          {
            id: "finance-profitability",
            titleKey: "layout.nav_finance_profitability",
            href: routes.tenant.finance.profitability(slug),
            icon: TrendingUp,
            permission: permissions.tenant.finance.reports.read,
          },
        ],
      },
      {
        // Reports (TEC-738, §15.1).
        id: "reports",
        labelKey: "layout.section_reports",
        icon: ChartColumn,
        defaultOpen: false,
        items: [
          {
            id: "reports-hub",
            titleKey: "layout.nav_reports_hub",
            href: routes.tenant.reports.root(slug),
            icon: LayoutDashboard,
            permission: permissions.tenant.finance.reports.read,
          },
          {
            id: "reports-occupancy",
            titleKey: "layout.nav_reports_occupancy",
            href: routes.tenant.reports.occupancy(slug),
            icon: Gauge,
            permission: permissions.tenant.finance.reports.read,
          },
          {
            id: "reports-income-expense",
            titleKey: "layout.nav_reports_income_expense",
            href: routes.tenant.reports.incomeExpense(slug),
            icon: ArrowLeftRight,
            permission: permissions.tenant.finance.reports.read,
          },
          {
            id: "reports-channels",
            titleKey: "layout.nav_reports_channels",
            href: routes.tenant.reports.channels(slug),
            icon: Share2,
            permission: permissions.tenant.finance.reports.read,
          },
          {
            id: "reports-aging",
            titleKey: "layout.nav_reports_aging",
            href: routes.tenant.reports.aging(slug),
            icon: CalendarClock,
            permission: permissions.tenant.finance.reports.read,
          },
          {
            id: "reports-branches",
            titleKey: "layout.nav_reports_branches",
            href: routes.tenant.reports.branches(slug),
            icon: Building2,
            permission: permissions.tenant.finance.reports.read,
          },
        ],
      },
      {
        id: "customers",
        labelKey: "layout.section_customers",
        icon: Users,
        defaultOpen: true,
        items: [
          {
            id: "customers",
            titleKey: "layout.nav_customers",
            href: routes.tenant.customers.root(slug),
            icon: Users,
            permission: permissions.tenant.customers.read,
          },
          {
            id: "agreements",
            titleKey: "layout.nav_agreements",
            href: routes.tenant.agreements.root(slug),
            icon: Handshake,
            permission: permissions.tenant.agreements.read,
          },
        ],
      },
      {
        id: "fleet",
        labelKey: "layout.section_fleet",
        icon: CarFront,
        defaultOpen: true,
        items: [
          {
            id: "fleet-vehicles",
            titleKey: "layout.nav_fleet_vehicles",
            href: routes.tenant.fleet.vehicles.root(slug),
            icon: CarFront,
            permission: permissions.tenant.vehicles.read,
          },
          {
            id: "fleet-calendar",
            titleKey: "layout.nav_fleet_calendar",
            href: routes.tenant.fleet.calendar(slug),
            icon: CalendarRange,
            permission: permissions.tenant.vehicles.read,
          },
          {
            id: "fleet-alerts",
            titleKey: "layout.nav_fleet_alerts",
            href: routes.tenant.fleet.alerts(slug),
            icon: CalendarClock,
            permission: permissions.tenant.vehicles.read,
          },
          {
            id: "fleet-transfers",
            titleKey: "layout.nav_fleet_transfers",
            href: routes.tenant.fleet.transfers.root(slug),
            icon: ArrowLeftRight,
            permission: permissions.tenant.transfers.read,
          },
          {
            id: "fleet-one-way-rules",
            titleKey: "layout.nav_fleet_one_way_rules",
            href: routes.tenant.fleet.oneWayRules.root(slug),
            icon: Route,
            permission: permissions.tenant.oneWay.read,
          },
          {
            id: "fleet-capacity-rules",
            titleKey: "layout.nav_fleet_capacity_rules",
            href: routes.tenant.fleet.capacityRules(slug),
            icon: Gauge,
            permission: permissions.tenant.capacityRules.read,
          },
          {
            id: "fleet-segments",
            titleKey: "layout.nav_fleet_segments",
            href: routes.tenant.fleet.segments.root(slug),
            icon: Layers,
            permission: permissions.tenant.fleet.read,
          },
          {
            id: "fleet-groups",
            titleKey: "layout.nav_fleet_groups",
            href: routes.tenant.fleet.groups.root(slug),
            icon: Boxes,
            permission: permissions.tenant.fleet.read,
          },
          {
            id: "fleet-products",
            titleKey: "layout.nav_fleet_products",
            href: routes.tenant.fleet.products.root(slug),
            icon: Tags,
            permission: permissions.tenant.fleet.read,
          },
        ],
      },
      {
        id: "pricing",
        labelKey: "layout.section_pricing",
        icon: BadgePercent,
        defaultOpen: true,
        items: [
          {
            id: "pricing-plans",
            titleKey: "layout.nav_pricing_plans",
            href: routes.tenant.pricing.plans.root(slug),
            icon: Banknote,
            permission: permissions.tenant.pricing.read,
          },
          {
            id: "pricing-rules",
            titleKey: "layout.nav_pricing_rules",
            href: routes.tenant.pricing.rules.root(slug),
            icon: CalendarDays,
            permission: permissions.tenant.pricing.read,
          },
          {
            id: "pricing-extras",
            titleKey: "layout.nav_pricing_extras",
            href: routes.tenant.pricing.extras.root(slug),
            icon: Baby,
            permission: permissions.tenant.extras.read,
          },
          {
            id: "pricing-campaigns",
            titleKey: "layout.nav_pricing_campaigns",
            href: routes.tenant.pricing.campaigns.root(slug),
            icon: TicketPercent,
            permission: permissions.tenant.campaigns.read,
          },
          {
            id: "pricing-preview",
            titleKey: "layout.nav_pricing_preview",
            href: routes.tenant.pricing.preview(slug),
            icon: Calculator,
            permission: permissions.tenant.pricing.read,
          },
        ],
      },
      {
        id: "settings",
        labelKey: "layout.section_tenant_settings",
        icon: Settings2,
        defaultOpen: false,
        items: [
          {
            id: "settings-general",
            titleKey: "layout.nav_tenant_settings_general",
            href: routes.tenant.settings.root(slug),
            icon: SlidersHorizontal,
            permission: permissions.tenant.settings.read,
          },
          {
            id: "settings-members",
            titleKey: "layout.nav_tenant_settings_members",
            href: routes.tenant.settings.members(slug),
            icon: Users,
            permission: permissions.tenant.members.read,
          },
          {
            id: "settings-roles",
            titleKey: "layout.nav_tenant_settings_roles",
            href: routes.tenant.settings.roles.root(slug),
            icon: Shield,
            permission: permissions.tenant.roles.read,
          },
          {
            id: "settings-integrations",
            titleKey: "layout.nav_tenant_settings_integrations",
            href: routes.tenant.settings.integrations(slug),
            icon: Plug,
            permission: permissions.tenant.integrations.manage,
          },
          {
            id: "settings-invoices",
            titleKey: "layout.nav_tenant_settings_invoices",
            href: routes.tenant.invoices.settings(slug),
            icon: ReceiptText,
            permission: permissions.tenant.invoices.settings,
          },
          {
            id: "settings-notifications",
            titleKey: "layout.nav_tenant_settings_notifications",
            href: routes.tenant.settings.notifications(slug),
            icon: BellRing,
            permission: permissions.tenant.notificationSettings.read,
          },
          {
            id: "settings-whatsapp",
            titleKey: "layout.nav_tenant_settings_whatsapp",
            href: routes.tenant.settings.whatsapp(slug),
            icon: MessageCircle,
            permission: permissions.tenant.integrations.manage,
          },
          {
            id: "settings-inspection-templates",
            titleKey: "layout.nav_tenant_settings_inspection_templates",
            href: routes.tenant.settings.inspectionTemplates(slug),
            icon: Camera,
            permission: permissions.tenant.rentals.read,
          },
          {
            id: "settings-contract-templates",
            titleKey: "layout.nav_tenant_settings_contract_templates",
            href: routes.tenant.settings.contractTemplates.root(slug),
            icon: FilePen,
            permission: permissions.tenant.contracts.read,
          },
        ],
      },
    ],
  });
}
