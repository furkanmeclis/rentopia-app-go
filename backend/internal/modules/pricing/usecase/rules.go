package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/pricing/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// DateRuleSortColumns are the sortable rule list fields.
var DateRuleSortColumns = map[string]string{
	"name": "name", "start_date": "start_date", "priority": "priority", "status": "status",
	"created_at": "created_at", "updated_at": "updated_at",
}

type ruleRow = db.GetDateRuleRow

// ruleState is a validated, complete set of versioned rule fields.
type ruleState struct {
	Start, End   rentaltime.Date
	ProductScope string
	Products     []db.ListPricingProductsByUUIDsRow
	Segments     []db.ListPricingSegmentsByUUIDsRow
	AllBranches  bool
	Branches     []db.ListPricingBranchesByUUIDsRow
	// Channels nil = every channel.
	Channels []string
	Kind     string
	Value    money.Money
	Priority int
	Note     string
}

func toRule(row ruleRow) model.DateRule {
	return model.DateRule{
		UUID: row.Uuid, Name: row.Name, Status: row.Status, CurrentVersion: int(row.CurrentVersion),
		ActiveVersion: int4Ptr(row.ActiveVersion), ActivatedAt: timePtr(row.ActivatedAt),
		CreatedAt: row.CreatedAt.Time.UTC(), UpdatedAt: row.UpdatedAt.Time.UTC(),
	}
}

func toRuleListRow(row db.ListDateRulesRow) model.DateRule {
	out := toRule(ruleRow{
		Uuid: row.Uuid, Name: row.Name, Status: row.Status, CurrentVersion: row.CurrentVersion,
		ActiveVersion: row.ActiveVersion, ActivatedAt: row.ActivatedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	})
	out.HasDraft, out.Kind, out.Priority = row.HasDraft, row.Kind, int(row.Priority)
	out.ProductScope, out.AllBranches, out.AllChannels = row.ProductScope, row.AllBranches, row.AllChannels
	if row.StartDate.Valid {
		d := dateOf(row.StartDate)
		out.StartDate = &d
	}
	if row.EndDate.Valid {
		d := dateOf(row.EndDate)
		out.EndDate = &d
	}
	if row.Value.Valid {
		out.Value = money.FormatNumeric(row.Value)
	}
	return out
}

func toRuleVersion(row db.GetDateRuleVersionByStatusRow, targets []db.ListDateRuleTargetsRow) *model.RuleVersion {
	v := &model.RuleVersion{
		UUID: row.Uuid, Version: int(row.Version), Status: row.Status,
		StartDate: dateOf(row.StartDate), EndDate: dateOf(row.EndDate), ProductScope: row.ProductScope,
		Products: []model.ProductRef{}, Segments: []model.SegmentRef{}, AllBranches: row.AllBranches,
		Branches: []model.BranchRef{}, Channels: row.Channels, Kind: row.Kind, Value: money.FormatNumeric(row.Value),
		Priority: int(row.Priority), ChangeNote: row.ChangeNote, CreatedAt: row.CreatedAt.Time.UTC(),
		CreatedBy:    actorFrom(row.CreatedByUuid, row.CreatedByName, row.CreatedBySurname, row.CreatedByEmail),
		ActivatedAt:  timePtr(row.ActivatedAt),
		ActivatedBy:  actorFrom(row.ActivatedByUuid, row.ActivatedByName, row.ActivatedBySurname, row.ActivatedByEmail),
		SupersededAt: timePtr(row.SupersededAt),
	}
	for _, t := range targets {
		switch {
		case t.ProductUuid.Valid:
			v.Products = append(v.Products, model.ProductRef{UUID: t.ProductUuid.Bytes, Code: t.ProductCode.String, Title: t.ProductTitle.String})
		case t.SegmentUuid.Valid:
			v.Segments = append(v.Segments, model.SegmentRef{UUID: t.SegmentUuid.Bytes, Code: t.SegmentCode.String, Name: t.SegmentName.String})
		case t.BranchUuid.Valid:
			v.Branches = append(v.Branches, model.BranchRef{UUID: t.BranchUuid.Bytes, Code: t.BranchCode.String, Name: t.BranchName.String})
		}
	}
	return v
}

func ruleTargets(ctx context.Context, q *db.Queries, orgID int64, ids []int64) (map[int64][]db.ListDateRuleTargetsRow, error) {
	out := map[int64][]db.ListDateRuleTargetsRow{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := q.ListDateRuleTargets(ctx, db.ListDateRuleTargetsParams{OrganizationID: orgID, VersionIds: ids})
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.DateRuleVersionID] = append(out[r.DateRuleVersionID], r)
	}
	return out, nil
}

