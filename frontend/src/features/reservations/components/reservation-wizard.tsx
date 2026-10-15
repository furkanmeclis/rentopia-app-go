"use client";

import {
  AlertTriangle,
  ArrowLeft,
  ArrowRight,
  Calculator,
  CheckCircle2,
  RefreshCw,
  Truck,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import {
  AppForm,
  AppInput,
  AppRadioGroup,
  AppTextarea,
} from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { AvailabilitySearchForm } from "@/features/availability/components/availability-search-form";
import { CAPACITY_REASON_LABEL_KEYS } from "@/features/availability/constants";
import { deliveryProblems } from "@/features/field-tasks/lib/field-tasks";
import { channelLabel } from "@/features/pricing/hooks/use-pricing-options";
import { moneyText } from "@/features/pricing/lib/format";
import {
  ConditionsList,
  PriceLinesTable,
  PriceTotals,
} from "@/features/reservations/components/price-breakdown";
import { RangeText } from "@/features/reservations/components/reservation-cells";
import {
  WizardCustomerStep,
  type WizardDrivers,
} from "@/features/reservations/components/wizard-customer-step";
import {
  WizardExtrasStep,
  type WizardDiscount,
} from "@/features/reservations/components/wizard-extras-step";
import { WizardProductTable } from "@/features/reservations/components/wizard-product-table";
import {
  DRIVER_REASON_KEYS,
  WARNING_KEYS,
  reasonText,
} from "@/features/reservations/constants";
import {
  useCreateQuote,
  useCreateReservation,
  useQuoteSearch,
} from "@/features/reservations/hooks/use-reservations";
import {
  reservationProblem,
  type ReservationProblem,
} from "@/features/reservations/lib/errors";
import type {
  Quote,
  QuoteRequest,
  QuoteSearchItem,
  QuoteSearchRequest,
  ReservationCreateRequest,
} from "@/features/reservations/types";
import { useTenant } from "@/features/tenant/components/tenant-provider";
import { cn } from "@/lib/utils";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type Step = "search" | "product" | "customer" | "extras" | "summary";

const STEPS: Step[] = ["search", "product", "customer", "extras", "summary"];

const STEP_LABEL_KEYS: Record<Step, string> = {
  search: "reservations.wizard.steps.search",
  product: "reservations.wizard.steps.product",
  customer: "reservations.wizard.steps.customer",
  extras: "reservations.wizard.steps.extras",
  summary: "reservations.wizard.steps.summary",
};

const OPTIONS_FORM_ID = "reservation-wizard-options";

type Mode = "standard" | "request_only" | "confirm_now";

type OptionValues = {
  mode: Mode;
  note: string;
  external_reference: string;
};

function StepIndicator({
  step,
  reachable,
  onStep,
}: {
  step: Step;
  reachable: (step: Step) => boolean;
  onStep: (step: Step) => void;
}) {
  const { t } = useLocale();
  const activeIndex = STEPS.indexOf(step);
  return (
    <ol className="flex flex-wrap gap-2 text-sm">
      {STEPS.map((s, index) => (
        <li key={s}>
          <Button
            type="button"
            size="sm"
            variant={index === activeIndex ? "default" : "outline"}
            className={cn(
              "rounded-full",
              index > activeIndex && "border-dashed",
            )}
            disabled={!reachable(s)}
            aria-current={index === activeIndex ? "step" : undefined}
            onClick={() => onStep(s)}
          >
            <span className="tabular-nums">{index + 1}.</span>
            {t(STEP_LABEL_KEYS[s])}
          </Button>
        </li>
      ))}
    </ol>
  );
}

/** The problem of a create attempt with what can be done next. */
function ProblemAlert({
  problem,
  canOverride,
  overrideReason,
  onOverrideReason,
  onUseReplacement,
  onRequote,
  onStep,
  onOverride,
  pending,
}: {
  problem: ReservationProblem;
  canOverride: boolean;
  overrideReason: string;
  onOverrideReason: (value: string) => void;
  onUseReplacement: (quote: Quote) => void;
  onRequote: () => void;
  onStep: (step: Step) => void;
  onOverride: () => void;
  pending: boolean;
}) {
  const { t, locale } = useLocale();
  switch (problem.kind) {
    case "price_changed":
      return (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{t("reservations.problems.price_changed")}</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>
              {problem.replacement
                ? t("reservations.problems.price_changed_detail", {
                    total: moneyText(problem.replacement.total, locale),
                  })
                : problem.message}
            </p>
            {problem.replacement ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => onUseReplacement(problem.replacement!)}
              >
                <CheckCircle2 className="size-3.5" />
                {t("reservations.problems.use_new_price")}
              </Button>
            ) : null}
          </AlertDescription>
        </Alert>
      );
    case "quote_expired":
    case "quote_used":
      return (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>
            {problem.kind === "quote_expired"
              ? t("reservations.problems.quote_expired")
              : t("reservations.problems.quote_used")}
          </AlertTitle>
          <AlertDescription>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={onRequote}
            >
              <RefreshCw className="size-3.5" />
              {t("reservations.problems.requote")}
            </Button>
          </AlertDescription>
        </Alert>
      );
    case "capacity":
      return (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{t("reservations.problems.capacity")}</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>
              {t(
                CAPACITY_REASON_LABEL_KEYS[problem.reason] ??
                  CAPACITY_REASON_LABEL_KEYS.no_vehicle,
              )}
            </p>
            {problem.alternatives &&
            problem.alternatives.products.length > 0 ? (
              <p className="text-sm">
                {t("reservations.problems.alternatives", {
                  products: problem.alternatives.products
                    .map((p) => `${p.product.code} (${p.sellable})`)
                    .join(", "),
                })}
              </p>
            ) : null}
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => onStep("product")}
            >
              <ArrowLeft className="size-3.5" />
              {t("reservations.problems.pick_other_product")}
            </Button>
          </AlertDescription>
        </Alert>
      );
    case "drivers":
      return (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{t("reservations.problems.drivers")}</AlertTitle>
          <AlertDescription className="space-y-3">
            <ul className="list-disc space-y-0.5 ps-4">
              {problem.drivers
                .filter((d) => !d.eligible)
                .map((d) => (
                  <li key={d.driver_uuid ?? d.name}>
                    <span className="font-medium">{d.name}</span>:{" "}
                    {d.reasons
                      .map((r) => reasonText(t, DRIVER_REASON_KEYS, r))
                      .join(", ")}
                  </li>
                ))}
            </ul>
            {canOverride ? (
              <div className="grid max-w-md gap-1.5">
                <Label htmlFor="override-reason">
                  {t("reservations.problems.override_reason")}
                </Label>
                <Input
                  id="override-reason"
                  value={overrideReason}
                  onChange={(e) => onOverrideReason(e.target.value)}
                />
                <div>
                  <Button
                    type="button"
                    size="sm"
                    variant="destructive"
                    disabled={overrideReason.trim().length < 3 || pending}
                    onClick={onOverride}
                  >
                    {t("reservations.problems.override")}
                  </Button>
                </div>
              </div>
            ) : (
              <p>{t("reservations.problems.override_forbidden")}</p>
            )}
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => onStep("customer")}
            >
              <ArrowLeft className="size-3.5" />
              {t("reservations.problems.change_drivers")}
            </Button>
          </AlertDescription>
        </Alert>
      );
    case "equipment":
      return (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{t("reservations.problems.equipment")}</AlertTitle>
          <AlertDescription>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => onStep("extras")}
            >
              <ArrowLeft className="size-3.5" />
              {t("reservations.problems.change_extras")}
            </Button>
          </AlertDescription>
        </Alert>
      );
    default:
      return (
        <Alert variant="destructive">
          <AlertTriangle className="size-4" />
          <AlertTitle>{t("reservations.problems.generic")}</AlertTitle>
          <AlertDescription>
            {"message" in problem ? problem.message : null}
          </AlertDescription>
        </Alert>
      );
  }
}

