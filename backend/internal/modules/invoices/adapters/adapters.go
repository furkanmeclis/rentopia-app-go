// Package adapters connects invoices to the shared engines: the list
// export (io engine) and the bulk actions (bulk engine): "invoice the
// selected rentals" (one draft per rental, reversible: the rollback cancels
// drafts that were not finalized) and "send" (finalize + send drafts).
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/invoices/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// MaxBulkTargets bounds one bulk run.
const MaxBulkTargets = 500

// Bulk action ids.
const (
	BulkCreateForRentals = "create_for_rentals"
	BulkSend             = "send"
)

var (
	errNoOrganization = errors.New("invoices: organization scope required")
	errNoImport       = errors.New("invoices: import is not supported")
)

func organizationID(ctx context.Context) (int64, error) {
	id, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return 0, errNoOrganization
	}
	return id, nil
}

func uuidList(raw string) []uuid.UUID {
	out := []uuid.UUID{}
	for _, p := range apiquery.SplitCSV(raw) {
		if id, err := uuid.Parse(p); err == nil {
			out = append(out, id)
		} else {
			out = append(out, uuid.Nil) // an unknown id matches nothing
		}
	}
	return out
}

func uuidPtr(raw string) *uuid.UUID {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		id = uuid.Nil
	}
	return &id
}

func strPtr(raw string) *string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return &raw
}

// Filter maps list query params (the list endpoint and the export / bulk
// query scope) to a filter.
func Filter(q map[string]string) model.ListFilter {
	f := model.ListFilter{
		Q: strings.TrimSpace(q["q"]), Statuses: apiquery.SplitCSV(q["status"]), Profiles: apiquery.SplitCSV(q["profile"]),
		Types: apiquery.SplitCSV(q["invoice_type"]), BranchUUIDs: uuidList(q["branch"]), AccountUUID: uuidPtr(q["account"]),
		RentalUUID: uuidPtr(q["rental"]), From: strPtr(q["from"]), To: strPtr(q["to"]),
	}
	if len(f.BranchUUIDs) == 0 {
		f.BranchUUIDs = nil
	}
	switch strings.ToLower(strings.TrimSpace(q["needs_action"])) {
	case "true", "1":
		v := true
		f.NeedsAction = &v
	case "false", "0":
		v := false
		f.NeedsAction = &v
	}
	return f
}

// IO exports invoices.
type IO struct{ svc *usecase.Service }

// NewIO builds the adapter (svc may be nil for metadata only).
func NewIO(svc *usecase.Service) *IO { return &IO{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *IO) Resource() string { return model.Resource }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "invoices." + key, Type: ioengine.ColumnTypeString}
	}
	return []ioengine.Column{
		col("number"), col("issue_date"), col("profile"), col("invoice_type"), col("status"), col("receiver_name"),
		col("receiver_identity_hint"), col("branch"), col("rental"), col("line_extension_amount"), col("tax_amount"),
		col("payable_amount"), col("currency"), col("external_ref"), col("last_error"),
		{Key: "uuid", LabelKey: "invoices.uuid", Type: ioengine.ColumnTypeUUID},
	}
}

func label(locale i18n.Locale, prefix, v string) string {
	if v == "" {
		return ""
	}
	key := prefix + v
	if t := i18n.Translate(locale, key); t != key {
		return t
	}
	return v
}

// Export implements ioengine.ResourceAdapter.
func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, locale i18n.Locale) (ioengine.Dataset, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f := Filter(query)
	f.Limit, f.SortField, f.SortDesc = usecase.MaxListTargets, "issue_date", true
	items, _, err := a.svc.List(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, it := range items {
		number, rental := "", ""
		if it.Number != nil {
			number = *it.Number
		}
		if it.Rental != nil {
			rental = it.Rental.Reference
		}
		rows = append(rows, map[string]any{
			"uuid": it.UUID.String(), "number": number, "issue_date": it.IssueDate, "profile": it.Profile,
			"invoice_type": it.Type, "status": label(locale, "invoices.status.", it.Status), "receiver_name": it.ReceiverName,
			"receiver_identity_hint": it.ReceiverHint, "branch": strings.TrimSpace(it.Branch.Code + " " + it.Branch.Name),
			"rental": rental, "line_extension_amount": it.LineExtension.String(), "tax_amount": it.Tax.String(),
			"payable_amount": it.Payable.String(), "currency": it.Currency, "external_ref": it.ExternalRef,
			"last_error": it.LastError,
		})
	}
	return ioengine.Dataset{Resource: model.Resource, Columns: a.ExportColumns(), Rows: rows}, nil
}

// ImportSchema implements ioengine.ResourceAdapter (no import).
func (a *IO) ImportSchema() []ioengine.ImportField { return []ioengine.ImportField{} }

// ApplyRow implements ioengine.ResourceAdapter (no import).
func (a *IO) ApplyRow(context.Context, map[string]any, map[string]any) (ioengine.RowResult, error) {
	return ioengine.RowResult{}, errNoImport
}

// RevertRow implements ioengine.ResourceAdapter (no import).
func (a *IO) RevertRow(context.Context, string, string, map[string]any) error { return errNoImport }

// bulk is the invoices bulk adapter.
type bulk struct{ svc *usecase.Service }

// NewBulk builds the bulk adapter (svc may be nil for metadata only).
func NewBulk(svc *usecase.Service) bulkengine.BulkAdapter { return &bulk{svc: svc} }

func (b *bulk) Resource() string { return model.Resource }

func (b *bulk) BulkActions() []bulkengine.BulkActionDef {
	return []bulkengine.BulkActionDef{
		{
			ID: BulkCreateForRentals, LabelKey: "bulk.actions.invoices.create_for_rentals",
			Permission: rbac.PermTenantInvoicesBulkCreateForRentals, Reversible: true,
			ConfirmKey: "bulk.confirm.invoices.create_for_rentals",
		},
		{
			ID: BulkSend, LabelKey: "bulk.actions.invoices.send", Permission: rbac.PermTenantInvoicesBulkSend,
			ConfirmKey: "bulk.confirm.invoices.send",
		},
	}
}

func dedupe(ids []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
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
}

// ResolveTargets: create_for_rentals takes rental uuids (ids) or the
// uninvoiced-rentals picker query (q); send takes invoice uuids or the list
// query (drafts only).
func (b *bulk) ResolveTargets(ctx context.Context, action string, target bulkengine.BulkTarget) ([]string, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return nil, err
	}
	switch action {
	case BulkCreateForRentals:
		switch target.Scope {
		case "ids":
			return dedupe(target.IDs)
		case "query":
			items, _, err := b.svc.UninvoicedRentals(ctx, orgID, model.RentalFilter{Q: target.Query["q"], Limit: MaxBulkTargets})
			if err != nil {
				return nil, err
			}
			out := make([]string, 0, len(items))
			for _, it := range items {
				out = append(out, it.UUID.String())
			}
			return out, nil
		}
	case BulkSend:
		switch target.Scope {
		case "ids":
			return dedupe(target.IDs)
		case "query":
			f := Filter(target.Query)
			f.Statuses, f.Limit = []string{model.StatusDraft}, MaxBulkTargets
			items, _, err := b.svc.List(ctx, orgID, f)
			if err != nil {
				return nil, err
			}
			out := make([]string, 0, len(items))
			for _, it := range items {
				out = append(out, it.UUID.String())
			}
			return out, nil
		}
	default:
		return nil, fmt.Errorf("unknown action")
	}
	return nil, fmt.Errorf("invalid target scope")
}

