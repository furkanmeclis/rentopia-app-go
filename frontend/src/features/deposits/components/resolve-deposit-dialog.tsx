"use client";

import { Plus, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { useFieldArray, useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import {
  AppInput,
  AppSelect,
  AppTextarea,
  AppToggleGroup,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  DEDUCTION_TARGET_LABEL_KEYS,
  DEDUCTION_TARGETS,
  DEPOSIT_METHOD_LABEL_KEYS,
  RESOLVE_HINT_KEYS,
} from "@/features/deposits/constants";
import {
  useRentalDeposits,
  useResolveDeposit,
  useReturnDamages,
} from "@/features/deposits/hooks/use-deposits";
import type {
  DeductionTarget,
  SecurityDeposit,
} from "@/features/deposits/types";
import { Amount } from "@/features/finance/components/finance-cells";
import { FinanceFormDialog } from "@/features/finance/components/form-dialog";
import {
  errorText,
  isPositiveAmount,
  normalizeAmount,
} from "@/features/finance/lib/format";
import {
  DAMAGE_TYPE_LABEL_KEYS,
  ZONE_LABEL_KEYS,
} from "@/features/inspections/constants";
import { CHARGE_KIND_LABEL_KEYS } from "@/features/rentals/constants";
import { useRental } from "@/features/rentals/hooks/use-rentals";
import { permissions } from "@/config/permissions";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const NONE = "none";

type Line = {
  amount: string;
  applies_to: string;
  rental_charge_uuid: string;
  inspection_damage_uuid: string;
  reason: string;
};

type Values = {
  mode: string;
  deductions: Line[];
  refund_cash_account_uuid: string;
  capture_cash_account_uuid: string;
  note: string;
};

type Option = { value: string; label: string };

function cents(raw: string): number {
  const v = normalizeAmount(raw ?? "");
  if (!isPositiveAmount(v)) return 0;
  return Math.round(Number(v) * 100);
}

/** Deduction rows (field array) and the live total / remainder. */
function DeductionsEditor({
  deposit,
  charges,
  damages,
}: {
  deposit: SecurityDeposit;
  charges: Option[];
  damages: Option[];
}) {
  const { t } = useLocale();
  const { control } = useFormContext<Values>();
  const { fields, append, remove } = useFieldArray({
    control,
    name: "deductions",
  });
  const lines = useWatch<Values, "deductions">({ name: "deductions" }) ?? [];
  const mode = useWatch<Values, "mode">({ name: "mode" });
  const total = lines.reduce((sum, l) => sum + cents(l?.amount ?? ""), 0);
  const deposited = Math.round(Number(deposit.amount) * 100);
  const remainder = deposited - total;
  return (
    <div className="space-y-4">
      {mode === "deduct" ? (
        <div className="grid gap-3">
          {fields.map((field, index) => (
            <div
              key={field.id}
              className="grid gap-3 rounded-md border p-3 sm:grid-cols-2"
            >
              <AppInput
                name={`deductions.${index}.amount`}
                label={t("deposits.fields.amount")}
                inputMode="decimal"
              />
              <AppSelect
                name={`deductions.${index}.applies_to`}
                label={t("deposits.fields.applies_to")}
                options={DEDUCTION_TARGETS.map((target) => ({
                  value: target,
                  label: t(DEDUCTION_TARGET_LABEL_KEYS[target]),
                }))}
              />
              <AppSelect
                name={`deductions.${index}.rental_charge_uuid`}
                label={t("deposits.fields.charge")}
                description={
                  charges.length <= 1
                    ? t("deposits.resolve.no_approved_charges")
                    : undefined
                }
                options={charges}
              />
              <AppSelect
                name={`deductions.${index}.inspection_damage_uuid`}
                label={t("deposits.fields.damage")}
                options={damages}
              />
              <AppTextarea
                name={`deductions.${index}.reason`}
                label={t("deposits.fields.reason")}
                className="sm:col-span-2"
              />
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-destructive justify-self-start"
                onClick={() => remove(index)}
              >
                <Trash2 className="size-4" />
                {t("deposits.resolve.remove_line")}
              </Button>
            </div>
          ))}
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="justify-self-start"
            onClick={() =>
              append({
                amount: "",
                applies_to: "charge",
                rental_charge_uuid: NONE,
                inspection_damage_uuid: NONE,
                reason: "",
              })
            }
          >
            <Plus className="size-4" />
            {t("deposits.resolve.add_line")}
          </Button>
        </div>
      ) : null}
      <dl className="bg-muted/40 grid grid-cols-3 gap-3 rounded-md p-3 text-sm">
        <div className="space-y-1">
          <dt className="text-muted-foreground text-xs">
            {t("deposits.fields.amount")}
          </dt>
          <dd>
            <Amount value={deposit.amount} currency={deposit.currency} />
          </dd>
        </div>
        <div className="space-y-1">
          <dt className="text-muted-foreground text-xs">
            {t("deposits.resolve.deducted")}
          </dt>
          <dd>
            <Amount
              value={(total / 100).toFixed(2)}
              currency={deposit.currency}
            />
          </dd>
        </div>
        <div className="space-y-1">
          <dt className="text-muted-foreground text-xs">
            {t(
              deposit.method === "cash" || deposit.method === "bank_transfer"
                ? "deposits.resolve.paid_back"
                : "deposits.resolve.released",
            )}
          </dt>
          <dd>
            <Amount
              value={(Math.max(remainder, 0) / 100).toFixed(2)}
              currency={deposit.currency}
              className={remainder < 0 ? "text-destructive" : "font-semibold"}
            />
          </dd>
        </div>
      </dl>
    </div>
  );
}

/**
 * Resolution of a held deposit: release it in full, or deduct items
 * (each with a reason, linked to an approved charge or a return damage)
 * and release / pay back the remainder. One submission per deposit (an
 * idempotency key per dialog opening). Online pre-auths are captured or
 * released at the provider; manual methods are recorded.
 */
export function ResolveDepositDialog({
  deposit,
  open,
  onOpenChange,
  initialMode = "release",
}: {
  deposit: SecurityDeposit;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialMode?: "release" | "deduct";
}) {
  const { t } = useLocale();
  const [problem, setProblem] = useState<string | null>(null);
  const resolve = useResolveDeposit(deposit.uuid);
  const { can } = usePermission();
  const canDeduct = deposit.capabilities.deduct;
  const readRental = open && can(permissions.tenant.rentals.read);
  const rental = useRental(readRental ? deposit.rental.uuid : "");
  const card = useRentalDeposits(open ? deposit.rental.uuid : null, open);
  const damages = useReturnDamages(
    deposit.rental.uuid,
    readRental && canDeduct,
  );
  // A fresh key per opening: a retried submission replays, never posts twice.
  const key = useMemo(
    () => (open ? crypto.randomUUID() : ""),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [open, deposit.uuid],
  );
  const cashHeld =
    deposit.method === "cash" || deposit.method === "bank_transfer";
  const chargeOptions: Option[] = [
    { value: NONE, label: t("deposits.fields.no_charge") },
    ...(rental.data?.charges ?? [])
      .filter(
        (c) =>
          c.status === "approved" &&
          !c.reversed &&
          !c.reversal_of_uuid &&
          Number(c.amount) > 0,
      )
      .map((c) => ({
        value: c.uuid,
        label: `${t(CHARGE_KIND_LABEL_KEYS[c.kind])} · ${c.amount} ${c.currency}`,
      })),
  ];
  const damageOptions: Option[] = [
    { value: NONE, label: t("deposits.fields.no_damage") },
    ...(damages.data ?? []).map((d) => ({
      value: d.uuid,
      label: `${t(ZONE_LABEL_KEYS[d.zone as keyof typeof ZONE_LABEL_KEYS] ?? d.zone)} · ${t(DAMAGE_TYPE_LABEL_KEYS[d.damage_type])}`,
    })),
  ];
  const cashOptions: Option[] = (card.data?.cash_accounts ?? []).map((a) => ({
    value: a.uuid,
    label: `${a.code} · ${a.name}`,
  }));
  const bankOptions: Option[] = [
    { value: NONE, label: t("deposits.fields.no_capture_account") },
    ...(card.data?.cash_accounts ?? [])
      .filter((a) => a.kind === "bank")
      .map((a) => ({ value: a.uuid, label: `${a.code} · ${a.name}` })),
  ];
  const line = z.object({
    amount: z
      .string()
      .trim()
      .refine(isPositiveAmount, t("finance.validation.amount")),
    applies_to: z.string(),
    rental_charge_uuid: z.string(),
    inspection_damage_uuid: z.string(),
    reason: z
      .string()
      .trim()
      .min(3, t("finance.validation.reason"))
      .max(500, t("finance.validation.reason")),
  });
  const schema = z
    .object({
      mode: z.string(),
      deductions: z.array(line),
      refund_cash_account_uuid: z.string(),
      capture_cash_account_uuid: z.string(),
      note: z.string().max(500),
    })
    .superRefine((v, ctx) => {
      if (v.mode !== "deduct") return;
      if (v.deductions.length === 0) {
        ctx.addIssue({
          code: "custom",
          path: ["deductions"],
          message: t("deposits.validation.no_lines"),
        });
      }
      v.deductions.forEach((l, i) => {
        if (l.applies_to === "charge" && l.rental_charge_uuid === NONE) {
          ctx.addIssue({
            code: "custom",
            path: ["deductions", i, "rental_charge_uuid"],
            message: t("deposits.validation.charge"),
          });
        }
      });
      const total = v.deductions.reduce((s, l) => s + cents(l.amount), 0);
      if (total > Math.round(Number(deposit.amount) * 100)) {
        ctx.addIssue({
          code: "custom",
          path: ["deductions"],
          message: t("deposits.validation.exceeds"),
        });
      }
    });
  const close = (next: boolean) => {
    if (!next) setProblem(null);
    onOpenChange(next);
  };
  return (
    <FinanceFormDialog<Values>
      open={open}
      onOpenChange={close}
      formId="deposit-resolve-form"
      title={t("deposits.resolve.title")}
      description={t("deposits.resolve.description", {
        reference: deposit.rental.code ?? "",
        method: t(DEPOSIT_METHOD_LABEL_KEYS[deposit.method]),
      })}
      submitLabel={t("deposits.actions.resolve")}
      pending={resolve.isPending}
      problem={problem}
      schema={schema}
      defaultValues={{
        mode: canDeduct ? initialMode : "release",
        deductions:
          canDeduct && initialMode === "deduct"
            ? [
                {
                  amount: "",
                  applies_to: "charge",
                  rental_charge_uuid: NONE,
                  inspection_damage_uuid: NONE,
                  reason: "",
                },
              ]
            : [],
        refund_cash_account_uuid: deposit.cash_account?.uuid ?? "",
        capture_cash_account_uuid: NONE,
        note: "",
      }}
      onSubmit={async (v) => {
        try {
          const deduct = v.mode === "deduct";
          await resolve.mutateAsync({
            idempotency_key: key,
            note: v.note.trim() || undefined,
            refund_cash_account_uuid:
              cashHeld && v.refund_cash_account_uuid
                ? v.refund_cash_account_uuid
                : undefined,
            capture_cash_account_uuid:
              deposit.method === "pos_preauth" &&
              deduct &&
              v.capture_cash_account_uuid !== NONE
                ? v.capture_cash_account_uuid
                : undefined,
            deductions: deduct
              ? v.deductions.map((l) => ({
                  amount: normalizeAmount(l.amount),
                  applies_to: l.applies_to as DeductionTarget,
                  reason: l.reason.trim(),
                  rental_charge_uuid:
                    l.rental_charge_uuid !== NONE
                      ? l.rental_charge_uuid
                      : undefined,
                  inspection_damage_uuid:
                    l.inspection_damage_uuid !== NONE
                      ? l.inspection_damage_uuid
                      : undefined,
                }))
              : [],
          });
          close(false);
        } catch (error) {
          setProblem(errorText(error, t("deposits.errors.resolve_failed")));
        }
      }}
    >
      {canDeduct ? (
        <AppToggleGroup
          name="mode"
          label={t("deposits.resolve.mode")}
          options={[
            { value: "release", label: t("deposits.resolve.release_full") },
            { value: "deduct", label: t("deposits.resolve.deduct") },
          ]}
        />
      ) : (
        <Alert>
          <AlertDescription>{t("deposits.resolve.no_deduct")}</AlertDescription>
        </Alert>
      )}
      <Alert>
        <AlertDescription>
          {t(RESOLVE_HINT_KEYS[deposit.method])}
        </AlertDescription>
      </Alert>
      <DeductionsEditor
        deposit={deposit}
        charges={chargeOptions}
        damages={damageOptions}
      />
      {cashHeld ? (
        <AppSelect
          name="refund_cash_account_uuid"
          label={t("deposits.fields.refund_account")}
          options={cashOptions}
        />
      ) : null}
      {deposit.method === "pos_preauth" ? (
        <AppSelect
          name="capture_cash_account_uuid"
          label={t("deposits.fields.capture_account")}
          description={t("deposits.resolve.capture_account_hint")}
          options={bankOptions}
        />
      ) : null}
      <AppTextarea name="note" label={t("deposits.fields.resolution_note")} />
    </FinanceFormDialog>
  );
}
