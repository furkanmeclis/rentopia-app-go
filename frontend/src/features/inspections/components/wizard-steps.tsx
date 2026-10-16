"use client";

import { AlertTriangle, CheckCircle2 } from "lucide-react";
import { useState } from "react";

import { EntityDetail, EntitySectionCard } from "@/components/entity";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { Textarea } from "@/components/ui/textarea";
import { RentalDepositCard } from "@/features/deposits/components/rental-deposit-card";
import { DEPOSIT_STATUS_LABEL_KEYS } from "@/features/deposits/constants";
import type {
  Inspection,
  InspectionDraftRequest,
} from "@/features/inspections/types";
import { currency, datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

export type StepProps = {
  ins: Inspection;
  readOnly: boolean;
  /** Saves draft fields (offline: kept on the device and synced later). */
  commit: (patch: InspectionDraftRequest) => Promise<boolean>;
};

function money(amount: string, code: string, locale: "tr" | "en") {
  const n = Number(amount);
  return Number.isFinite(n) ? currency(n, code || "TRY", locale) : amount;
}

function CheckRow({
  id,
  label,
  checked,
  disabled,
  onChange,
  hint,
}: {
  id: string;
  label: string;
  checked: boolean;
  disabled?: boolean;
  onChange: (value: boolean) => void;
  hint?: string;
}) {
  return (
    <div className="flex items-start gap-3 rounded-md border p-3">
      <Checkbox
        id={id}
        checked={checked}
        disabled={disabled}
        onCheckedChange={(v) => onChange(v === true)}
        className="mt-0.5 size-5"
      />
      <div className="grid gap-0.5">
        <Label htmlFor={id} className="text-sm leading-snug font-medium">
          {label}
        </Label>
        {hint ? (
          <span className="text-muted-foreground text-xs">{hint}</span>
        ) : null}
      </div>
    </div>
  );
}

/** Step 1: the vehicle and the customer / driver are who the rental says. */
export function VerifyStep({ ins, readOnly, commit }: StepProps) {
  const { t } = useLocale();
  const v = ins.verification;
  const set = (patch: Partial<typeof v>) =>
    void commit({ verification: { ...v, ...patch } });
  const handover = ins.kind === "handover";
  return (
    <div className="grid gap-4">
      <EntitySectionCard title={t("inspections.verify.vehicle")}>
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <span className="rounded-md border-2 border-current px-2 py-0.5 font-mono text-lg font-semibold">
            {ins.rental.vehicle.plate}
          </span>
          <span className="text-muted-foreground text-sm">
            {ins.rental.vehicle.label}
          </span>
        </div>
        <div className="grid gap-2">
          <CheckRow
            id="plate_confirmed"
            label={t("inspections.verify.plate_confirmed")}
            checked={v.plate_confirmed}
            disabled={readOnly}
            onChange={(value) => set({ plate_confirmed: value })}
          />
          <CheckRow
            id="vehicle_ok"
            label={t("inspections.verify.vehicle_ok")}
            checked={v.vehicle_ok}
            disabled={readOnly}
            onChange={(value) => set({ vehicle_ok: value })}
          />
        </div>
      </EntitySectionCard>
      <EntitySectionCard title={t("inspections.verify.customer")}>
        <p className="mb-3 font-medium">{ins.rental.customer.display_name}</p>
        <div className="grid gap-3">
          <div className="grid gap-1.5">
            <Label>{t("inspections.verify.driver")}</Label>
            <Select
              value={v.driver_uuid ?? ""}
              disabled={readOnly}
              onValueChange={(value) => set({ driver_uuid: value || null })}
            >
              <SelectTrigger className="w-full">
                <SelectValue
                  placeholder={t("inspections.verify.driver_pick")}
                />
              </SelectTrigger>
              <SelectContent>
                {ins.rental.drivers
                  .filter((d) => d.uuid)
                  .map((d) => (
                    <SelectItem key={d.uuid} value={d.uuid as string}>
                      {d.name}
                      {d.licence_hint ? ` · ${d.licence_hint}` : ""}
                    </SelectItem>
                  ))}
              </SelectContent>
            </Select>
          </div>
          {handover ? (
            <>
              <CheckRow
                id="identity_checked"
                label={t("inspections.verify.identity_checked")}
                checked={v.identity_checked}
                disabled={readOnly}
                onChange={(value) => set({ identity_checked: value })}
              />
              <CheckRow
                id="licence_checked"
                label={t("inspections.verify.licence_checked")}
                checked={v.licence_checked}
                disabled={readOnly}
                onChange={(value) => set({ licence_checked: value })}
              />
            </>
          ) : null}
        </div>
      </EntitySectionCard>
    </div>
  );
}

/**
 * Step 2 (handover): money from payments (display only) and the security
 * deposit card ("Depozito al", TEC-719).
 */
export function AmountsStep({ ins }: StepProps) {
  const { t, locale } = useLocale();
  const a = ins.rental.amounts;
  const balance = Number(a.balance);
  const depositStatus =
    a.deposit_status === "none"
      ? t("deposits.status.none")
      : t(DEPOSIT_STATUS_LABEL_KEYS[a.deposit_status]);
  return (
    <div className="space-y-4">
      <EntitySectionCard title={t("inspections.amounts.title")}>
        <EntityDetail
          sections={[
            {
              id: "amounts",
              fields: [
                {
                  key: "total",
                  label: t("inspections.amounts.total"),
                  value: money(a.total, a.currency, locale),
                },
                {
                  key: "paid",
                  label: t("inspections.amounts.paid"),
                  value: money(a.paid, a.currency, locale),
                },
                {
                  key: "balance",
                  label: t("inspections.amounts.balance"),
                  value: (
                    <span
                      className={
                        balance > 0 ? "text-destructive font-semibold" : ""
                      }
                    >
                      {money(a.balance, a.currency, locale)}
                    </span>
                  ),
                },
                {
                  key: "payment_status",
                  label: t("inspections.amounts.payment_status"),
                  value: a.payment_status || "—",
                },
                {
                  key: "deposit",
                  label: t("inspections.amounts.deposit"),
                  value: a.deposit_amount
                    ? money(a.deposit_amount, a.currency, locale)
                    : "—",
                },
                {
                  key: "deposit_status",
                  label: t("inspections.amounts.deposit_status"),
                  value: depositStatus,
                },
              ],
            },
          ]}
        />
        {balance > 0 ? (
          <Alert className="mt-3">
            <AlertTriangle className="size-4" />
            <AlertDescription>
              {t("inspections.amounts.balance_hint")}
            </AlertDescription>
          </Alert>
        ) : null}
      </EntitySectionCard>
      <RentalDepositCard rentalUuid={ins.rental.uuid} />
    </div>
  );
}

/** Step 3: km, fuel and equipment (a return shows the handover values). */
export function ReadingsStep({ ins, readOnly, commit }: StepProps) {
  const { t } = useLocale();
  const [km, setKm] = useState(ins.km != null ? String(ins.km) : "");
  const [fuel, setFuel] = useState<number>(ins.fuel ?? 100);
  // Server values replace the local copy when they change.
  const [seen, setSeen] = useState({ km: ins.km, fuel: ins.fuel });
  if (seen.km !== ins.km || seen.fuel !== ins.fuel) {
    setSeen({ km: ins.km, fuel: ins.fuel });
    setKm(ins.km != null ? String(ins.km) : "");
    if (ins.fuel != null) setFuel(ins.fuel);
  }
  const base = ins.baseline;
  const kmNumber = Number(km);
  const kmInvalid = km !== "" && (!Number.isInteger(kmNumber) || kmNumber < 0);
  const belowHandover =
    base?.km != null && km !== "" && kmNumber < (base.km as number);
  return (
    <div className="grid gap-4">
      <EntitySectionCard title={t("inspections.readings.title")}>
        <div className="grid gap-4">
          <div className="grid gap-1.5">
            <Label htmlFor="km">{t("inspections.readings.km")}</Label>
            <Input
              id="km"
              inputMode="numeric"
              pattern="[0-9]*"
              value={km}
              disabled={readOnly}
              aria-invalid={kmInvalid || belowHandover}
              onChange={(e) => setKm(e.target.value.replace(/\D/g, ""))}
              onBlur={() => {
                if (km !== "" && !kmInvalid && kmNumber !== ins.km) {
                  void commit({ km: kmNumber });
                }
              }}
            />
            {base?.km != null ? (
              <span className="text-muted-foreground text-xs">
                {t("inspections.readings.at_handover_km", { km: base.km })}
              </span>
            ) : null}
            {belowHandover ? (
              <span className="text-destructive text-xs">
                {t("inspections.readings.km_below")}
              </span>
            ) : null}
          </div>
          <div className="grid gap-2">
            <div className="flex items-center justify-between">
              <Label>{t("inspections.readings.fuel")}</Label>
              <Badge variant="outline" className="tabular-nums">
                %{fuel}
              </Badge>
            </div>
            <Slider
              value={[fuel]}
              min={0}
              max={100}
              step={5}
              disabled={readOnly}
              aria-label={t("inspections.readings.fuel")}
              onValueChange={(v) => setFuel(v[0] ?? 0)}
              onValueCommit={(v) => void commit({ fuel: v[0] ?? 0 })}
            />
            {base?.fuel != null ? (
              <span className="text-muted-foreground text-xs">
                {t("inspections.readings.at_handover_fuel", {
                  fuel: base.fuel,
                })}
              </span>
            ) : null}
          </div>
        </div>
      </EntitySectionCard>
      <EntitySectionCard
        title={t("inspections.readings.equipment")}
        badge={ins.template.checklist.length}
      >
        <div className="grid gap-2 sm:grid-cols-2">
          {ins.template.checklist.map((item) => {
            const value = ins.equipment[item.key];
            const handed = base?.equipment?.[item.key];
            const missing = base && handed && value === false;
            return (
              <CheckRow
                key={item.key}
                id={`eq-${item.key}`}
                label={item.label}
                checked={value === true}
                disabled={readOnly}
                hint={
                  missing
                    ? t("inspections.readings.missing_now")
                    : value === undefined && item.required
                      ? t("inspections.readings.answer_required")
                      : undefined
                }
                onChange={(checked) =>
                  void commit({ equipment: { [item.key]: checked } })
                }
              />
            );
          })}
        </div>
        {!readOnly ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="mt-3"
            onClick={() => {
              const all: Record<string, boolean> = {};
              for (const item of ins.template.checklist) {
                if (ins.equipment[item.key] === undefined) all[item.key] = true;
              }
              void commit({ equipment: all });
            }}
          >
            <CheckCircle2 className="size-4" />
            {t("inspections.readings.mark_rest_present")}
          </Button>
        ) : null}
      </EntitySectionCard>
    </div>
  );
}