func itemError(entityUUID string, err error) (bulkengine.BulkItemResult, error) {
	var ve *usecase.ValidationError
	var de *usecase.DocumentError
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "not found"}, nil
	case errors.Is(err, usecase.ErrNothingToInvoice), errors.Is(err, usecase.ErrAlreadyInvoiced),
		errors.Is(err, usecase.ErrNotDraft), errors.As(err, &ve), errors.As(err, &de):
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: err.Error()}, nil
	}
	return bulkengine.BulkItemResult{}, err
}

func (b *bulk) ApplyItem(ctx context.Context, action, entityUUID string) (bulkengine.BulkItemResult, error) {
	orgID, err := organizationID(ctx)
	if err != nil {
		return bulkengine.BulkItemResult{}, err
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "invalid target"}, nil
	}
	switch action {
	case BulkCreateForRentals:
		// One draft per rental; the key makes a re-run of the same job item
		// answer the first draft.
		d, err := b.svc.CreateDraft(ctx, orgID, model.CreateInput{
			RentalUUID: &id, IdempotencyKey: "bulk:" + entityUUID + ":" + time.Now().UTC().Format("20060102"),
		})
		if errors.Is(err, usecase.ErrNotFound) {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "not found"}, nil
		}
		var ve *usecase.ValidationError
		if errors.As(err, &ve) && len(ve.Details) > 0 && ve.Details[0].Field == "rental_uuid" {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "not found"}, nil
		}
		if err != nil {
			return itemError(entityUUID, err)
		}
		return bulkengine.BulkItemResult{
			EntityUUID: entityUUID, EntityType: "rentals", OK: true, Op: "update",
			Previous: map[string]any{"invoice_uuid": d.UUID.String()},
		}, nil
	case BulkSend:
		d, err := b.svc.Get(ctx, orgID, id)
		if err != nil {
			return itemError(entityUUID, err)
		}
		if d.Status != model.StatusDraft {
			return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "not a draft"}, nil
		}
		if _, err := b.svc.Finalize(ctx, orgID, id); err != nil {
			return itemError(entityUUID, err)
		}
		return bulkengine.BulkItemResult{EntityUUID: entityUUID, EntityType: "invoices", OK: true, Op: "update"}, nil
	}
	return bulkengine.BulkItemResult{EntityUUID: entityUUID, Error: "unknown action"}, nil
}

// RevertItem cancels the draft a create_for_rentals item made (a draft
// that was finalized in the meantime stays: it is cancelled or returned by
// hand).
func (b *bulk) RevertItem(ctx context.Context, action, _ string, previous map[string]any) error {
	if action != BulkCreateForRentals {
		return fmt.Errorf("not reversible")
	}
	orgID, err := organizationID(ctx)
	if err != nil {
		return err
	}
	raw, _ := previous["invoice_uuid"].(string)
	id, err := uuid.Parse(raw)
	if err != nil {
		return err
	}
	d, err := b.svc.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	if d.Status != model.StatusDraft {
		return fmt.Errorf("the invoice is no longer a draft")
	}
	_, err = b.svc.Cancel(ctx, orgID, id, model.CancelInput{Reason: "bulk rollback"})
	return err
}

var (
	_ ioengine.ResourceAdapter = (*IO)(nil)
	_ bulkengine.BulkAdapter   = (*bulk)(nil)
)
