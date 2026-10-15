# Pricing (M2: TEC-675, TEC-699, TEC-700, TEC-701)

This doc covers the money and time rules, the pure engine, rate plans,
date rules and the preview, plus the versioned record pattern they share.

## Building blocks (TEC-675)

### Money: `backend/pkg/money`

- An amount is an `int64` count of minor units (kuruş) plus a currency
  (`TRY` by default). Every stored amount is `NUMERIC(14,2)`, i.e. less than
  10^14 kuruş, which fits an int64 with a lot of room. Addition, comparison and
  equality are exact, and the package needs no decimal library.
  Multiplying by a rate (VAT, a % rule, a % prepayment) goes through `math/big`,
  so the intermediate product cannot overflow, and is then rounded once.
- **There is one rounding rule: half away from zero, to the kuruş** (this is
  "half-up" for positive prices). `MulRate`, `Ratio`, `ParseRound` and
  `FromNumeric` of a wider numeric all apply it.
- In JSON an amount is always a string, `"1300.00"`. Input also accepts a
  number or a comma decimal separator. Rates are basis points (`money.Rate`,
  `Percent(20)` = 2000), so a percentage with two decimals is exact.
- pgx: `FromNumeric` / `Numeric()` convert to and from sqlc's `pgtype.Numeric`.
  `Money` also implements `NumericScanner` / `NumericValuer`.
- The `FormatMoney` helpers of branches, fleet, vehicles and fleetops now
  delegate to `money.FormatNumeric`. Their *input* parsers (regex +
  `pgtype.Numeric.Scan`) are unchanged, because they accept slightly different
  grammars (`+5`, `.5`). Moving them to `money.Parse` is a follow-up.

### Rental time: `backend/pkg/rentaltime`

- Instants are stored in UTC. Calendar decisions use Europe/Istanbul
  (`time/tzdata` is embedded).
- Ranges are half-open, `[start, end)`. A date rule "1 June – 1 September"
  covers June 1 to August 31.
- Billing days = `ceil(duration / 24h)`, at least 1. Exactly 24h is 1 day,
  24h plus 1 minute is 2 days, and 3h is 1 day.
- A billing day *starts* at `start + i·24h`. The Istanbul calendar date of
  that instant decides which date rule applies to that day.

### Versioned records

Both rate plans and date rules use the same pattern. It is modeled on product
versions (TEC-691) and settings versions (TEC-690):

| Piece | Rule |
|-------|------|
| Head row (`rate_plans`, `date_rules`) | Holds identity and what never changes a price (product, scope target, name), plus `status` (`draft` → `active` ↔ `inactive`), `current_version` (highest number) and the **current pointer** `active_version` |
| Version rows (`*_versions`) | Hold the price-affecting fields. `status` is `draft` / `active` / `superseded`, with **at most one draft and one active per head** (partial unique indexes) |
| Edits | Always write the single draft. The first edit after an activation copies the active version into draft N+1 (children too: bands, targets) |
| Activation | Validate (see below), then active → superseded and draft → active, then move the head pointer, all in one transaction under `SELECT … FOR UPDATE` on the head |
| Immutability | Triggers reject any content change of a non-draft version, and any insert / update / delete of its children (bands, targets). The only transition left is active → superseded. An organization delete still cascades: the child trigger finds no parent and lets it through |
| Consumers | Bind to a version uuid (reservations in M3). The preview breakdown reports plan / rule ids and versions |

A generic Go helper was not extracted. The two resources share the SQL shape,
but their children differ (bands vs targets), and a helper over sqlc queries
would add indirection without removing code. Copy the pattern instead (same
migration triggers, `ensure*Draft`, `activate*Tx`).

## Engine (TEC-699): `internal/platform/pricing`

`pricing.Calculate(Input) (Breakdown, error)` is pure. It does no database
access, no clock and no context. The caller loads the active candidates.

1. Billing days come from rentaltime.
2. Plan: among the plans that match the branch and channel, the most specific
   scope wins (branch+channel → branch → channel → common), then the highest
   priority. Two plans with the same scope and priority return
   `ErrAmbiguousPlan`, with both ids. The selected plan has **no fallback**: if
   it has no band for the length, the result is `ErrNoBand`.
3. One band is chosen from the whole rental length, and every day uses its
   daily price.
4. For each day, the single highest-priority rule that matches by date,
   product / segment, branch and channel is applied: `±%` or `±₺` on the unit
   price. A tie returns `ErrAmbiguousRule`. A day price is clamped at 0.
