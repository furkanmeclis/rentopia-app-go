// Package adapters connects toll passes to the io engine: the list export
// and the file import fallback (CSV / XLSX / TSV / JSON) for organizations
// whose provider has no API or while it is not chosen (TEC-683). The
// organization (and the requester's scope) come from the context: the
// request scope, or the import / export job's scope in the worker.
//
// Import column mapping (header → field; Turkish and English headers are
// suggested automatically from the label keys):
//
//	external_id   provider transaction id (required, unique per provider)
//	passed_at     pass time (required): RFC 3339, "2006-01-02 15:04[:05]"
//	              or "02.01.2006 15:04[:05]" (Istanbul time without a zone)
//	plate         plate (plate or tag_number required)
//	tag_number    HGS / OGS tag number
//	station       station or route entry (required)
//	exit_station  route exit (optional)
//	amount        amount (required): "47.00", "47,00" or "1.234,56"
//	currency      3-letter code (default TRY)
//	provider      provider / issuer code (default "hgs")
//
// A row that arrives again (same provider + external_id) updates the same
// pass: unchanged rows change nothing, changed facts are a revision. Every
// invalid row is reported with its row number and field; valid rows are
// stored.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/i18n"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Resource is the engine slug.
const Resource = model.Resource

// DefaultProvider is the provider code of file rows without one.
const DefaultProvider = "hgs"

var errNoOrganization = errors.New("tolls: organization scope is required")

// IO is the toll pass io adapter.
type IO struct{ svc *usecase.Service }

// NewIO builds the adapter.
func NewIO(svc *usecase.Service) *IO { return &IO{svc: svc} }

// Resource implements ioengine.ResourceAdapter.
func (a *IO) Resource() string { return Resource }

// ExportColumns implements ioengine.ResourceAdapter.
func (a *IO) ExportColumns() []ioengine.Column {
	col := func(key string, t ioengine.ColumnType) ioengine.Column {
		return ioengine.Column{Key: key, LabelKey: "tolls." + key, Type: t}
	}
	s := ioengine.ColumnTypeString
	return []ioengine.Column{
		col("passed_at", ioengine.ColumnTypeDatetime), col("plate", s), col("tag_number", s), col("station", s),
		col("exit_station", s), col("amount", s), col("currency", s), col("fee_amount", s), col("status", s),
		col("review_reason", s), col("rental", s), col("vehicle", s), col("provider", s), col("external_id", s),
		col("source", s), col("revision", s), col("uuid", ioengine.ColumnTypeUUID),
	}
}

// Export implements ioengine.ResourceAdapter (list filters honoured).
func (a *IO) Export(ctx context.Context, query ioengine.ExportQuery, loc i18n.Locale) (ioengine.Dataset, error) {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return ioengine.Dataset{}, errNoOrganization
	}
	f, err := model.FilterFromQuery(query)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	f.Limit = usecase.MaxExport
	if s := strings.TrimSpace(query["sort"]); s != "" {
		f.SortField, f.SortDesc = strings.TrimPrefix(s, "-"), strings.HasPrefix(s, "-")
		if _, ok := model.Sortable[f.SortField]; !ok {
			f.SortField, f.SortDesc = "", false
		}
	}
	items, _, err := a.svc.List(ctx, orgID, f)
	if err != nil {
		return ioengine.Dataset{}, err
	}
	rows := make([]map[string]any, 0, len(items))
	for _, p := range items {
		rental, vehicle, fee := "", "", ""
		if p.Rental != nil {
			rental = p.Rental.Reference
		}
		if p.Vehicle != nil {
			vehicle = p.Vehicle.Plate
		}
		if p.FeeAmount != nil {
			fee = p.FeeAmount.String()
		}
		reason := ""
		if p.ReviewReason != "" {
			reason = i18n.Translate(loc, "tolls.review_reason."+p.ReviewReason)
		}
		rows = append(rows, map[string]any{
			"passed_at": p.PassedAt, "plate": p.Plate, "tag_number": p.TagNumber, "station": p.Station,
			"exit_station": p.ExitStation, "amount": p.Amount.String(), "currency": p.Currency, "fee_amount": fee,
			"status": i18n.Translate(loc, "tolls.status."+p.Status), "review_reason": reason, "rental": rental,
			"vehicle": vehicle, "provider": p.Provider, "external_id": p.ExternalID,
			"source": i18n.Translate(loc, "tolls.source."+p.Source), "revision": fmt.Sprintf("%d", p.Revision),
			"uuid": p.UUID.String(),
		})
	}
	return ioengine.Dataset{Resource: Resource, Columns: a.ExportColumns(), Rows: rows}, nil
}

