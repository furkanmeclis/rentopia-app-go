"use client";

import { Plus, Trash2 } from "lucide-react";
import { useCallback, useState } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import {
  AGREEMENT_STATUS_LABEL_KEYS,
  AGREEMENT_TRANSITIONS,
  CONTACT_ROLES,
  CONTACT_ROLE_LABEL_KEYS,
} from "@/features/agreements/constants";
import {
  useAddAgreementContact,
  useAddAgreementDriver,
  useSetAgreementStatus,
} from "@/features/agreements/hooks/use-agreements";
import type {
  Agreement,
  AgreementStatus,
  ContactRole,
} from "@/features/agreements/types";
import {
  useCustomerContacts,
  useCustomerDrivers,
} from "@/features/customers/hooks/use-customers";
import { customersService } from "@/features/customers/services/customers.service";
import {
  useActivateRatePlan,
  useCreateRatePlan,
} from "@/features/pricing/hooks/use-pricing";
import { useProductOptions } from "@/features/pricing/hooks/use-pricing-options";
import { isAmount, normalizeAmount } from "@/features/pricing/lib/format";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

function Footer({
  onCancel,
  onConfirm,
  disabled,
  label,
}: {
  onCancel: () => void;
  onConfirm: () => void;
  disabled: boolean;
  label: string;
}) {
  const { t } = useLocale();
  return (
    <>
      <Button type="button" variant="outline" onClick={onCancel}>
        {t("form.cancel")}
      </Button>
      <Button type="button" disabled={disabled} onClick={onConfirm}>
        {label}
      </Button>
    </>
  );
}