5. One discount (campaign / coupon hook for TEC-703) applies to the rental
   subtotal. It is capped, so the rental total never goes negative.
6. Extras, the one-way fee (resolved from fleetops one-way rules) and the
   delivery fee are added as lines.
7. **VAT:** prices include VAT. Each line (rental, each extra, each fee) is
   split as `base = round(gross / (1 + rate))` and `VAT = gross − base`. The
   totals are the sums of the lines, so `base_total + vat_total = total` to the
   kuruş. When the settings have no VAT rate, there is no split
   (`vat_rate: null`, VAT 0) and the preview shows a warning.
8. Prepayment follows the settings rule: `none`, `fixed` (capped at the total),
   `percent` of the total, or `full`.

Acceptance tables RT-A07..RT-A11 live in `pricing_test.go`, together with the
24h / +1 minute boundaries, the September 1 exclusion, negative clamps and
rounding. They run again through HTTP in
`modules/pricing/integration_test.go`.

## Rate plans (TEC-700)

- Tables: `rate_plans`, `rate_plan_versions` (priority 0..1000) and
  `rate_bands` (`min_days..max_days` inclusive, `max_days NULL` = open,
  `daily_price NUMERIC(14,2)`). Bands of one version cannot overlap (an
  `EXCLUDE` constraint over `int4range`).
- Scopes: `common`, `branch`, `channel`, `branch_channel`. A CHECK ties the
  branch and channel columns to the scope. Channels are the settings'
  sales-channel codes.
- **Online coverage.** A plan prices online sales when its channel is online,
  or when it has no channel and some online channel is switched on. Every
  channel except `branch` is online. Such a plan must cover every length from
  1 day up to `reservation.max_online_rental_days`, a new settings field
  (default 30). This is checked at activation, with `coverage_start`,
  `coverage_gap` and `coverage_short` issues.
- **Equal-priority conflict.** Activation is refused when another *active*
  plan of the same product and scope target has the same priority
  (`priority_conflict`, with a ref to that plan).
- `GET …/{uuid}/activation-check` lists the issues with their params.
  `POST …/activate` answers `422 ACTIVATION_BLOCKED` with the same codes.
- Matrix: `GET/PUT /v1/tenant/rate-plans/matrix` and `POST …/matrix/activate`
  work on one scope target (scope, branch, channel, priority). A save only
  turns *changed* rows into drafts. Activation is all or nothing.
- Import of the İdes price list: product code, band (`1-3`, `15+`), daily
  price, and optionally branch code, channel and priority. It only writes
  drafts, offers a dry-run diff, and rollback restores the price, removes the
  band, or deletes a plan the import created.
- Branch scope: branch plans are visible inside the member's branch scope.
  Common and channel plans are visible to everyone.

## Date rules (TEC-701)

- Tables: `date_rules` (name unique per organization, case-insensitive),
  `date_rule_versions` (`[start_date, end_date)`, `product_scope` =
  all / segments / products, `all_branches`, `channels` (NULL = all), kind
  percent (−100..1000) / amount, signed value, priority) and
  `date_rule_version_targets` (product / segment / branch, exactly one per row,
  with composite foreign keys).
- Activation is refused when an active rule has the same priority and its
  dates, products (a product list is intersected with segments through the
  product's segment), branches and channels overlap.
- The UI says explicitly that the end day is excluded ("bitiş günü hariç").

## Preview

`POST /v1/tenant/pricing/preview` prices one rental twice:

- **current** uses the active plans and rules;
- **proposed** applies the draft of `rate_plan_uuid` / `date_rule_uuid`, or an
  unsaved `rate_plan_draft` / `date_rule_draft`. An unsaved plan replaces the
  active plan with the same target and priority.

Engine refusals come back per side as `error.code` (`no_plan`,
`ambiguous_plan` …), not as HTTP errors. Every uuid in the body is resolved
inside the organization.

## Permissions

| Slug | owner / admin / merkez_yoneticisi | sube_yoneticisi, rezervasyon_satis, muhasebe, member | teslim_iade |
|------|---|---|---|
| `tenant.pricing.read` | ✓ | ✓ | – |
| `tenant.pricing.write` (drafts, matrix save, import) | ✓ | – | – |
| `tenant.pricing.activate` | ✓ | – | – |

## Open items

- Reservations bind to `rate_plan_versions.uuid` / `date_rule_versions.uuid`
  (M3). `Service.Quote` is the entry point.
- Coupons (TEC-703) plug into `Input.Discount`. Extras catalog: `Input.Extras`.
- The input parsers of older modules still parse money themselves (see above).