func ruleVersion(ctx context.Context, q *db.Queries, orgID, ruleID int64, status string) (*model.RuleVersion, *db.GetDateRuleVersionByStatusRow, error) {
	row, err := q.GetDateRuleVersionByStatus(ctx, db.GetDateRuleVersionByStatusParams{
		OrganizationID: orgID, DateRuleID: ruleID, Status: status,
	})
	if err != nil {
		if errors.Is(repository.MapErr(err), repository.ErrNotFound) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	targets, err := ruleTargets(ctx, q, orgID, []int64{row.ID})
	if err != nil {
		return nil, nil, err
	}
	return toRuleVersion(row, targets[row.ID]), &row, nil
}

// ListRules returns one page of date rules.
func (s *Service) ListRules(ctx context.Context, orgID int64, f model.DateRuleFilter) ([]model.DateRule, int64, error) {
	if _, ok := DateRuleSortColumns[f.SortField]; !ok {
		f.SortField, f.SortDesc = "start_date", false
	}
	q := s.repo.Q()
	rows, err := q.ListDateRules(ctx, db.ListDateRulesParams{
		OrganizationID: orgID, Statuses: strs(f.Statuses), Kinds: strs(f.Kinds), HasDraft: boolArg(f.HasDraft),
		DateFrom: dateArg(f.DateFrom), DateTo: dateArg(f.DateTo), Q: textArg(f.Q),
		SortField: f.SortField, SortDesc: f.SortDesc, LimitCount: f.Limit, OffsetCount: f.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountDateRules(ctx, db.CountDateRulesParams{
		OrganizationID: orgID, Statuses: strs(f.Statuses), Kinds: strs(f.Kinds), HasDraft: boolArg(f.HasDraft),
		DateFrom: dateArg(f.DateFrom), DateTo: dateArg(f.DateTo), Q: textArg(f.Q),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.DateRule, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRuleListRow(row))
	}
	return out, total, nil
}

// RuleFacets returns the dynamic filter options of the rule list.
func (s *Service) RuleFacets(ctx context.Context, orgID int64) (model.DateRuleFacets, error) {
	rows, err := s.repo.Q().ListDateRuleFacets(ctx, orgID)
	if err != nil {
		return model.DateRuleFacets{}, err
	}
	out := model.DateRuleFacets{Status: []model.Facet{}, Kind: []model.Facet{}}
	for _, row := range rows {
		f := model.Facet{Value: row.Value, Count: row.Count}
		switch row.Facet {
		case "status":
			out.Status = append(out.Status, f)
		case "kind":
			if row.Value != "" {
				out.Kind = append(out.Kind, f)
			}
		}
	}
	return out, nil
}

func (s *Service) getRule(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (ruleRow, error) {
	row, err := q.GetDateRule(ctx, db.GetDateRuleParams{OrganizationID: orgID, Uuid: id})
	if err != nil {
		return ruleRow{}, mapNotFound(repository.MapErr(err), ErrRuleNotFound)
	}
	return row, nil
}

func (s *Service) lockRule(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (ruleRow, error) {
	row, err := s.getRule(ctx, q, orgID, id)
	if err != nil {
		return ruleRow{}, err
	}
	if _, err := q.LockDateRule(ctx, db.LockDateRuleParams{OrganizationID: orgID, ID: row.ID}); err != nil {
		return ruleRow{}, mapNotFound(repository.MapErr(err), ErrRuleNotFound)
	}
	return s.getRule(ctx, q, orgID, id)
}

// GetRule returns one rule with its active and draft versions.
func (s *Service) GetRule(ctx context.Context, orgID int64, id uuid.UUID) (model.DateRule, error) {
	q := s.repo.Q()
	row, err := s.getRule(ctx, q, orgID, id)
	if err != nil {
		return model.DateRule{}, err
	}
	out := toRule(row)
	active, _, err := ruleVersion(ctx, q, orgID, row.ID, model.VersionActive)
	if err != nil {
		return model.DateRule{}, err
	}
	draft, _, err := ruleVersion(ctx, q, orgID, row.ID, model.VersionDraft)
	if err != nil {
		return model.DateRule{}, err
	}
	out.Active, out.Draft, out.HasDraft = active, draft, draft != nil
	eff := draft
	if eff == nil {
		eff = active
	}
	if eff != nil {
		start, end := eff.StartDate, eff.EndDate
		out.StartDate, out.EndDate, out.Kind, out.Value, out.Priority = &start, &end, eff.Kind, eff.Value, eff.Priority
		out.ProductScope, out.AllBranches, out.AllChannels = eff.ProductScope, eff.AllBranches, eff.Channels == nil
	}
	return out, nil
}

// RuleVersions lists every version of a rule (newest first).
func (s *Service) RuleVersions(ctx context.Context, orgID int64, id uuid.UUID) ([]model.RuleVersion, error) {
	q := s.repo.Q()
	row, err := s.getRule(ctx, q, orgID, id)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListDateRuleVersions(ctx, db.ListDateRuleVersionsParams{OrganizationID: orgID, DateRuleID: row.ID})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, v := range rows {
		ids = append(ids, v.ID)
	}
	targets, err := ruleTargets(ctx, q, orgID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]model.RuleVersion, 0, len(rows))
	for _, v := range rows {
		out = append(out, *toRuleVersion(db.GetDateRuleVersionByStatusRow(v), targets[v.ID]))
	}
	return out, nil
}

// RuleActivity lists the audit events of a rule.
func (s *Service) RuleActivity(ctx context.Context, orgID int64, id uuid.UUID, limit, offset int32) ([]model.ActivityEntry, int64, error) {
	if _, err := s.getRule(ctx, s.repo.Q(), orgID, id); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListActivity(ctx, orgID, model.ResourceDateRules, id, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return activityEntries(rows), total, nil
}

// stateFromVersion turns a stored version into a ruleState (for patches).
func stateFromVersion(row db.GetDateRuleVersionByStatusRow, targets []db.ListDateRuleTargetsRow) ruleState {
	st := ruleState{
		Start: dateOf(row.StartDate), End: dateOf(row.EndDate), ProductScope: row.ProductScope,
		AllBranches: row.AllBranches, Channels: row.Channels, Kind: row.Kind,
		Value: money.FromNumericOrZero(row.Value), Priority: int(row.Priority), Note: row.ChangeNote,
	}
	for _, t := range targets {
		switch {
		case t.RentalProductID.Valid:
			st.Products = append(st.Products, db.ListPricingProductsByUUIDsRow{ID: t.RentalProductID.Int64, Uuid: t.ProductUuid.Bytes, Code: t.ProductCode.String, Title: t.ProductTitle.String})
		case t.SegmentID.Valid:
			st.Segments = append(st.Segments, db.ListPricingSegmentsByUUIDsRow{ID: t.SegmentID.Int64, Uuid: t.SegmentUuid.Bytes, Code: t.SegmentCode.String, Name: t.SegmentName.String})
		case t.BranchID.Valid:
			st.Branches = append(st.Branches, db.ListPricingBranchesByUUIDsRow{ID: t.BranchID.Int64, Uuid: t.BranchUuid.Bytes, Code: t.BranchCode.String, Name: t.BranchName.String})
		}
	}
	return st
}

func defaultRuleState() ruleState {
	return ruleState{ProductScope: pricing.ProductsAll, AllBranches: true, Kind: pricing.KindPercent, Value: money.Zero()}
}

// applyRuleFields validates the request fields over st (nil = unchanged).
// Every uuid is resolved inside the organization; unknown ones are
// "not_found" details.
func (s *Service) applyRuleFields(ctx context.Context, q *db.Queries, orgID int64, ve *ValidationError, prefix string, st *ruleState, in model.DateRuleFields, create bool) {
	f := func(name string) string { return prefix + name }
	if in.StartDate != nil {
		st.Start = *in.StartDate
	} else if create {
		ve.add(f("start_date"), "required", "start_date is required")
	}
	if in.EndDate != nil {
		st.End = *in.EndDate
	} else if create {
		ve.add(f("end_date"), "required", "end_date is required")
	}
	if !ve.has(f("start_date")) && !ve.has(f("end_date")) && !st.End.After(st.Start) {
		ve.add(f("end_date"), "before_start", "end_date (excluded) must be after start_date")
	}
	if in.ProductScope != nil {
		st.ProductScope = *in.ProductScope
	}
	if !contains(model.ProductScopes, st.ProductScope) {
		ve.add(f("product_scope"), "invalid", "product_scope must be all, segments or products")
	}
	if in.ProductUUIDs != nil {
		st.Products = nil
		if ids := dedupe(*in.ProductUUIDs); len(ids) > 0 {
			rows, err := q.ListPricingProductsByUUIDs(ctx, db.ListPricingProductsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
			if err != nil || len(rows) != len(ids) {
				ve.add(f("rental_product_uuids"), "not_found", "rental product was not found")
			}
			st.Products = rows
		}
	}
	if in.SegmentUUIDs != nil {
		st.Segments = nil
		if ids := dedupe(*in.SegmentUUIDs); len(ids) > 0 {
			rows, err := q.ListPricingSegmentsByUUIDs(ctx, db.ListPricingSegmentsByUUIDsParams{OrganizationID: orgID, Uuids: ids})
			if err != nil || len(rows) != len(ids) {
				ve.add(f("segment_uuids"), "not_found", "segment was not found")
			}
			st.Segments = rows
		}
	}
	switch st.ProductScope {
	case pricing.ProductsAll:
		st.Products, st.Segments = nil, nil
	case pricing.ProductsSegments:
		st.Products = nil
		if len(st.Segments) == 0 && !ve.has(f("segment_uuids")) {
			ve.add(f("segment_uuids"), "required", "select at least one segment")
		}
	case pricing.ProductsSelected:
		st.Segments = nil
		if len(st.Products) == 0 && !ve.has(f("rental_product_uuids")) {
			ve.add(f("rental_product_uuids"), "required", "select at least one rental product")
		}
	}
	if in.AllBranches != nil {
		st.AllBranches = *in.AllBranches
	}
	if in.BranchUUIDs != nil {
		st.Branches = nil
		if ids := dedupe(*in.BranchUUIDs); len(ids) > 0 {
			rows, err := q.ListPricingBranchesByUUIDs(ctx, db.ListPricingBranchesByUUIDsParams{OrganizationID: orgID, Uuids: ids})
			if err != nil || len(rows) != len(ids) {
				ve.add(f("branch_uuids"), "not_found", "branch was not found")
			}
			st.Branches = rows
		}
	}
	if st.AllBranches {
		st.Branches = nil
	} else if len(st.Branches) == 0 && !ve.has(f("branch_uuids")) {
		ve.add(f("branch_uuids"), "required", "select at least one branch")
	}
	if in.AllChannels != nil && *in.AllChannels {
		st.Channels = nil
	} else if in.Channels != nil || (in.AllChannels != nil && !*in.AllChannels) {
		chs := []string{}
		if in.Channels == nil && st.Channels != nil {
			chs = st.Channels
		}
		if in.Channels != nil {
			for _, c := range *in.Channels {
				c = strings.TrimSpace(c)
				if !validChannel(c) {
					ve.add(f("channels"), "invalid", "channel is not a sales channel")
					break
				}
				if !contains(chs, c) {
					chs = append(chs, c)
				}
			}
		}
		if len(chs) == 0 && !ve.has(f("channels")) {
			ve.add(f("channels"), "required", "select at least one channel")
		}
		st.Channels = chs
	}
	if in.Kind != nil {
		st.Kind = *in.Kind
	}
	if !contains(model.RuleKinds, st.Kind) {
		ve.add(f("kind"), "invalid", "kind must be percent or amount")
	}
	if in.Value != nil {
		v, err := money.Parse(string(*in.Value))
		if err != nil {
			ve.add(f("value"), "invalid", "value must be a signed number with at most 2 decimals")
		} else {
			st.Value = v
		}
	} else if create {
		ve.add(f("value"), "required", "value is required")
	}
	if st.Kind == pricing.KindPercent && (st.Value.Minor() < -10000 || st.Value.Minor() > 100000) && !ve.has(f("value")) {
		ve.add(f("value"), "out_of_range", "a percentage must be between -100 and 1000")
	}
	if st.Value.IsZero() && !ve.has(f("value")) && in.Value != nil {
		ve.add(f("value"), "zero", "value must not be zero")
	}
	if in.Priority != nil {
		st.Priority = checkPriority(ve, f("priority"), in.Priority)
	}
	if in.ChangeNote != nil {
		st.Note = checkText(ve, f("change_note"), *in.ChangeNote, 2000)
	}
}

func dedupe(ids []uuid.UUID) []uuid.UUID {
	out := []uuid.UUID{}
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func writeRuleTargets(ctx context.Context, q *db.Queries, orgID, versionID int64, st ruleState) error {
	if err := q.DeleteDateRuleTargets(ctx, db.DeleteDateRuleTargetsParams{OrganizationID: orgID, DateRuleVersionID: versionID}); err != nil {
		return repository.MapErr(err)
	}
	insert := func(p db.InsertDateRuleTargetParams) error {
		p.OrganizationID, p.DateRuleVersionID = orgID, versionID
		return repository.MapErr(q.InsertDateRuleTarget(ctx, p))
	}
	for _, p := range st.Products {
		if err := insert(db.InsertDateRuleTargetParams{RentalProductID: int8Arg(p.ID, true)}); err != nil {
			return err
		}
	}
	for _, sg := range st.Segments {
		if err := insert(db.InsertDateRuleTargetParams{SegmentID: int8Arg(sg.ID, true)}); err != nil {
			return err
		}
	}
	for _, b := range st.Branches {
		if err := insert(db.InsertDateRuleTargetParams{BranchID: int8Arg(b.ID, true)}); err != nil {
			return err
		}
	}
	return nil
}

func checkRuleName(ve *ValidationError, raw *string, create bool) string {
	if raw == nil {
		if create {
			ve.add("name", "required", "name is required")
		}
		return ""
	}
	name := strings.TrimSpace(*raw)
	switch {
	case name == "":
		ve.add("name", "required", "name is required")
	case len([]rune(name)) > 64:
		ve.add("name", "too_long", "name must be at most 64 characters")
	}
	return name
}

// CreateRule stores a new rule as draft version 1.
func (s *Service) CreateRule(ctx context.Context, orgID int64, in model.DateRuleInput) (model.DateRule, error) {
	q := s.repo.Q()
	ve := &ValidationError{}
	name := checkRuleName(ve, in.Name, true)
	st := defaultRuleState()
	s.applyRuleFields(ctx, q, orgID, ve, "", &st, in.DateRuleFields, true)
	if err := ve.orNil(); err != nil {
		return model.DateRule{}, err
	}
	if taken, err := q.DateRuleNameTaken(ctx, db.DateRuleNameTakenParams{OrganizationID: orgID, Name: name}); err != nil {
		return model.DateRule{}, err
	} else if taken {
		return model.DateRule{}, ErrNameTaken
	}
	var id uuid.UUID
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		tq := tx.Q()
		head, err := tq.CreateDateRule(ctx, db.CreateDateRuleParams{OrganizationID: orgID, Name: name})
		if err != nil {
			return repository.MapErr(err)
		}
		v, err := tq.CreateDateRuleVersion(ctx, versionParams(orgID, head.ID, 1, st, actorArg(ctx)))
		if err != nil {
			return repository.MapErr(err)
		}
		id = head.Uuid
		return writeRuleTargets(ctx, tq, orgID, v.ID, st)
	})
	if err != nil {
		return model.DateRule{}, mapRuleErr(err)
	}
	return s.GetRule(ctx, orgID, id)
}

func versionParams(orgID, ruleID int64, version int32, st ruleState, actor pgtype.Int8) db.CreateDateRuleVersionParams {
	return db.CreateDateRuleVersionParams{
		OrganizationID: orgID, DateRuleID: ruleID, Version: version,
		StartDate: pgtype.Date{Time: st.Start.Time(), Valid: true}, EndDate: pgtype.Date{Time: st.End.Time(), Valid: true},
		ProductScope: st.ProductScope, AllBranches: st.AllBranches, Channels: st.Channels, Kind: st.Kind,
		Value: st.Value.Numeric(), Priority: int32(st.Priority), ChangeNote: st.Note, CreatedBy: actor,
	}
}

func mapRuleErr(err error) error {
	if errors.Is(err, repository.ErrNameTaken) {
		return ErrNameTaken
	}
	return mapNotFound(err, ErrRuleNotFound)
}

// ensureRuleDraft returns the draft version of a locked rule, creating
// version N+1 from the active version when there is none.
func ensureRuleDraft(ctx context.Context, q *db.Queries, orgID int64, row ruleRow) (db.GetDateRuleVersionByStatusRow, error) {
	get := func(status string) (db.GetDateRuleVersionByStatusRow, error) {
		return q.GetDateRuleVersionByStatus(ctx, db.GetDateRuleVersionByStatusParams{OrganizationID: orgID, DateRuleID: row.ID, Status: status})
	}
	draft, err := get(model.VersionDraft)
	if err == nil {
		return draft, nil
	}
	if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
		return draft, err
	}
	active, err := get(model.VersionActive)
	if err != nil {
		return draft, repository.MapErr(err)
	}
	next := row.CurrentVersion + 1
	v, err := q.CopyDateRuleVersion(ctx, db.CopyDateRuleVersionParams{
		OrganizationID: orgID, FromVersionID: active.ID, Version: next, CreatedBy: actorArg(ctx),
	})
	if err != nil {
		return draft, repository.MapErr(err)
	}
	if err := q.CopyDateRuleTargets(ctx, db.CopyDateRuleTargetsParams{OrganizationID: orgID, FromVersionID: active.ID, ToVersionID: v.ID}); err != nil {
		return draft, repository.MapErr(err)
	}
	if err := repository.Rows(q.SetDateRuleState(ctx, db.SetDateRuleStateParams{
		OrganizationID: orgID, ID: row.ID, Status: row.Status, CurrentVersion: next,
		ActiveVersion: row.ActiveVersion, ActivatedAt: row.ActivatedAt,
	})); err != nil {
		return draft, err
	}
	d, err := get(model.VersionDraft)
	return d, repository.MapErr(err)
}

// PatchRule renames a rule and / or edits its draft.
func (s *Service) PatchRule(ctx context.Context, orgID int64, id uuid.UUID, in model.DateRuleInput) (model.DateRule, error) {
	ve := &ValidationError{}
	name := checkRuleName(ve, in.Name, false)
	if err := ve.orNil(); err != nil {
		return model.DateRule{}, err
	}
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		row, err := s.lockRule(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		if in.Name != nil && name != row.Name {
			taken, err := q.DateRuleNameTaken(ctx, db.DateRuleNameTakenParams{OrganizationID: orgID, Name: name, ExcludeID: int8Arg(row.ID, true)})
			if err != nil {
				return err
			}
			if taken {
				return ErrNameTaken
			}
			if err := repository.Rows(q.UpdateDateRuleName(ctx, db.UpdateDateRuleNameParams{OrganizationID: orgID, ID: row.ID, Name: name})); err != nil {
				return err
			}
		}
		if (in.DateRuleFields == model.DateRuleFields{}) {
			return q.TouchDateRule(ctx, db.TouchDateRuleParams{OrganizationID: orgID, ID: row.ID})
		}
		draft, err := ensureRuleDraft(ctx, q, orgID, row)
		if err != nil {
			return err
		}
		targets, err := ruleTargets(ctx, q, orgID, []int64{draft.ID})
		if err != nil {
			return err
		}
		st := stateFromVersion(draft, targets[draft.ID])
		fve := &ValidationError{}
		s.applyRuleFields(ctx, q, orgID, fve, "", &st, in.DateRuleFields, false)
		if err := fve.orNil(); err != nil {
			return err
		}
		if err := repository.Rows(q.UpdateDateRuleDraft(ctx, db.UpdateDateRuleDraftParams{
			OrganizationID: orgID, ID: draft.ID,
			StartDate: pgtype.Date{Time: st.Start.Time(), Valid: true}, EndDate: pgtype.Date{Time: st.End.Time(), Valid: true},
			ProductScope: st.ProductScope, AllBranches: st.AllBranches, Channels: st.Channels, Kind: st.Kind,
			Value: st.Value.Numeric(), Priority: int32(st.Priority), ChangeNote: st.Note,
		})); err != nil {
			return err
		}
		if err := writeRuleTargets(ctx, q, orgID, draft.ID, st); err != nil {
			return err
		}
		return q.TouchDateRule(ctx, db.TouchDateRuleParams{OrganizationID: orgID, ID: row.ID})
	})
	if err != nil {
		return model.DateRule{}, mapRuleErr(err)
	}
	return s.GetRule(ctx, orgID, id)
}

// DiscardRuleDraft deletes the draft of a rule that has an active version.
func (s *Service) DiscardRuleDraft(ctx context.Context, orgID int64, id uuid.UUID) (model.DateRule, error) {
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		row, err := s.lockRule(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		if !row.ActiveVersion.Valid {
			return stateErr("never_activated", "the rule was never activated; delete it instead")
		}
		if err := repository.Rows(q.DeleteDateRuleDraft(ctx, db.DeleteDateRuleDraftParams{OrganizationID: orgID, DateRuleID: row.ID})); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return stateErr("no_draft", "the rule has no draft")
			}
			return err
		}
		return repository.Rows(q.SetDateRuleState(ctx, db.SetDateRuleStateParams{
			OrganizationID: orgID, ID: row.ID, Status: row.Status, CurrentVersion: row.ActiveVersion.Int32,
			ActiveVersion: row.ActiveVersion, ActivatedAt: row.ActivatedAt,
		}))
	})
	if err != nil {
		return model.DateRule{}, mapRuleErr(err)
	}
	return s.GetRule(ctx, orgID, id)
}

