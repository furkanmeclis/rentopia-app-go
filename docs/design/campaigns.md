# Campaigns and coupons (TEC-703, RT-A12)

Status: **implemented** (M2). Code: `backend/internal/modules/campaigns` (`usecase` = CRUD, `Evaluate`, `Redeem` / `Consume` / `Release`). Migrations 000070 (tables), 000071 (permissions).

## 1. Model

`campaigns` (soft delete):

| Field | Meaning |
|-------|---------|
| `campaign_type` | `coupon` (applied by `code`) or `repeat_customer` (applied automatically) |
| `code` | coupons only; 3-32 of `A-Z 0-9 - _`, stored upper case, unique per organization among live campaigns (case-insensitive index) |
| `discount_kind` / `discount_value` | `percent` (0 < v ≤ 100) or `amount` (TRY) |
| `max_discount` | cap of the discount; `min_subtotal`: minimum rental subtotal |
| `start_date` / `end_date` | inclusive validity, Europe/Istanbul calendar of the moment the code is used (NULL = open) |
| `channels` | sales channel codes; NULL = every channel |
| `product_scope` + `campaign_targets` | `all`, `segments` or `products` (composite FKs) |
| `customer_uuids` | optional customer list, **no foreign key** until the customers table exists (TEC-705) |
| `usage_limit` / `used_count` | total uses; `CHECK (used_count <= usage_limit)` |
| `per_customer_limit` | uses per customer |
| `min_completed_rentals` | repeat customer: completed rentals in the organization |
| `release_on_cancel` | a cancelled / refunded reservation gives the use back (refund policy) |
| `corporate_eligible` | may be used on top of corporate agreement prices |
| `is_active` | switch |

Computed `state` (lists, facets): `inactive` → `expired` → `scheduled` → `exhausted` → `active` (first match).

`campaign_redemptions`: one row per reservation that used a campaign — `reserved` → `consumed` / `released` (`release_reason`, `usage_returned`). A unique index on `(organization_id, reservation_uuid) WHERE status <> 'released'` allows **one discount per reservation** (a released one may be replaced). `reservation_uuid` / `customer_uuid` have no foreign key until TEC-706 / TEC-705.

## 2. Eligibility (`Evaluate`)

`Evaluate(org, Quote{Code | CampaignUUID, ProductID, SegmentID, Channel, Subtotal, CustomerUUID, Corporate, At})` never consumes anything. Without a code it picks the repeat-customer campaign with the largest discount the customer qualifies for. A rejection is an answer, not an error — reason codes:

`not_found`, `inactive`, `not_started`, `expired`, `channel`, `scope`, `customer_scope`, `customer_required`, `customer_limit`, `limit`, `min_amount` (params `min_subtotal`, `subtotal`), `corporate`, `not_eligible` (repeat customer below `min_completed_rentals`).

Repeat customers: `CompletedRentalCounter` interface (rentals arrive in M3). `ZeroRentalCounter` answers 0, so repeat-customer campaigns are configurable now and apply once M3 plugs a real counter (`Service.SetRentalCounter`).

## 3. Redemption and the atomic counter (`Redeem`)

One transaction:

1. every rule except the total limit (`*RejectionError` otherwise);
2. `UPDATE campaigns SET used_count = used_count + 1 WHERE id = $1 AND is_active AND (usage_limit IS NULL OR used_count < usage_limit) RETURNING used_count` — no row = `limit`. The UPDATE row-locks the campaign: concurrent redemptions of one campaign queue on it and each re-evaluates the condition against the latest committed row (READ COMMITTED re-check), so the limit is never exceeded; the CHECK constraint is the backstop;
3. the per-customer count, read after the lock (two concurrent uses by one customer cannot both pass); `customer_required` when the campaign has a per-customer limit and no customer is given;
4. insert the redemption (`ErrAlreadyRedeemed` on the unique index). Any failure rolls back the counter.

`Consume(org, reservation)` marks it consumed; `Release(org, reservation, reason)` releases it and, when `release_on_cancel`, decrements the counter (`usage_returned = true`).

`TestRT_A12_LastRedemptionConcurrency` (limit 1 with 2 and 8 goroutines, limit 3 with 10) and `TestPerCustomerLimitConcurrency` run with `-race -count=20`: successes = limit exactly, `used_count` = limit.

## 4. Pricing integration

The engine has a single discount on the rental subtotal (`pricing.Input.Discount`): `percent` of the subtotal or `amount`, capped by `MaxAmount` (`DiscountLine.Capped`), never above the subtotal (the rental total never goes negative). `CampaignID` is carried to the breakdown.

`POST /v1/tenant/pricing/preview` accepts `coupon_code` and `customer_uuid`. Each side (current / proposed) is priced once without a discount to get its rental subtotal, the campaign is evaluated against it (minimum amount, percent base), and an applied campaign is the discount of the second pass. `side.coupon` = `{code, campaign_uuid, campaign_name, status: applied|rejected, reason, params}`. A coupon and a manual `discount` together are refused (`400`). Using a code needs no permission of its own; a manual discount at sale time stays `tenant.discount.apply` (TEC-706).

## 5. Permissions (migration 000071)

| Slug | owner / admin / merkez_yoneticisi | sube_yoneticisi, rezervasyon_satis, muhasebe, member | teslim_iade |
|------|---|---|---|
| `tenant.campaigns.read` | ✓ | ✓ | – |
| `tenant.campaigns.write` | ✓ | – | – |

## 6. Open items

- TEC-706 calls `Redeem` when a reservation is created with a code (subtotal from its quote), `Consume` on completion and `Release` on cancel / refund. No HTTP redeem route exists yet.
- `customer_uuids` gets its foreign key (or a join table) when customers land (TEC-705); M3 provides the completed-rental counter and the corporate agreement flag of a sale.
