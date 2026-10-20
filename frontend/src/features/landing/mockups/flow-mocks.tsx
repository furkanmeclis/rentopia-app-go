"use client";

import {
  BadgeCheck,
  FileText,
  Fuel,
  Gauge,
  MessageSquareText,
  Receipt,
  ShieldCheck,
  Wallet,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatTry } from "../lib/format";

const CARD =
  "bg-card text-card-foreground w-full max-w-md rounded-2xl border p-5 shadow-[0_1px_0_rgba(20,21,39,0.04),0_24px_48px_-24px_rgba(20,21,39,0.25)] sm:p-6";

/** Agreement filled from the template, approved with a signed link. */
export function ContractMock({ className }: { className?: string }) {
  const { t } = useLocale();
  const rows = [
    ["landing.flow.contract_customer", "A. Yılmaz"],
    ["landing.flow.contract_vehicle", "34 RTP 482"],
    ["landing.flow.contract_period", t("landing.flow.contract_period_value")],
  ] as const;

  return (
    <div aria-hidden className={cn(CARD, className)}>
      <div className="flex items-center justify-between gap-3">
        <span className="inline-flex items-center gap-2 font-semibold">
          <FileText className="text-muted-foreground size-4" />
          {t("landing.flow.contract_title")}
        </span>
        <Badge variant="success">{t("landing.flow.step2_state")}</Badge>
      </div>

      <dl className="mt-5 grid gap-2.5 text-sm">
        {rows.map(([label, value]) => (
          <div key={label} className="flex justify-between gap-4">
            <dt className="text-muted-foreground">{t(label)}</dt>
            <dd className="font-medium tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>

      <div className="bg-muted/60 mt-5 grid gap-1.5 rounded-xl p-3">
        {[92, 100, 76, 88].map((w, i) => (
          <span
            key={i}
            className="bg-foreground/10 h-1.5 rounded-full"
            style={{ width: `${w}%` }}
          />
        ))}
      </div>

      <div className="mt-5 flex items-center gap-2 text-sm">
        <MessageSquareText className="text-primary size-4" />
        {t("landing.flow.contract_link")}
      </div>
      <div className="mt-3 flex items-center justify-between gap-3">
        <span className="text-muted-foreground text-xs">
          {t("landing.flow.contract_code")}
        </span>
        <div className="flex gap-1.5">
          {["4", "8", "1", "9", "0", "6"].map((d, i) => (
            <span
              key={i}
              className="grid size-8 place-items-center rounded-md border text-sm font-semibold tabular-nums"
            >
              {d}
            </span>
          ))}
        </div>
      </div>
      <div className="mt-5 flex items-center justify-between border-t pt-4 text-sm">
        <span className="inline-flex items-center gap-2 font-medium">
          <BadgeCheck className="size-4 text-emerald-600 dark:text-emerald-300" />
          {t("landing.flow.contract_pdf")}
        </span>
        <span className="text-muted-foreground text-xs">PDF</span>
      </div>
    </div>
  );
}

/** Differences found at return, written to the rental as charges. */
export function ReturnMock({ className }: { className?: string }) {
  const { t, locale } = useLocale();
  const rows = [
    { icon: Gauge, label: "landing.flow.return_km", value: "+612 km" },
    {
      icon: Fuel,
      label: "landing.flow.return_fuel",
      value: t("landing.flow.return_fuel_value"),
    },
    {
      icon: Receipt,
      label: "landing.flow.return_tolls",
      value: t("landing.flow.return_tolls_value"),
    },
    {
      icon: ShieldCheck,
      label: "landing.flow.return_damage",
      value: t("landing.flow.return_damage_value"),
    },
  ];
  const charges = [
    { label: "landing.flow.return_fuel", amount: 1150 },
    { label: "landing.flow.return_tolls", amount: 186 },
  ];

  return (
    <div aria-hidden className={cn(CARD, className)}>
      <div className="flex items-center justify-between gap-3">
        <span className="font-semibold">{t("landing.flow.return_title")}</span>
        <Badge variant="secondary">{t("landing.flow.step4_state")}</Badge>
      </div>
      <ul className="mt-5 grid grid-cols-2 gap-2">
        {rows.map((row) => (
          <li key={row.label} className="rounded-xl border p-3">
            <row.icon className="text-muted-foreground size-4" />
            <p className="text-muted-foreground mt-2 text-xs">{t(row.label)}</p>
            <p className="text-sm font-semibold tabular-nums">{row.value}</p>
          </li>
        ))}
      </ul>
      <p className="text-muted-foreground mt-5 text-xs">
        {t("landing.flow.return_charges")}
      </p>
      <ul className="mt-2 grid gap-1.5 text-sm">
        {charges.map((c) => (
          <li key={c.label} className="flex justify-between">
            <span>{t(c.label)}</span>
            <span className="tabular-nums">{formatTry(locale, c.amount)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** Deposit released, payment posted, invoice issued. */
export function CloseoutMock({ className }: { className?: string }) {
  const { t, locale } = useLocale();
  const steps = [
    {
      icon: Wallet,
      key: "landing.flow.invoice_deposit",
      meta: formatTry(locale, 5000),
    },
    {
      icon: BadgeCheck,
      key: "landing.flow.invoice_payment",
      meta: formatTry(locale, 8246),
    },
    {
      icon: Receipt,
      key: "landing.flow.invoice_issued",
      meta: "RNT2026000148",
    },
  ];

  return (
    <div aria-hidden className={cn(CARD, className)}>
      <div className="flex items-center justify-between gap-3">
        <span className="font-semibold">{t("landing.flow.invoice_title")}</span>
        <Badge variant="success">{t("landing.flow.step5_state")}</Badge>
      </div>
      <ol className="relative mt-6 grid gap-5">
        <span className="bg-border absolute top-2 bottom-2 left-[0.9375rem] w-px" />
        {steps.map((step) => (
          <li key={step.key} className="relative flex items-start gap-3">
            <span className="bg-primary text-primary-foreground relative grid size-8 shrink-0 place-items-center rounded-full">
              <step.icon className="size-4" />
            </span>
            <div className="min-w-0 pt-1">
              <p className="text-sm font-medium">{t(step.key)}</p>
              <p className="text-muted-foreground text-xs tabular-nums">
                {step.meta}
              </p>
            </div>
          </li>
        ))}
      </ol>
      <div className="mt-6 flex items-center justify-between border-t pt-4 text-xs">
        <span className="text-muted-foreground">
          {t("landing.flow.invoice_number")}
        </span>
        <span className="font-medium tabular-nums">RNT2026000148</span>
      </div>
    </div>
  );
}