// DeleteRule soft-deletes a rule.
func (s *Service) DeleteRule(ctx context.Context, orgID int64, id uuid.UUID) error {
	row, err := s.getRule(ctx, s.repo.Q(), orgID, id)
	if err != nil {
		return err
	}
	return mapRuleErr(repository.Rows(s.repo.Q().SoftDeleteDateRule(ctx, db.SoftDeleteDateRuleParams{OrganizationID: orgID, ID: row.ID})))
}

// engineRule converts a version + targets to an engine rule.
func engineRule(ruleUUID uuid.UUID, name string, versionUUID uuid.UUID, version int32, st ruleState) pricing.DateRule {
	r := pricing.DateRule{
		ID: ruleUUID.String(), VersionID: versionUUID.String(), Version: int(version), Name: name,
		Range:        rentaltime.DateRange{Start: st.Start, End: st.End},
		ProductScope: st.ProductScope, Kind: st.Kind, Priority: st.Priority,
	}
	for _, p := range st.Products {
		r.ProductIDs = append(r.ProductIDs, p.Uuid.String())
	}
	for _, sg := range st.Segments {
		r.SegmentIDs = append(r.SegmentIDs, sg.Uuid.String())
	}
	if !st.AllBranches {
		r.BranchIDs = []string{}
		for _, b := range st.Branches {
			r.BranchIDs = append(r.BranchIDs, b.Uuid.String())
		}
	}
	if st.Channels != nil {
		r.Channels = append([]string{}, st.Channels...)
	}
	if st.Kind == pricing.KindPercent {
		r.Percent = money.Rate(st.Value.Minor())
	} else {
		r.Amount = st.Value
	}
	return r
}

