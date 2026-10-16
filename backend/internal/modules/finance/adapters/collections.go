package adapters

// Collections engines (TEC-718): the list export and the bulk "allocate
// the credit to the oldest open items" action (reversible: the rollback
// releases what the run allocated).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/google/uuid"
)

// CollectionFilter maps list query params to a collection filter.
func CollectionFilter(q map[string]string) model.CollectionFilter {
	return model.CollectionFilter{
		Q: strings.TrimSpace(q["q"]), Methods: csv(q["method"]), Statuses: csv(q["status"]),
		AllocationStatuses: csv(q["allocation_status"]), BranchUUIDs: uuids(q["branch"]), CashAccountUUIDs: uuids(q["cash_account"]),
		AccountUUID: uuidPtr(q["account"]), RentalUUID: uuidPtr(q["rental"]), From: datePtr(q["from"]), To: datePtr(q["to"]),
	}
}

// CollectionsIO exports collections.
type CollectionsIO struct{ svc *usecase.Service }

// NewCollectionsIO builds the adapter.
func NewCollectionsIO(svc *usecase.Service) *CollectionsIO { return &CollectionsIO{svc: svc} }

func (a *CollectionsIO) Resource() string { return model.ResourceCollections }

func (a *CollectionsIO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "finance_collections." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("code"), col("collection_date"), col("account"), col("method"), col("reference_no"), col("payer_name"),
		col("branch"), col("cash_account"), col("rental"), col("amount"), col("gross_amount"), col("commission_amount"),
		col("net_amount"), col("allocated_amount"), col("unallocated_amount"), col("refunded_amount"), col("currency"),
		col("status"), col("allocation_status"), col("settlement_status"), col("note"),
		{Key: "uuid", LabelKey: "finance_collections.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func (a *CollectionsIO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := CollectionFilter(query)
	f.Limit, f.SortField, f.SortDesc = usecase.MaxListTargets, "collection_date", true
	items, _, err := a.svc.ListCollections(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		rental := ""
		if it.Rental != nil {
			rental = it.Rental.Code
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "code": it.Code, "collection_date": it.CollectionDate,
			"account": strings.TrimSpace(it.Account.Code + " " + it.Account.Name), "method": label(locale, "finance.method.", it.Method),
			"reference_no": it.ReferenceNo, "payer_name": it.PayerName, "branch": refText(it.Branch), "cash_account": refText(it.CashAccount),
			"rental": rental, "amount": it.Amount.String(), "gross_amount": it.GrossAmount.String(),
			"commission_amount": it.CommissionAmount.String(), "net_amount": it.NetAmount.String(),
			"allocated_amount": it.AllocatedAmount.String(), "unallocated_amount": it.UnallocatedAmount.String(),
			"refunded_amount": it.RefundedAmount.String(), "currency": it.Currency,
			"status":            label(locale, "finance.collection_status.", it.Status),
			"allocation_status": label(locale, "finance.allocation_status.", it.AllocationStatus),
			"settlement_status": label(locale, "finance.settlement_status.", it.SettlementStatus), "note": it.Note,
		})
	}
	return ioengine.Dataset{Resource: model.ResourceCollections, Columns: a.ExportColumns(), Rows: rows}, nil
}

func (a *CollectionsIO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

func (a *CollectionsIO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

func (a *CollectionsIO) RevertRow(context.Context, string, string, map[string]any) error {
	return errNoImport
}

// collectionsBulk is the "allocate" bulk action of collections.
type collectionsBulk struct{ svc *usecase.Service }

// NewCollectionsBulk builds the collections bulk adapter (svc may be nil
// for metadata only).
func NewCollectionsBulk(svc *usecase.Service) bulkengine.BulkAdapter {
	return &collectionsBulk{svc: svc}
}

// BulkAllocate is the bulk action id.
const BulkAllocate = "allocate"

func (b *collectionsBulk) Resource() string { return model.ResourceCollections }

func (b *collectionsBulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{{
		ID: BulkAllocate, LabelKey: "bulk.actions.finance_collections.allocate",
		Permission: rbac.PermTenantFinanceCollectionsBulkAllocate, Reversible: true,
	}}
}

func (b *collectionsBulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	if action != BulkAllocate {
		return nil, fmt.Errorf("unknown action")
	}
	switch target.Scope {
	case "ids":
		seen := map[string]bool{}
		out := make([]string, 0, len(target.IDs))
		for _, raw := range target.IDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil || seen[id.String()] {
				continue
			}
			seen[id.String()] = true
			out = append(out, id.String())
		}
		if len(out) > MaxBulkTargets {
			return nil, fmt.Errorf("at most %d targets are allowed", MaxBulkTargets)
		}
		return out, nil
	case "query":
		f := CollectionFilter(target.Query)
		f.Limit = MaxBulkTargets
		items, _, err := b.svc.ListCollections(ctx, orgID, f)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(items))
		for _, it := range items {
			if it.UnallocatedAmount.IsPositive() && it.AllocationStatus != model.AllocNotAllocatable {
				out = append(out, it.UUID.String())
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("invalid target scope")
}

func (b *collectionsBulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil || action != BulkAllocate {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "invalid target"}, nil
	}
	key := uuid.New()
	applied, err := b.svc.AllocateCredit(ctx, orgID, id, key)
	if err != nil {
		var ve *usecase.ValidationError
		var se *usecase.StateError
		switch {
		case errors.Is(err, usecase.ErrCollectionNotFound):
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "not found"}, nil
		case errors.As(err, &ve), errors.As(err, &se):
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: err.Error()}, nil
		}
		return bulkengine.BulkItemResult{}, err
	}
	if !applied.IsPositive() {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "nothing to allocate"}, nil
	}
	return bulkengine.BulkItemResult{
		EntityUUID: entityUUID, EntityType: "finance_collections", OK: true, Op: "update",
		Previous: map[string]any{"request_key": key.String(), "amount": applied.String()},
	}, nil
}

func (b *collectionsBulk) RevertItem(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	key, err := uuid.Parse(cell(previous, "request_key"))
	if err != nil {
		return err
	}
	return b.svc.UndoAllocation(ctx, orgID, id, key)
}

var (
	_ ioengine.ResourceAdapter = (*CollectionsIO)(nil)
	_ bulkengine.BulkAdapter   = (*collectionsBulk)(nil)
)