/** Status change: activate, suspend, resume or terminate (with a reason). */
export function AgreementStatusDialog({
  agreement,
  open,
  onOpenChange,
}: {
  agreement: Agreement;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const setStatus = useSetAgreementStatus();
  const targets = AGREEMENT_TRANSITIONS[agreement.status];
  const [status, setTarget] = useState<AgreementStatus | "">(targets[0] ?? "");
  const [reason, setReason] = useState("");
  const needsReason = status === "terminated";
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("agreements.status_dialog.title")}
      description={t("agreements.status_dialog.description")}
      footer={
        <Footer
          onCancel={() => onOpenChange(false)}
          label={t("agreements.status_dialog.confirm")}
          disabled={
            !status ||
            setStatus.isPending ||
            (needsReason && reason.trim().length < 3)
          }
          onConfirm={() =>
            status &&
            setStatus.mutate(
              {
                uuid: agreement.uuid,
                body: { status: status as "active", reason: reason.trim() },
              },
              { onSuccess: () => onOpenChange(false) },
            )
          }
        />
      }
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="agreement-status">
            {t("agreements.status_dialog.status")}
          </Label>
          <Select
            value={status}
            onValueChange={(v) => setTarget(v as AgreementStatus)}
          >
            <SelectTrigger id="agreement-status">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {targets.map((s) => (
                <SelectItem key={s} value={s}>
                  {t(AGREEMENT_STATUS_LABEL_KEYS[s])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {needsReason ? (
          <div className="grid gap-1.5">
            <Label htmlFor="agreement-status-reason">
              {t("agreements.status_dialog.reason")}
            </Label>
            <Textarea
              id="agreement-status-reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
            />
            <p className="text-muted-foreground text-xs">
              {t("agreements.status_dialog.terminate_hint")}
            </p>
          </div>
        ) : null}
      </div>
    </AppDialog>
  );
}

/** Authorise one of the corporate customer's contacts. */
export function AgreementContactDialog({
  agreement,
  open,
  onOpenChange,
}: {
  agreement: Agreement;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const add = useAddAgreementContact();
  const contacts = useCustomerContacts(agreement.customer.uuid, open);
  const taken = new Set(agreement.contacts.map((c) => c.contact_uuid));
  const options = (contacts.data?.items ?? []).filter(
    (c) => !taken.has(c.uuid),
  );
  const [contact, setContact] = useState("");
  const [role, setRole] = useState<ContactRole>("operations");
  const [note, setNote] = useState("");
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("agreements.contacts.add_title")}
      description={t("agreements.contacts.add_description")}
      footer={
        <Footer
          onCancel={() => onOpenChange(false)}
          label={t("agreements.contacts.add")}
          disabled={!contact || add.isPending}
          onConfirm={() =>
            add.mutate(
              {
                uuid: agreement.uuid,
                body: { contact_uuid: contact, role, note: note.trim() },
              },
              {
                onSuccess: () => {
                  setContact("");
                  setNote("");
                  onOpenChange(false);
                },
              },
            )
          }
        />
      }
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="agreement-contact">
            {t("agreements.contacts.contact")}
          </Label>
          <Select value={contact} onValueChange={setContact}>
            <SelectTrigger id="agreement-contact">
              <SelectValue
                placeholder={
                  options.length
                    ? t("agreements.contacts.pick")
                    : t("agreements.contacts.none_left")
                }
              />
            </SelectTrigger>
            <SelectContent>
              {options.map((c) => (
                <SelectItem key={c.uuid} value={c.uuid}>
                  {c.full_name}
                  {c.title ? ` · ${c.title}` : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-muted-foreground text-xs">
            {t("agreements.contacts.pick_hint")}
          </p>
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="agreement-contact-role">
            {t("agreements.contacts.role")}
          </Label>
          <Select value={role} onValueChange={(v) => setRole(v as ContactRole)}>
            <SelectTrigger id="agreement-contact-role">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CONTACT_ROLES.map((r) => (
                <SelectItem key={r} value={r}>
                  {t(CONTACT_ROLE_LABEL_KEYS[r])}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="agreement-contact-note">
            {t("agreements.fields.note")}
          </Label>
          <Input
            id="agreement-contact-note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
      </div>
    </AppDialog>
  );
}

/**
 * Permit a driver: a driver record of the corporate customer, or an
 * individual customer (every driver record of that person).
 */
export function AgreementDriverDialog({
  agreement,
  open,
  onOpenChange,
}: {
  agreement: Agreement;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const add = useAddAgreementDriver();
  const [kind, setKind] = useState<"driver" | "customer">("driver");
  const [driver, setDriver] = useState("");
  const [customer, setCustomer] = useState("");
  const [note, setNote] = useState("");
  const drivers = useCustomerDrivers(agreement.customer.uuid);
  const taken = new Set(agreement.drivers.map((d) => d.driver_uuid));
  const driverOptions = (drivers.data?.items ?? []).filter(
    (d) => !taken.has(d.uuid),
  );
  const loadCustomers = useCallback(
    async (query: string): Promise<ComboboxOption[]> => {
      try {
        const page = await customersService.list(slug, {
          limit: 20,
          offset: 0,
          q: query || undefined,
          customer_type: "individual",
          anonymized: "false",
        });
        return page.items.map((c) => ({
          value: c.uuid,
          label: c.display_name,
          description: c.identity_masked || undefined,
        }));
      } catch {
        return [];
      }
    },
    [slug],
  );
  const ready = kind === "driver" ? Boolean(driver) : Boolean(customer);
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("agreements.drivers.add_title")}
      description={t("agreements.drivers.add_description")}
      footer={
        <Footer
          onCancel={() => onOpenChange(false)}
          label={t("agreements.drivers.add")}
          disabled={!ready || add.isPending}
          onConfirm={() =>
            add.mutate(
              {
                uuid: agreement.uuid,
                body:
                  kind === "driver"
                    ? { driver_uuid: driver, note: note.trim() }
                    : { customer_uuid: customer, note: note.trim() },
              },
              {
                onSuccess: () => {
                  setDriver("");
                  setCustomer("");
                  setNote("");
                  onOpenChange(false);
                },
              },
            )
          }
        />
      }
    >
      <div className="grid gap-4">
        <ToggleGroup
          type="single"
          variant="outline"
          value={kind}
          onValueChange={(v) => v && setKind(v as "driver" | "customer")}
        >
          <ToggleGroupItem value="driver">
            {t("agreements.drivers.kind_driver")}
          </ToggleGroupItem>
          <ToggleGroupItem value="customer">
            {t("agreements.drivers.kind_customer")}
          </ToggleGroupItem>
        </ToggleGroup>
        {kind === "driver" ? (
          <div className="grid gap-1.5">
            <Label htmlFor="agreement-driver">
              {t("agreements.drivers.driver")}
            </Label>
            <Select value={driver} onValueChange={setDriver}>
              <SelectTrigger id="agreement-driver">
                <SelectValue
                  placeholder={
                    driverOptions.length
                      ? t("agreements.drivers.pick")
                      : t("agreements.drivers.none_left")
                  }
                />
              </SelectTrigger>
              <SelectContent>
                {driverOptions.map((d) => (
                  <SelectItem key={d.uuid} value={d.uuid}>
                    {d.full_name}
                    {d.licence_masked ? ` · ${d.licence_masked}` : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        ) : (
          <div className="grid gap-1.5">
            <Label htmlFor="agreement-driver-customer">
              {t("agreements.drivers.customer")}
            </Label>
            <AsyncCombobox
              id="agreement-driver-customer"
              value={customer}
              onValueChange={setCustomer}
              loadOptions={loadCustomers}
              placeholder={t("agreements.drivers.customer_placeholder")}
              searchPlaceholder={t("agreements.fields.customer_search")}
              emptyText={t("agreements.fields.customer_empty")}
            />
          </div>
        )}
        <div className="grid gap-1.5">
          <Label htmlFor="agreement-driver-note">
            {t("agreements.fields.note")}
          </Label>
          <Input
            id="agreement-driver-note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </div>
      </div>
    </AppDialog>
  );
}

type BandRow = { min: string; max: string; price: string };

/**
 * New agreement tariff: an agreement-scoped rate plan of one rental
 * product with its duration bands (versioned like every rate plan; later
 * edits happen on the rate plan page).
 */
export function AgreementTariffDialog({
  agreement,
  open,
  onOpenChange,
}: {
  agreement: Agreement;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const products = useProductOptions();
  const create = useCreateRatePlan();
  const activate = useActivateRatePlan();
  const [product, setProduct] = useState("");
  const [bands, setBands] = useState<BandRow[]>([
    { min: "1", max: "", price: "" },
  ]);
  const [activateNow, setActivateNow] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const valid =
    product !== "" &&
    bands.every(
      (b) =>
        /^\d+$/.test(b.min) &&
        (b.max === "" || /^\d+$/.test(b.max)) &&
        isAmount(b.price),
    );
  const update = (i: number, patch: Partial<BandRow>) =>
    setBands((rows) => rows.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  const submit = async () => {
    setError(null);
    try {
      const plan = await create.mutateAsync({
        rental_product_uuid: product,
        scope: "agreement",
        agreement_uuid: agreement.uuid,
        name: `${agreement.code} · ${agreement.name}`,
        priority: 0,
        bands: bands.map((b) => ({
          min_days: Number(b.min),
          max_days: b.max === "" ? null : Number(b.max),
          daily_price: normalizeAmount(b.price),
        })),
      });
      if (activateNow) {
        await activate.mutateAsync({ uuid: plan.uuid, body: {} });
      }
      setProduct("");
      setBands([{ min: "1", max: "", price: "" }]);
      onOpenChange(false);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };
  return (
    <AppDialog
      open={open}
      onOpenChange={onOpenChange}
      title={t("agreements.tariffs.add_title")}
      description={t("agreements.tariffs.add_description")}
      className="sm:max-w-xl"
      footer={
        <Footer
          onCancel={() => onOpenChange(false)}
          label={t("agreements.tariffs.add")}
          disabled={!valid || create.isPending || activate.isPending}
          onConfirm={() => void submit()}
        />
      }
    >
      <div className="grid gap-4">
        <div className="grid gap-1.5">
          <Label htmlFor="agreement-tariff-product">
            {t("agreements.tariffs.product")}
          </Label>
          <Select value={product} onValueChange={setProduct}>
            <SelectTrigger id="agreement-tariff-product">
              <SelectValue placeholder={t("agreements.tariffs.pick_product")} />
            </SelectTrigger>
            <SelectContent>
              {products.options.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="grid gap-2">
          <Label>{t("agreements.tariffs.bands")}</Label>
          {bands.map((b, i) => (
            <div
              key={i}
              className="grid grid-cols-[1fr_1fr_1.4fr_auto] items-center gap-2"
            >
              <Input
                aria-label={t("agreements.tariffs.min_days")}
                placeholder={t("agreements.tariffs.min_days")}
                inputMode="numeric"
                value={b.min}
                onChange={(e) => update(i, { min: e.target.value })}
              />
              <Input
                aria-label={t("agreements.tariffs.max_days")}
                placeholder={t("agreements.tariffs.max_days_open")}
                inputMode="numeric"
                value={b.max}
                onChange={(e) => update(i, { max: e.target.value })}
              />
              <Input
                aria-label={t("agreements.tariffs.daily_price")}
                placeholder={t("agreements.tariffs.daily_price")}
                inputMode="decimal"
                value={b.price}
                onChange={(e) => update(i, { price: e.target.value })}
              />
              <Button
                type="button"
                size="icon"
                variant="ghost"
                aria-label={t("agreements.tariffs.remove_band")}
                disabled={bands.length === 1}
                onClick={() =>
                  setBands((rows) => rows.filter((_, j) => j !== i))
                }
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
          <div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() =>
                setBands((rows) => [...rows, { min: "", max: "", price: "" }])
              }
            >
              <Plus className="size-3.5" />
              {t("agreements.tariffs.add_band")}
            </Button>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Switch
            id="agreement-tariff-activate"
            checked={activateNow}
            onCheckedChange={setActivateNow}
          />
          <Label htmlFor="agreement-tariff-activate">
            {t("agreements.tariffs.activate_now")}
          </Label>
        </div>
        {error ? <p className="text-destructive text-sm">{error}</p> : null}
      </div>
    </AppDialog>
  );
}