// activeEngineRules loads the active rule versions overlapping
// [from, to) (nil = unbounded) as engine rules.
func activeEngineRules(ctx context.Context, q *db.Queries, orgID int64, from, to *rentaltime.Date) ([]pricing.DateRule, error) {
	rows, err := q.ListActiveDateRuleVersions(ctx, db.ListActiveDateRuleVersionsParams{
		OrganizationID: orgID, DateFrom: dateArg(from), DateTo: dateArg(to),
	})
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	targets, err := ruleTargets(ctx, q, orgID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]pricing.DateRule, 0, len(rows))
	for _, r := range rows {
		st := stateFromVersion(db.GetDateRuleVersionByStatusRow{
			StartDate: r.StartDate, EndDate: r.EndDate, ProductScope: r.ProductScope, AllBranches: r.AllBranches,
			Channels: r.Channels, Kind: r.Kind, Value: r.Value, Priority: r.Priority,
		}, targets[r.ID])
		out = append(out, engineRule(r.RuleUuid, r.Name, r.Uuid, r.Version, st))
	}
	return out, nil
}

// productSegments maps product uuids to segment uuids (overlap checks).
func productSegments(ctx context.Context, q *db.Queries, orgID int64) (map[string]string, error) {
	rows, err := q.ListPricingProducts(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Uuid.String()] = r.SegmentUuid.String()
	}
	return out, nil
}