/** Quote summary: price lines, totals, conditions and validity. */
export function QuoteSummary({ quote }: { quote: Quote }) {
  const { t, locale } = useLocale();
  const discount = quote.breakdown.pricing?.discount;
  return (
    <div className="space-y-4">
      <PriceLinesTable
        lines={quote.lines}
        persistKey="tenant-reservation-quote-lines-v1"
      />
      <PriceTotals
        rows={[
          {
            key: "rental_subtotal",
            label: t("reservations.price.rental_subtotal"),
            value: quote.rental_subtotal,
          },
          ...(Number(quote.discount_total) > 0
            ? [
                {
                  key: "discount",
                  label: discount?.name
                    ? t("reservations.price.discount_named", {
                        name: discount.name,
                      })
                    : t("reservations.price.discount"),
                  value: quote.discount_total,
                  negative: true,
                },
              ]
            : []),
          {
            key: "extras",
            label: t("reservations.price.extras_total"),
            value: Number(quote.extras_total) > 0 ? quote.extras_total : null,
          },
          {
            key: "one_way",
            label: t("reservations.price.one_way_fee"),
            value: Number(quote.one_way_fee) > 0 ? quote.one_way_fee : null,
          },
          {
            key: "delivery",
            label: t("reservations.price.delivery_fee"),
            value: Number(quote.delivery_fee) > 0 ? quote.delivery_fee : null,
          },
          {
            key: "base",
            label: t("reservations.price.base_total"),
            value: quote.base_total,
          },
          {
            key: "vat",
            label: t("reservations.price.vat_total"),
            value: quote.vat_total,
          },
          {
            key: "total",
            label: t("reservations.price.total"),
            value: quote.total,
            strong: true,
          },
          {
            key: "prepayment",
            label: t("reservations.price.prepayment"),
            value: quote.prepayment,
          },
        ]}
      />
      {quote.breakdown.warnings.length > 0 ? (
        <Alert>
          <AlertDescription>
            {quote.breakdown.warnings
              .map((w) => reasonText(t, WARNING_KEYS, w))
              .join(" · ")}
          </AlertDescription>
        </Alert>
      ) : null}
      {quote.breakdown.conditions ? (
        <div className="space-y-2">
          <h4 className="text-sm font-semibold">
            {t("reservations.conditions.title")}
          </h4>
          <ConditionsList conditions={quote.breakdown.conditions} />
        </div>
      ) : null}
      <p className="text-muted-foreground text-xs">
        {t("reservations.wizard.quote_valid_until", {
          time: datetime(quote.valid_until, "HH:mm", locale),
        })}
      </p>
    </div>
  );
}