// ImportSchema implements ioengine.ResourceAdapter.
func (a *IO) ImportSchema() []ioengine.ImportField {
	f := func(key string, t ioengine.ColumnType, required bool, hint string) ioengine.ImportField {
		return ioengine.ImportField{Key: key, LabelKey: "tolls." + key, Type: t, Required: required, DefaultHint: hint}
	}
	s := ioengine.ColumnTypeString
	return []ioengine.ImportField{
		f("external_id", s, true, ""),
		f("passed_at", ioengine.ColumnTypeDatetime, true, ""),
		f("plate", s, false, ""),
		f("tag_number", s, false, ""),
		f("station", s, true, ""),
		f("exit_station", s, false, ""),
		f("amount", s, true, ""),
		f("currency", s, false, "TRY"),
		f("provider", s, false, DefaultProvider),
	}
}

// SampleRows implements ioengine.SampleProvider.
func (a *IO) SampleRows() []map[string]any {
	return []map[string]any{
		{
			"external_id": "HGS-2026-000123", "passed_at": "2026-05-04 08:42:10", "plate": "34 ABC 123",
			"tag_number": "HGS1234567", "station": "Osmangazi Köprüsü", "exit_station": "", "amount": "795,00",
			"currency": "TRY", "provider": DefaultProvider,
		},
		{
			"external_id": "HGS-2026-000124", "passed_at": "04.05.2026 09:15", "plate": "34 ABC 123",
			"tag_number": "HGS1234567", "station": "Gebze", "exit_station": "Orhangazi", "amount": "412.50",
			"currency": "TRY", "provider": DefaultProvider,
		},
	}
}

func cell(row map[string]any, key string) string {
	v, ok := row[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case time.Time:
		return t.Format(time.RFC3339)
	case float64:
		return fmt.Sprintf("%.2f", t)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

var timeLayouts = []string{
	"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02T15:04",
	"02.01.2006 15:04:05", "02.01.2006 15:04", "02/01/2006 15:04:05", "02/01/2006 15:04",
}

// ParseTime reads a pass time: RFC 3339, or a local Istanbul time.
func ParseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	for _, l := range timeLayouts {
		if t, err := time.ParseInLocation(l, s, model.Istanbul); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unreadable time %q", s)
}

// ParseAmount reads "47.00", "47,00" or "1.234,56" (Turkish thousands).
func ParseAmount(s string) (money.Money, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "TL"), "₺")
	if strings.Contains(s, ",") && strings.Contains(s, ".") {
		if strings.LastIndex(s, ",") > strings.LastIndex(s, ".") {
			s = strings.ReplaceAll(s, ".", "")
		} else {
			s = strings.ReplaceAll(s, ",", "")
		}
	}
	return money.Parse(s)
}

// rowInput maps a file row; field errors name the import column.
func rowInput(row map[string]any, defaults map[string]any) (model.PassInput, error) {
	get := func(key string) string {
		if v := cell(row, key); v != "" {
			return v
		}
		if d, ok := defaults[key]; ok && d != nil {
			return strings.TrimSpace(fmt.Sprint(d))
		}
		return ""
	}
	ve := &usecase.ValidationError{}
	in := model.PassInput{
		Source: model.SourceFile, ExternalID: get("external_id"), Plate: get("plate"), TagNumber: get("tag_number"),
		Station: get("station"), ExitStation: get("exit_station"), Currency: get("currency"), Provider: get("provider"),
		Raw: map[string]any{"import": true},
	}
	if in.Provider == "" {
		in.Provider = DefaultProvider
	}
	if raw := get("passed_at"); raw == "" {
		ve.Details = append(ve.Details, usecase.Detail{Field: "passed_at", Code: "required", Message: "the pass time is required"})
	} else if t, err := ParseTime(raw); err != nil {
		ve.Details = append(ve.Details, usecase.Detail{Field: "passed_at", Code: "invalid", Message: err.Error()})
	} else {
		in.PassedAt = t
	}
	if raw := get("amount"); raw == "" {
		ve.Details = append(ve.Details, usecase.Detail{Field: "amount", Code: "required", Message: "amount is required"})
	} else if m, err := ParseAmount(raw); err != nil {
		ve.Details = append(ve.Details, usecase.Detail{Field: "amount", Code: "invalid", Message: fmt.Sprintf("unreadable amount %q", raw)})
	} else {
		in.Amount = m
	}
	if len(ve.Details) > 0 {
		return in, ve
	}
	return in, nil
}

