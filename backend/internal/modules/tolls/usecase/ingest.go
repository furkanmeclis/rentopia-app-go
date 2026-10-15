package usecase

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tolls/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	providerPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{1,31}$`)
	currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)
)

// futureTolerance is how far a pass time may lie ahead of the server clock
// (provider clocks drift; a later time is a data error).
const futureTolerance = 24 * time.Hour

// NormalizePass validates and normalises one pass (provider fetch or file
// row). The field names are the import columns.
func (s *Service) NormalizePass(in model.PassInput) (model.PassInput, error) {
	ve := &ValidationError{}
	in.Provider = strings.ToLower(strings.TrimSpace(in.Provider))
	if !providerPattern.MatchString(in.Provider) {
		ve.add("provider", "invalid", "provider must be a lower-case code (2-32 letters, digits, _)")
	}
	in.ExternalID = strings.TrimSpace(in.ExternalID)
	if in.ExternalID == "" || len(in.ExternalID) > 120 {
		ve.add("external_id", "required", "the provider transaction id is required (at most 120 characters)")
	}
	if in.Source != model.SourceAPI && in.Source != model.SourceFile {
		ve.add("source", "invalid", "source must be api or file")
	}
	if in.PassedAt.IsZero() {
		ve.add("passed_at", "required", "the pass time is required")
	} else if in.PassedAt.After(s.clock().Add(futureTolerance)) {
		ve.add("passed_at", "future", "the pass time lies in the future")
	}
	in.PassedAt = in.PassedAt.UTC().Truncate(time.Second)
	in.Plate = strings.ToUpper(strings.Join(strings.Fields(in.Plate), " "))
	in.TagNumber = strings.TrimSpace(in.TagNumber)
	if in.Plate == "" && in.TagNumber == "" {
		ve.add("plate", "required", "a plate or a tag number is required")
	}
	if len(in.Plate) > 20 {
		ve.add("plate", "too_long", "plate is at most 20 characters")
	}
	if len(in.TagNumber) > 40 {
		ve.add("tag_number", "too_long", "tag number is at most 40 characters")
	}
	in.Station, in.ExitStation = strings.TrimSpace(in.Station), strings.TrimSpace(in.ExitStation)
	if in.Station == "" {
		ve.add("station", "required", "the station or route is required")
	}
	if len([]rune(in.Station)) > 160 || len([]rune(in.ExitStation)) > 160 {
		ve.add("station", "too_long", "station is at most 160 characters")
	}
	if in.Amount.IsNegative() {
		ve.add("amount", "invalid", "amount must be zero or positive")
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = "TRY"
	}
	if !currencyPattern.MatchString(in.Currency) {
		ve.add("currency", "invalid", "currency must be a 3-letter code")
	}
	if in.Raw == nil {
		in.Raw = map[string]any{}
	}
	return in, ve.orNil()
}

// lockByExternal loads a pass by its provider transaction id FOR UPDATE.
func lockByExternal(ctx context.Context, q *db.Queries, orgID int64, provider, external string) (pass, error) {
	r, err := q.LockTollPassByExternal(ctx, db.LockTollPassByExternalParams{OrganizationID: orgID, Provider: provider, ExternalID: external})
	return pass(r), err
}

// Ingest stores one reported pass and matches it:
//
//   - new transaction id → a new pass (event received), matched at once;
//   - the same facts again → only last_fetched_at moves (no second charge);
//   - changed facts → a revision row, the facts updated, and the matching
//     settles again: an amount change reverses the live charge and creates
//     a new one (never an edit in place).
//
// It does not publish live events (fetch jobs and imports publish once).
func (s *Service) Ingest(ctx context.Context, orgID int64, in model.PassInput) (model.IngestResult, error) {
	in, err := s.NormalizePass(in)
	if err != nil {
		return model.IngestResult{}, err
	}
	var out model.IngestResult
	var rentals []uuid.UUID
	err = s.tx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		out, rentals = model.IngestResult{}, nil
		p, err := lockByExternal(ctx, q, orgID, in.Provider, in.ExternalID)
		if notFound(err) {
			_, ierr := q.InsertTollPass(ctx, db.InsertTollPassParams{
				OrganizationID: orgID, Provider: in.Provider, ExternalID: in.ExternalID, Source: in.Source,
				IntegrationAccountID: int8Of(in.AccountID), PassedAt: ts(in.PassedAt), Plate: in.Plate,
				TagNumber: in.TagNumber, Station: in.Station, ExitStation: in.ExitStation, Currency: in.Currency,
				Amount: in.Amount.Numeric(), Raw: mustJSON(in.Raw),
			})
			if ierr != nil && !notFound(ierr) {
				return ierr
			}
			p, err = lockByExternal(ctx, q, orgID, in.Provider, in.ExternalID)
			if err != nil {
				return err
			}
			if ierr == nil {
				out.Op = model.OpCreated
				if err := passEvent(ctx, q, orgID, p.ID, model.EventReceived, "", p.Status, "", map[string]any{
					"source": in.Source, "amount": in.Amount.String(),
				}); err != nil {
					return err
				}
				rentals, err = s.automatic(ctx, q, tx, orgID, &p, "")
				out.UUID, out.Status = p.Uuid, p.Status
				return err
			}
		} else if err != nil {
			return err
		}
		out.UUID = p.Uuid
		changed := changedFields(p, in)
		if len(changed) == 0 {
			out.Op, out.Status = model.OpUnchanged, p.Status
			return q.TouchTollPassFetched(ctx, db.TouchTollPassFetchedParams{OrganizationID: orgID, ID: p.ID})
		}
		before := passFacts(p)
		revision := p.Revision + 1
		if err := q.UpdateTollPassFacts(ctx, db.UpdateTollPassFactsParams{
			OrganizationID: orgID, ID: p.ID, PassedAt: ts(in.PassedAt), Plate: in.Plate, TagNumber: in.TagNumber,
			Station: in.Station, ExitStation: in.ExitStation, Currency: in.Currency, Amount: in.Amount.Numeric(),
			Raw: mustJSON(in.Raw), Revision: revision, Source: in.Source, IntegrationAccountID: int8Of(in.AccountID),
		}); err != nil {
			return err
		}
		fresh, err := lockByExternal(ctx, q, orgID, in.Provider, in.ExternalID)
		if err != nil {
			return err
		}
		after := passFacts(fresh)
		if err := q.InsertTollPassRevision(ctx, db.InsertTollPassRevisionParams{
			OrganizationID: orgID, TollPassID: p.ID, Revision: revision, Source: in.Source, ChangedFields: changed,
			Previous: mustJSON(before), Current: mustJSON(after), AmountBefore: p.Amount, AmountAfter: in.Amount.Numeric(),
			CreatedBy: actorID(ctx),
		}); err != nil {
			return err
		}
		if err := passEvent(ctx, q, orgID, p.ID, model.EventRevised, "", "", "", map[string]any{
			"revision": revision, "changed_fields": changed, "amount_before": money.FromNumericOrZero(p.Amount).String(),
			"amount_after": in.Amount.String(),
		}); err != nil {
			return err
		}
		reason := "toll pass revision " + strconv.Itoa(int(revision)) + ": " + strings.Join(changed, ", ")
		rentals, err = s.automatic(ctx, q, tx, orgID, &fresh, reason)
		out.Op, out.Status, out.ChangedFields, out.Previous = model.OpRevised, fresh.Status, changed, before
		return err
	})
	if err != nil {
		return model.IngestResult{}, err
	}
	if len(rentals) > 0 {
		s.publish(ctx, orgID, "updated", nil, rentals)
	}
	return out, nil
}

// changedFields lists the facts of in that differ from the stored pass.
func changedFields(p pass, in model.PassInput) []string {
	out := []string{}
	if !p.PassedAt.Time.UTC().Equal(in.PassedAt) {
		out = append(out, "passed_at")
	}
	if p.Plate != in.Plate {
		out = append(out, "plate")
	}
	if p.TagNumber != in.TagNumber {
		out = append(out, "tag_number")
	}
	if p.Station != in.Station {
		out = append(out, "station")
	}
	if p.ExitStation != in.ExitStation {
		out = append(out, "exit_station")
	}
	if !money.FromNumericOrZero(p.Amount).Equal(in.Amount) {
		out = append(out, "amount")
	}
	if strings.TrimSpace(p.Currency) != in.Currency {
		out = append(out, "currency")
	}
	return out
}

// PreviewPass dry-runs one pass: validation, and whether it would be new,
// a revision (with the changed fields) or unchanged.
func (s *Service) PreviewPass(ctx context.Context, orgID int64, in model.PassInput) (string, []string, map[string]any, error) {
	in, err := s.NormalizePass(in)
	if err != nil {
		return "", nil, nil, err
	}
	r, err := s.q.LockTollPassByExternal(ctx, db.LockTollPassByExternalParams{OrganizationID: orgID, Provider: in.Provider, ExternalID: in.ExternalID})
	if notFound(err) {
		return model.OpCreated, nil, nil, nil
	}
	if err != nil {
		return "", nil, nil, err
	}
	p := pass(r)
	changed := changedFields(p, in)
	if len(changed) == 0 {
		return model.OpUnchanged, nil, nil, nil
	}
	return model.OpRevised, changed, passFacts(p), nil
}

// RevertImported undoes an imported row (import rollback): a pass the
// import created is removed after its charges were reversed; a revision is
// answered by a new revision back to the previous facts (charges settle
// again: reversal + new charge).
func (s *Service) RevertImported(ctx context.Context, orgID int64, id uuid.UUID, previous map[string]any) error {
	if op, _ := previous["op"].(string); op != model.OpCreated {
		facts, _ := previous["facts"].(map[string]any)
		if facts == nil {
			return nil
		}
		in := model.PassInput{Source: model.SourceFile, Raw: map[string]any{}}
		in.Provider, _ = facts["provider"].(string)
		in.ExternalID, _ = facts["external_id"].(string)
		in.Plate, _ = facts["plate"].(string)
		in.TagNumber, _ = facts["tag_number"].(string)
		in.Station, _ = facts["station"].(string)
		in.ExitStation, _ = facts["exit_station"].(string)
		in.Currency, _ = facts["currency"].(string)
		if v, ok := facts["passed_at"].(string); ok {
			in.PassedAt, _ = time.Parse(time.RFC3339Nano, v)
		}
		if v, ok := facts["amount"].(string); ok {
			in.Amount, _ = money.Parse(v)
		}
		_, err := s.Ingest(ctx, orgID, in)
		return err
	}
	var rentals []uuid.UUID
	err := s.tx(ctx, func(q *db.Queries, tx pgx.Tx) error {
		r, err := q.LockTollPassByUUID(ctx, db.LockTollPassByUUIDParams{OrganizationID: orgID, Uuid: id})
		if notFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		p := r
		rentals, err = s.settle(ctx, q, tx, orgID, &p, target{status: model.StatusIgnored, vehicleID: p.VehicleID, method: p.MatchMethod, manual: true}, "import rolled back")
		if err != nil {
			return err
		}
		return q.DeleteTollPass(ctx, db.DeleteTollPassParams{OrganizationID: orgID, ID: p.ID})
	})
	if err != nil {
		return err
	}
	s.publish(ctx, orgID, "deleted", []uuid.UUID{id}, rentals)
	return nil
}