// ruleIssues validates the version activation would put live.
func (s *Service) ruleIssues(ctx context.Context, q *db.Queries, orgID int64, row ruleRow) ([]model.Issue, error) {
	_, draft, err := ruleVersion(ctx, q, orgID, row.ID, model.VersionDraft)
	if err != nil {
		return nil, err
	}
	version := draft
	if version == nil {
		if row.Status != model.StatusInactive {
			return []model.Issue{{Field: "draft", Code: "no_draft", Message: "there is no draft to activate"}}, nil
		}
		if _, version, err = ruleVersion(ctx, q, orgID, row.ID, model.VersionActive); err != nil || version == nil {
			return []model.Issue{{Field: "draft", Code: "no_draft", Message: "there is no version to activate"}}, err
		}
	}
	targets, err := ruleTargets(ctx, q, orgID, []int64{version.ID})
	if err != nil {
		return nil, err
	}
	mine := engineRule(row.Uuid, row.Name, version.Uuid, version.Version, stateFromVersion(*version, targets[version.ID]))
	start, end := mine.Range.Start, mine.Range.End
	others, err := activeEngineRules(ctx, q, orgID, &start, &end)
	if err != nil {
		return nil, err
	}
	segs, err := productSegments(ctx, q, orgID)
	if err != nil {
		return nil, err
	}
	issues := []model.Issue{}
	for _, o := range others {
		if o.ID == mine.ID || o.Priority != mine.Priority || !pricing.RulesOverlap(mine, o, segs) {
			continue
		}
		ref, _ := uuid.Parse(o.ID)
		issues = append(issues, model.Issue{
			Field: "priority", Code: "priority_conflict",
			Message: fmt.Sprintf("active rule %q overlaps with the same priority %d", o.Name, o.Priority),
			Params:  map[string]int{"priority": o.Priority},
			Ref:     &model.IssueRef{UUID: ref, Name: o.Name},
		})
	}
	return issues, nil
}

