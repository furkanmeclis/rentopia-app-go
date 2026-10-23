# Organization (company) delete

## Platform delete is a soft delete

`DELETE /v1/platform/organizations/{uuid}` (handler `organizations/handler.Delete`,
use case `DeleteOrganization`) runs `SoftDeleteOrganization`: it sets
`organizations.deleted_at` and nothing else.

- Every organization lookup (`GetOrganizationBy*`, the tenant scope, the
  customer portal, notification sweeps) filters `deleted_at IS NULL`, so the
  company disappears from the platform list, `/t/{slug}` and the customer
  portal at once.
  The organization cache entry is invalidated in the same call.
- The slug is unique only among live rows (`uq_organizations_slug_live`), so
  it can be reused by a new company.
- All tenant data (fleet, products, pricing, reservations, rentals, contracts,
  payments, the append-only ledger, …) stays in place. The delete hooks
  (`OnDeleted` of branches, finance, …) are deliberately no-ops.
- There is no restore endpoint and no API for a hard delete. This is the
  intended behaviour: accounting records and contracts must survive a company
  leaving the platform.

## Hard delete and the cascade guarantee

A hard delete is an operator action (data purge on request, removing a
company created by mistake) and what database tests do in their cleanup:

```sql
DELETE FROM organizations WHERE id = $1;
```

This single statement must remove the whole tenant. The schema guarantees it:

- every table with an `organization_id` column reaches `organizations`
  through `ON DELETE CASCADE`, directly or through its parent chain
  (`activity_events` is the exception: its `organization_id` is
  `ON DELETE SET NULL`, the platform keeps the audit trail);
- a foreign key between two tenant tables whose referencing side is deleted
  later in the cascade than the referenced side is `DEFERRABLE INITIALLY
  DEFERRED` (checked at commit, when both sides are gone). Since `000133`:
  `fk_rental_product_versions_group` and the three
  `fk_date_rule_version_targets_{product,segment,branch}` keys. They keep
  refusing a hard delete of a rental group, product, segment or branch on its
  own (product versions and activated date rules are immutable history).

`TestOrganizationHardDeleteCascades` (`internal/modules/organizations`) fills
an organization through the fleet, product, pricing, reservation, rental,
contract, payment and finance modules, deletes it and checks that no table
with an `organization_id` keeps a row of it. Run it with `TEST_DATABASE_URL`
when adding tenant tables.

### Rules for new tenant tables

- `organization_id BIGINT NOT NULL REFERENCES organizations (id) ON DELETE
  CASCADE`, or a composite key to a parent that cascades.
- A plain (`NO ACTION`) key to another tenant table is fine when the
  referencing table cascades from `organizations` at the same depth or
  closer than the referenced one. When it is deeper (for example
  organization → parent → child referencing a direct child of the
  organization), make the key `DEFERRABLE INITIALLY DEFERRED` instead of
  `ON DELETE CASCADE` unless deleting the referenced row should really delete
  the referencing one.