// rowError joins the field problems of a row ("amount: …; plate: …").
func rowError(err error) string {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		parts := make([]string, 0, len(ve.Details))
		for _, d := range ve.Details {
			parts = append(parts, d.Field+": "+d.Message)
		}
		return strings.Join(parts, "; ")
	}
	return err.Error()
}

// validate maps and validates a row (both mapping and field errors).
func (a *IO) validate(row, defaults map[string]any) (model.PassInput, error) {
	in, err := rowInput(row, defaults)
	n, verr := a.svc.NormalizePass(in)
	if err != nil || verr != nil {
		merged := &usecase.ValidationError{}
		var ve *usecase.ValidationError
		if errors.As(err, &ve) {
			merged.Details = append(merged.Details, ve.Details...)
		}
		if errors.As(verr, &ve) {
			for _, d := range ve.Details {
				dup := false
				for _, m := range merged.Details {
					dup = dup || m.Field == d.Field
				}
				if !dup {
					merged.Details = append(merged.Details, d)
				}
			}
		}
		return in, merged
	}
	return n, nil
}

// PreviewRow implements ioengine.RowPreviewer: per-row validation and
// whether the row creates a pass, revises one or changes nothing.
func (a *IO) PreviewRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowDiff, error) {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return ioengine.RowDiff{}, errNoOrganization
	}
	in, err := a.validate(row, defaults)
	if err != nil {
		return ioengine.RowDiff{Error: rowError(err)}, nil
	}
	op, changed, before, err := a.svc.PreviewPass(ctx, orgID, in)
	if err != nil {
		return ioengine.RowDiff{Error: rowError(err)}, nil
	}
	switch op {
	case model.OpCreated:
		return ioengine.RowDiff{Op: "create"}, nil
	case model.OpUnchanged:
		return ioengine.RowDiff{Op: "update"}, nil
	}
	after := map[string]any{
		"passed_at": in.PassedAt, "plate": in.Plate, "tag_number": in.TagNumber, "station": in.Station,
		"exit_station": in.ExitStation, "amount": in.Amount.String(), "currency": in.Currency,
	}
	changes := make([]ioengine.FieldChange, 0, len(changed))
	for _, f := range changed {
		changes = append(changes, ioengine.FieldChange{Field: f, From: fmt.Sprint(before[f]), To: fmt.Sprint(after[f])})
	}
	return ioengine.RowDiff{Op: "update", Changes: changes}, nil
}

// ApplyRow implements ioengine.ResourceAdapter.
func (a *IO) ApplyRow(ctx context.Context, row map[string]any, defaults map[string]any) (ioengine.RowResult, error) {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return ioengine.RowResult{}, errNoOrganization
	}
	in, err := a.validate(row, defaults)
	if err != nil {
		return ioengine.RowResult{OK: false, Error: rowError(err)}, nil
	}
	res, err := a.svc.Ingest(ctx, orgID, in)
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		return ioengine.RowResult{OK: false, Error: rowError(err)}, nil
	}
	if err != nil {
		return ioengine.RowResult{}, err
	}
	out := ioengine.RowResult{OK: true, EntityType: "toll_pass", EntityUUID: res.UUID.String(), Op: "update"}
	switch res.Op {
	case model.OpCreated:
		out.Op, out.Previous = "create", map[string]any{"op": model.OpCreated}
	case model.OpRevised:
		out.Previous = map[string]any{"op": model.OpRevised, "facts": res.Previous}
	default:
		out.Previous = map[string]any{"op": model.OpUnchanged}
	}
	return out, nil
}

// RevertRow implements ioengine.ResourceAdapter (import rollback): a
// created pass is removed after its charges are reversed; a revision is
// answered by a revision back to the previous facts.
func (a *IO) RevertRow(ctx context.Context, _ string, entityUUID string, previous map[string]any) error {
	orgID, ok := orgctx.OrganizationID(ctx)
	if !ok {
		return errNoOrganization
	}
	id, err := uuid.Parse(entityUUID)
	if err != nil {
		return err
	}
	return a.svc.RevertImported(ctx, orgID, id, previous)
}