// RuleActivationCheck reports what activation would refuse.
func (s *Service) RuleActivationCheck(ctx context.Context, orgID int64, id uuid.UUID) (model.ActivationCheck, error) {
	q := s.repo.Q()
	row, err := s.getRule(ctx, q, orgID, id)
	if err != nil {
		return model.ActivationCheck{}, err
	}
	issues, err := s.ruleIssues(ctx, q, orgID, row)
	if err != nil {
		return model.ActivationCheck{}, err
	}
	return model.ActivationCheck{Ready: len(issues) == 0, Issues: issues}, nil
}

// ActivateRule validates and freezes the draft (or re-enables an inactive
// rule); an equal-priority overlap with an active rule refuses it.
func (s *Service) ActivateRule(ctx context.Context, orgID int64, id uuid.UUID, in model.ActivateInput) (model.DateRule, error) {
	ve := &ValidationError{}
	var note *string
	if in.ChangeNote != nil {
		n := checkText(ve, "change_note", *in.ChangeNote, 2000)
		note = &n
	}
	if err := ve.orNil(); err != nil {
		return model.DateRule{}, err
	}
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		row, err := s.lockRule(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		issues, err := s.ruleIssues(ctx, q, orgID, row)
		if err != nil {
			return err
		}
		if len(issues) > 0 {
			return &ActivationError{Issues: issues}
		}
		now := s.clock()
		draft, err := q.GetDateRuleVersionByStatus(ctx, db.GetDateRuleVersionByStatusParams{OrganizationID: orgID, DateRuleID: row.ID, Status: model.VersionDraft})
		if err != nil {
			if !errors.Is(repository.MapErr(err), repository.ErrNotFound) {
				return err
			}
			return repository.Rows(q.SetDateRuleState(ctx, db.SetDateRuleStateParams{
				OrganizationID: orgID, ID: row.ID, Status: model.StatusActive, CurrentVersion: row.CurrentVersion,
				ActiveVersion: row.ActiveVersion, ActivatedAt: ts(now),
			}))
		}
		if note != nil {
			if err := repository.Rows(q.UpdateDateRuleDraft(ctx, db.UpdateDateRuleDraftParams{
				OrganizationID: orgID, ID: draft.ID, StartDate: draft.StartDate, EndDate: draft.EndDate,
				ProductScope: draft.ProductScope, AllBranches: draft.AllBranches, Channels: draft.Channels,
				Kind: draft.Kind, Value: draft.Value, Priority: draft.Priority, ChangeNote: *note,
			})); err != nil {
				return err
			}
		}
		if _, err := q.SupersedeDateRuleVersion(ctx, db.SupersedeDateRuleVersionParams{OrganizationID: orgID, DateRuleID: row.ID, SupersededAt: ts(now)}); err != nil {
			return repository.MapErr(err)
		}
		if err := repository.Rows(q.ActivateDateRuleVersion(ctx, db.ActivateDateRuleVersionParams{
			OrganizationID: orgID, ID: draft.ID, ActivatedAt: ts(now), ActivatedBy: actorArg(ctx),
		})); err != nil {
			return err
		}
		return repository.Rows(q.SetDateRuleState(ctx, db.SetDateRuleStateParams{
			OrganizationID: orgID, ID: row.ID, Status: model.StatusActive, CurrentVersion: draft.Version,
			ActiveVersion: pgtype.Int4{Int32: draft.Version, Valid: true}, ActivatedAt: ts(now),
		}))
	})
	if err != nil {
		return model.DateRule{}, mapRuleErr(err)
	}
	return s.GetRule(ctx, orgID, id)
}