/** Customer review: the summary shown to the customer before signing. */
export function ReviewStep({ ins, readOnly, commit }: StepProps) {
  const { t, locale } = useLocale();
  const [statement, setStatement] = useState(ins.customer_statement);
  const [seen, setSeen] = useState(ins.customer_statement);
  if (seen !== ins.customer_statement) {
    setSeen(ins.customer_statement);
    setStatement(ins.customer_statement);
  }
  const missing = ins.template.checklist.filter(
    (item) => ins.equipment[item.key] === false,
  );
  const photos = ins.photos.filter((p) => p.status === "completed").length;
  return (
    <div className="grid gap-4">
      <EntitySectionCard title={t("inspections.review_step.summary")}>
        <EntityDetail
          sections={[
            {
              id: "summary",
              fields: [
                {
                  key: "vehicle",
                  label: t("inspections.review_step.vehicle"),
                  value: `${ins.rental.vehicle.plate} · ${ins.rental.vehicle.label}`,
                },
                {
                  key: "km",
                  label: t("inspections.readings.km"),
                  value: ins.km != null ? ins.km.toLocaleString(locale) : "—",
                },
                {
                  key: "fuel",
                  label: t("inspections.readings.fuel"),
                  value: ins.fuel != null ? `%${ins.fuel}` : "—",
                },
                {
                  key: "missing",
                  label: t("inspections.review_step.missing_equipment"),
                  value: missing.length
                    ? missing.map((i) => i.label).join(", ")
                    : t("inspections.review_step.none"),
                },
                {
                  key: "damages",
                  label: t("inspections.review_step.damages"),
                  value: String(ins.damages.length),
                },
                {
                  key: "photos",
                  label: t("inspections.review_step.photos"),
                  value: String(photos),
                },
                {
                  key: "time",
                  label: t("inspections.review_step.time"),
                  value: datetime(
                    ins.occurred_at ?? new Date().toISOString(),
                    "d MMM yyyy HH:mm",
                    locale,
                  ),
                },
              ],
            },
          ]}
        />
      </EntitySectionCard>
      <EntitySectionCard title={t("inspections.review_step.statement")}>
        <Textarea
          rows={3}
          value={statement}
          disabled={readOnly}
          placeholder={t("inspections.review_step.statement_hint")}
          onChange={(e) => setStatement(e.target.value)}
          onBlur={() => {
            if (statement !== ins.customer_statement) {
              void commit({ customer_statement: statement });
            }
          }}
        />
      </EntitySectionCard>
      {ins.customer_reviewed_at ? (
        <Alert>
          <CheckCircle2 className="size-4 text-emerald-600" />
          <AlertDescription>
            {t("inspections.review_step.reviewed_at", {
              at: datetime(ins.customer_reviewed_at, "HH:mm", locale),
            })}
          </AlertDescription>
        </Alert>
      ) : (
        <Button
          type="button"
          size="lg"
          disabled={readOnly}
          onClick={() => void commit({ customer_reviewed: true })}
        >
          <CheckCircle2 className="size-4" />
          {t("inspections.review_step.confirm")}
        </Button>
      )}
    </div>
  );
}
