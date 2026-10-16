"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Handshake } from "lucide-react";
import { useEffect, useMemo } from "react";

import { EntityTable } from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { INVOICE_PERIOD_LABEL_KEYS } from "@/features/agreements/constants";
import {
  useAgreement,
  useAgreementsList,
} from "@/features/agreements/hooks/use-agreements";
import type {
  AgreementPricing,
  BillingPeriodPlan,
} from "@/features/agreements/types";
import { useCustomer } from "@/features/customers/hooks/use-customers";
import { moneyText } from "@/features/pricing/lib/format";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const NONE = "__none__";

/**
 * Reservation wizard: a corporate customer's active agreements valid
 * today. Choosing one prices the sale with the agreement's tariff and
 * payment conditions (open account within the credit limit).
 */
export function WizardAgreementPicker({
  customerUuid,
  value,
  onChange,
  useOpenAccount,
  onUseOpenAccountChange,
}: {
  customerUuid: string;
  value: string;
  onChange: (uuid: string) => void;
  useOpenAccount: boolean;
  onUseOpenAccountChange: (value: boolean) => void;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const allowed = can(permissions.tenant.agreements.read);
  const customer = useCustomer(customerUuid || null);
  const corporate = customer.data?.customer_type === "corporate";
  const list = useAgreementsList(
    {
      limit: 50,
      offset: 0,
      customer: customerUuid,
      status: "active",
      validity: "current",
      sort: "code",
    },
    allowed && corporate && customerUuid !== "",
  );
  const items = useMemo(() => list.data?.items ?? [], [list.data]);
  const selected = useAgreement(value || null);

  // One agreement: offer it by default (the person can still sell without).
  useEffect(() => {
    if (!value && items.length === 1 && items[0]) onChange(items[0].uuid);
  }, [items, onChange, value]);

  if (!allowed || !corporate || items.length === 0) return null;
  const a = selected.data;
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <div className="flex items-center gap-2 text-sm font-semibold">
        <Handshake className="size-4" aria-hidden />
        {t("agreements.wizard.title")}
      </div>
      <div className="grid max-w-md gap-1.5">
        <Label htmlFor="wizard-agreement">
          {t("agreements.wizard.agreement")}
        </Label>
        <Select
          value={value || NONE}
          onValueChange={(v) => onChange(v === NONE ? "" : v)}
        >
          <SelectTrigger id="wizard-agreement">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE}>{t("agreements.wizard.none")}</SelectItem>
            {items.map((item) => (
              <SelectItem key={item.uuid} value={item.uuid}>
                {item.code} · {item.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {a ? (
        <div className="space-y-2 text-sm">
          <div className="flex flex-wrap gap-1.5">
            <Badge variant="outline">
              {t(INVOICE_PERIOD_LABEL_KEYS[a.invoice_period])}
            </Badge>
            <Badge variant="outline">
              {t("agreements.wizard.term", { days: a.payment_term_days })}
            </Badge>
            <Badge variant="outline">
              {a.coupon_eligible
                ? t("agreements.detail.coupons_corporate")
                : t("agreements.detail.coupons_none")}
            </Badge>
          </div>
          {a.open_account && a.exposure ? (
            <div className="flex flex-wrap items-center gap-3">
              <div className="flex items-center gap-2">
                <Switch
                  id="wizard-open-account"
                  checked={useOpenAccount}
                  onCheckedChange={onUseOpenAccountChange}
                />
                <Label htmlFor="wizard-open-account">
                  {t("agreements.wizard.use_open_account")}
                </Label>
              </div>
              <span className="text-muted-foreground text-xs">
                {t("agreements.wizard.available", {
                  amount: moneyText(a.exposure.available, locale),
                  limit: moneyText(a.exposure.credit_limit, locale),
                })}
              </span>
            </div>
          ) : (
            <p className="text-muted-foreground text-xs">
              {t("agreements.detail.prepayment_rules")}
            </p>
          )}
        </div>
      ) : null}
    </div>
  );
}

/** Quote summary: the agreement the sale is priced under and its period plan. */
export function AgreementQuoteInfo({
  agreement,
  plan,
}: {
  agreement: AgreementPricing | null | undefined;
  plan: BillingPeriodPlan[] | undefined;
}) {
  const { t, locale } = useLocale();
  const columns = useMemo(
    () =>
      [
        createColumn<BillingPeriodPlan>({
          accessorKey: "no",
          labelKey: "agreements.billing.columns.no",
          enableSorting: false,
          cell: ({ row }) => row.original.no,
        }),
        createColumn<BillingPeriodPlan>({
          id: "range",
          accessorFn: (row) => row.starts_on,
          labelKey: "agreements.billing.columns.range",
          enableSorting: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="flex flex-wrap items-center gap-1 text-xs tabular-nums">
              {date(row.original.starts_on, undefined, locale)} –{" "}
              {date(row.original.ends_on, undefined, locale)}
              {row.original.partial ? (
                <Badge variant="warning" className="px-1 py-0 text-[10px]">
                  {t("agreements.billing.partial")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<BillingPeriodPlan>({
          accessorKey: "days",
          labelKey: "agreements.billing.columns.days",
          enableSorting: false,
          cell: ({ row }) => row.original.days,
        }),
        createColumn<BillingPeriodPlan>({
          accessorKey: "amount",
          labelKey: "agreements.billing.columns.amount",
          enableSorting: false,
          cell: ({ row }) => (
            <span className="tabular-nums">
              {moneyText(row.original.amount, locale)}
            </span>
          ),
        }),
      ] as ColumnDef<BillingPeriodPlan, unknown>[],
    [locale, t],
  );
  if (!agreement) return null;
  return (
    <div className="space-y-3">
      <Alert>
        <Handshake className="size-4" />
        <AlertTitle>
          {agreement.code} · {agreement.name}
        </AlertTitle>
        <AlertDescription>
          {agreement.tariff_applied
            ? t("agreements.wizard.tariff_applied")
            : t("agreements.wizard.tariff_public")}
        </AlertDescription>
      </Alert>
      {plan && plan.length > 0 ? (
        <div className="space-y-2">
          <h4 className="text-sm font-semibold">
            {t("agreements.wizard.billing_plan")}
          </h4>
          <p className="text-muted-foreground text-xs">
            {t("agreements.wizard.billing_plan_hint")}
          </p>
          <EntityTable
            columns={columns}
            data={plan}
            getRowId={(row) => String(row.no)}
            emptyTitle={t("agreements.billing.empty")}
            manual={{ filtering: false, sorting: false, pagination: false }}
            features={{
              persistKey: "tenant-quote-billing-plan-v1",
              rowSelection: false,
              columnFilters: false,
              facetedFilters: false,
              sorting: false,
              globalFilter: false,
            }}
          />
        </div>
      ) : null}
    </div>
  );
}