// DeactivateRule switches an active rule off.
func (s *Service) DeactivateRule(ctx context.Context, orgID int64, id uuid.UUID) (model.DateRule, error) {
	err := s.repo.InTx(ctx, func(tx *repository.Postgres) error {
		q := tx.Q()
		row, err := s.lockRule(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		if row.Status != model.StatusActive {
			return stateErr("not_active", "only an active rule can be deactivated")
		}
		return repository.Rows(q.SetDateRuleState(ctx, db.SetDateRuleStateParams{
			OrganizationID: orgID, ID: row.ID, Status: model.StatusInactive, CurrentVersion: row.CurrentVersion,
			ActiveVersion: row.ActiveVersion, ActivatedAt: row.ActivatedAt,
		}))
	})
	if err != nil {
		return model.DateRule{}, mapRuleErr(err)
	}
	return s.GetRule(ctx, orgID, id)
}

// RuleExportRow is one row of the rule export.
type RuleExportRow = db.ListDateRulesForExportRow

// RuleExportRows returns the rules matching a filter (export), capped.
func (s *Service) RuleExportRows(ctx context.Context, orgID int64, f model.DateRuleFilter) ([]RuleExportRow, error) {
	return s.repo.Q().ListDateRulesForExport(ctx, db.ListDateRulesForExportParams{
		OrganizationID: orgID, Statuses: strs(f.Statuses), Kinds: strs(f.Kinds), DateFrom: dateArg(f.DateFrom),
		DateTo: dateArg(f.DateTo), Q: textArg(f.Q), LimitCount: MaxListTargets,
	})
}