/**
 * "Yeni rezervasyon": search → product → customer & drivers → extras &
 * discount → summary. The quote is made on the server (price, extras,
 * coupon) and re-validated when the reservation is created; every answer
 * that needs a decision (price changed, capacity gone, driver rules,
 * equipment) is shown inline with the next step.
 */
export function ReservationWizardPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const router = useRouter();
  const { can } = usePermission();
  const [step, setStep] = useState<Step>("search");
  const [search, setSearch] = useState<QuoteSearchRequest | null>(null);
  const [product, setProduct] = useState<QuoteSearchItem | null>(null);
  const [customerUuid, setCustomerUuid] = useState("");
  const [drivers, setDrivers] = useState<WizardDrivers>({
    selected: {},
    primary: "",
  });
  const [quantities, setQuantities] = useState<Record<string, number>>({});
  const [discount, setDiscount] = useState<WizardDiscount>({
    coupon: "",
    manualKind: "percent",
    manualValue: "",
    manualReason: "",
  });
  const [couponError, setCouponError] = useState<string | null>(null);
  const [quote, setQuote] = useState<Quote | null>(null);
  const [quoteError, setQuoteError] = useState<string | null>(null);
  const [problem, setProblem] = useState<ReservationProblem | null>(null);
  const [overrideReason, setOverrideReason] = useState("");
  const [options, setOptions] = useState<OptionValues | null>(null);

  const searchQuery = useQuoteSearch(search);
  const createQuote = useCreateQuote();
  const createReservation = useCreateReservation();

  const selectedDrivers = Object.keys(drivers.selected).filter(
    (id) => drivers.selected[id],
  );

  const reachable = useCallback(
    (s: Step) => {
      switch (s) {
        case "search":
          return true;
        case "product":
          return search !== null;
        case "customer":
          return product !== null;
        case "extras":
          return product !== null && customerUuid !== "";
        case "summary":
          return quote !== null;
      }
    },
    [customerUuid, product, quote, search],
  );

  const goto = (s: Step) => {
    if (reachable(s)) setStep(s);
  };

  const quoteRequest = useMemo<QuoteRequest | null>(() => {
    if (!search || !product) return null;
    const manual =
      discount.coupon.trim() === "" && discount.manualValue.trim() !== ""
        ? {
            kind: discount.manualKind,
            value: discount.manualValue.trim().replace(",", "."),
            reason: discount.manualReason.trim(),
          }
        : null;
    return {
      ...search,
      product_uuid: product.product.uuid,
      customer_uuid: customerUuid || null,
      extras: Object.entries(quantities).map(([extra_uuid, quantity]) => ({
        extra_uuid,
        quantity,
      })),
      coupon_code: discount.coupon.trim() || undefined,
      manual_discount: manual,
    };
  }, [customerUuid, discount, product, quantities, search]);

  /** Prices the selection on the server; coupon problems stay on the extras step. */
  const makeQuote = async () => {
    if (!quoteRequest) return;
    setCouponError(null);
    setQuoteError(null);
    setProblem(null);
    try {
      const created = await createQuote.mutateAsync(quoteRequest);
      setQuote(created);
      setStep("summary");
    } catch (error) {
      const p = reservationProblem(error);
      if (p.kind === "coupon") {
        setCouponError(p.reason);
        setStep("extras");
        return;
      }
      const delivery = deliveryProblems(t, error);
      setQuoteError(
        delivery.length
          ? delivery.join(" · ")
          : "message" in p
            ? p.message
            : t("reservations.problems.generic"),
      );
    }
  };

  const invalidateQuote = () => {
    setQuote(null);
    setProblem(null);
  };

  const submit = async (values: OptionValues, override?: string) => {
    if (!quote) return;
    setOptions(values);
    setProblem(null);
    const body: ReservationCreateRequest = {
      quote_uuid: quote.uuid,
      customer_uuid: customerUuid,
      drivers: selectedDrivers.map((uuid) => ({
        driver_uuid: uuid,
        role: uuid === drivers.primary ? "primary" : "additional",
      })),
      note: values.note.trim() || undefined,
      external_reference: values.external_reference.trim() || undefined,
      request_only: values.mode === "request_only",
      confirm_now: values.mode === "confirm_now",
      eligibility_override: override ? { reason: override } : null,
    };
    try {
      const reservation = await createReservation.mutateAsync(body);
      router.push(routes.tenant.reservations.detail(slug, reservation.uuid));
    } catch (error) {
      setProblem(reservationProblem(error));
    }
  };

  const optionSchema = useMemo(
    () =>
      z.object({
        mode: z.enum(["standard", "request_only", "confirm_now"]),
        note: z.string().max(2000),
        external_reference: z.string().max(64),
      }),
    [],
  );
  const modeOptions = [
    {
      value: "standard",
      label: t("reservations.wizard.mode.standard"),
      description: t("reservations.wizard.mode.standard_hint"),
    },
    {
      value: "request_only",
      label: t("reservations.wizard.mode.request_only"),
      description: t("reservations.wizard.mode.request_only_hint"),
    },
    ...(can(permissions.tenant.reservations.confirm)
      ? [
          {
            value: "confirm_now",
            label: t("reservations.wizard.mode.confirm_now"),
            description: t("reservations.wizard.mode.confirm_now_hint"),
          },
        ]
      : []),
  ];

  const selection = (
    <div className="text-muted-foreground flex flex-wrap items-center gap-2 text-sm">
      {search?.starts_at && search.ends_at ? (
        <RangeText startsAt={search.starts_at} endsAt={search.ends_at} />
      ) : null}
      {search?.channel ? (
        <Badge variant="outline">{channelLabel(t, search.channel)}</Badge>
      ) : null}
      {product ? (
        <Badge variant="secondary">
          {product.product.code} · {product.product.title}
        </Badge>
      ) : null}
      {search?.pickup_point_uuid || search?.return_point_uuid ? (
        <Badge variant="outline" className="gap-1">
          <Truck className="size-3" aria-hidden />
          {t("field_tasks.delivery.badge")}
        </Badge>
      ) : null}
      {selectedDrivers.length > 0 ? (
        <Badge variant="outline">
          {t("reservations.wizard.drivers_count", {
            count: selectedDrivers.length,
          })}
        </Badge>
      ) : null}
      {Object.keys(quantities).length > 0 ? (
        <Badge variant="outline">
          {t("reservations.wizard.extras_count", {
            count: Object.keys(quantities).length,
          })}
        </Badge>
      ) : null}
    </div>
  );

  return (
    <EntityPage
      title={t("reservations.wizard.title")}
      description={t("reservations.wizard.description")}
      permission={permissions.tenant.reservations.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("reservations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("reservations.title"),
          href: routes.tenant.reservations.root(slug),
        },
        { label: t("reservations.wizard.title") },
      ]}
    >
      <div className="space-y-4">
        <StepIndicator step={step} reachable={reachable} onStep={goto} />
        {selection}

        <div className={step === "search" ? undefined : "hidden"}>
          <EntitySectionCard title={t("reservations.wizard.steps.search")}>
            <AvailabilitySearchForm
              withDelivery
              pending={searchQuery.isFetching}
              onSearch={(request) => {
                setSearch(request);
                setProduct(null);
                invalidateQuote();
                setStep("product");
              }}
            />
          </EntitySectionCard>
        </div>

        {step === "product" ? (
          <EntitySectionCard title={t("reservations.wizard.steps.product")}>
            {deliveryProblems(t, searchQuery.error).length ? (
              <Alert variant="destructive" className="mb-4">
                <AlertTriangle className="size-4" />
                <AlertTitle>
                  {t("field_tasks.delivery.not_eligible")}
                </AlertTitle>
                <AlertDescription className="space-y-2">
                  <ul className="list-inside list-disc">
                    {deliveryProblems(t, searchQuery.error).map((text) => (
                      <li key={text}>{text}</li>
                    ))}
                  </ul>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => setStep("search")}
                  >
                    <ArrowLeft className="size-3.5" />
                    {t("field_tasks.delivery.change")}
                  </Button>
                </AlertDescription>
              </Alert>
            ) : null}
            {searchQuery.data?.warnings.length ? (
              <Alert className="mb-4">
                <AlertDescription>
                  {searchQuery.data.warnings
                    .map((w) => reasonText(t, WARNING_KEYS, w))
                    .join(" · ")}
                </AlertDescription>
              </Alert>
            ) : null}
            <WizardProductTable
              items={searchQuery.data?.items ?? []}
              selected={product?.product.uuid ?? null}
              isLoading={searchQuery.isLoading}
              isError={searchQuery.isError}
              onRetry={() => void searchQuery.refetch()}
              onSelect={(item) => {
                setProduct(item);
                invalidateQuote();
                setStep("customer");
              }}
            />
          </EntitySectionCard>
        ) : null}

        {step === "customer" ? (
          <EntitySectionCard title={t("reservations.wizard.steps.customer")}>
            <WizardCustomerStep
              customerUuid={customerUuid}
              onCustomerChange={(uuid) => {
                setCustomerUuid(uuid);
                setDrivers({ selected: {}, primary: "" });
                invalidateQuote();
              }}
              drivers={drivers}
              onDriversChange={setDrivers}
            />
            <div className="mt-6 flex justify-between gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setStep("product")}
              >
                <ArrowLeft className="size-4" />
                {t("reservations.wizard.back")}
              </Button>
              <Button
                type="button"
                disabled={!customerUuid}
                onClick={() => setStep("extras")}
              >
                {t("reservations.wizard.next")}
                <ArrowRight className="size-4" />
              </Button>
            </div>
          </EntitySectionCard>
        ) : null}

        {step === "extras" && search ? (
          <EntitySectionCard title={t("reservations.wizard.steps.extras")}>
            <WizardExtrasStep
              search={search}
              quantities={quantities}
              onQuantitiesChange={(next) => {
                setQuantities(next);
                invalidateQuote();
              }}
              discount={discount}
              onDiscountChange={(next) => {
                setDiscount(next);
                setCouponError(null);
                invalidateQuote();
              }}
              couponError={couponError}
            />
            {quoteError ? (
              <Alert variant="destructive" className="mt-4">
                <AlertTriangle className="size-4" />
                <AlertTitle>
                  {t("reservations.problems.quote_failed")}
                </AlertTitle>
                <AlertDescription>{quoteError}</AlertDescription>
              </Alert>
            ) : null}
            <div className="mt-6 flex justify-between gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setStep("customer")}
              >
                <ArrowLeft className="size-4" />
                {t("reservations.wizard.back")}
              </Button>
              <Button
                type="button"
                disabled={
                  createQuote.isPending ||
                  !can(permissions.tenant.quotes.write) ||
                  (discount.manualValue.trim() !== "" &&
                    discount.coupon.trim() === "" &&
                    discount.manualReason.trim().length < 3)
                }
                onClick={() => void makeQuote()}
              >
                <Calculator className="size-4" />
                {t("reservations.wizard.make_quote")}
              </Button>
            </div>
          </EntitySectionCard>
        ) : null}

        {step === "summary" && quote ? (
          <div className="grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
            <EntitySectionCard
              title={t("reservations.wizard.quote_title")}
              actions={
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={createQuote.isPending}
                  onClick={() => void makeQuote()}
                >
                  <RefreshCw className="size-3.5" />
                  {t("reservations.problems.requote")}
                </Button>
              }
            >
              <QuoteSummary quote={quote} />
            </EntitySectionCard>
            <EntitySectionCard title={t("reservations.wizard.options_title")}>
              <div className="space-y-4">
                {problem ? (
                  <ProblemAlert
                    problem={problem}
                    canOverride={can(
                      permissions.tenant.reservations.overrideEligibility,
                    )}
                    overrideReason={overrideReason}
                    onOverrideReason={setOverrideReason}
                    onUseReplacement={(replacement) => {
                      setQuote(replacement);
                      setProblem(null);
                    }}
                    onRequote={() => void makeQuote()}
                    onStep={goto}
                    onOverride={() => {
                      if (options) void submit(options, overrideReason.trim());
                    }}
                    pending={createReservation.isPending}
                  />
                ) : null}
                <AppForm<OptionValues>
                  id={OPTIONS_FORM_ID}
                  className="space-y-4"
                  schema={optionSchema}
                  defaultValues={
                    options ?? {
                      mode: "standard",
                      note: "",
                      external_reference: "",
                    }
                  }
                  onSubmit={(values) => submit(values)}
                >
                  <AppRadioGroup
                    name="mode"
                    label={t("reservations.wizard.mode.label")}
                    options={modeOptions}
                  />
                  <AppInput
                    name="external_reference"
                    label={t("reservations.fields.external_reference")}
                    description={t(
                      "reservations.fields.external_reference_hint",
                    )}
                  />
                  <AppTextarea
                    name="note"
                    label={t("reservations.fields.note")}
                    rows={3}
                  />
                </AppForm>
                <div className="flex justify-between gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => setStep("extras")}
                  >
                    <ArrowLeft className="size-4" />
                    {t("reservations.wizard.back")}
                  </Button>
                  <Button
                    type="submit"
                    form={OPTIONS_FORM_ID}
                    disabled={createReservation.isPending}
                  >
                    <CheckCircle2 className="size-4" />
                    {t("reservations.wizard.create")}
                  </Button>
                </div>
              </div>
            </EntitySectionCard>
          </div>
        ) : null}
      </div>
    </EntityPage>
  );
}
