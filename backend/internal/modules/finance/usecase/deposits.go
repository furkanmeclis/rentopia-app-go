package usecase

// Security deposits and pre-authorizations (TEC-719,
// docs/design/deposits.md). A deposit is a liability, never revenue:
//
//   - cash / bank transfer: deposit_hold (credit, purpose deposit) on the
//     customer account plus a deposit_in movement into the till / bank.
//     Purpose deposit is outside the receivable balance, so the customer
//     still owes the rent and the charges.
//   - POS / online pre-authorization: nothing is posted while held (no
//     money moved).
//   - resolution: each deduction is captured as a collection credit
//     applied to its charge (purpose charge / rent / general); a cash
//     deposit's capture also posts deposit_capture (debit, purpose
//     deposit) so the liability shrinks by the same amount. The remainder
//     goes back: deposit_return + deposit_out for money, a release (iyzico
//     cancel, or the captured post-auth) for a pre-authorization.
//
// One resolution per deposit (row lock, status held → closed, idempotency
// key); the partial unique indexes on ledger_entries make a second hold,
// return or capture leg impossible even outside this code.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrDepositNotFound   = errors.New("security deposit not found")
	ErrDeductionNotFound = errors.New("deposit deduction not found")
	ErrNoDocument        = errors.New("the deduction has no document")
)

// ProviderError: the payment provider refused or failed an online
// pre-authorization step (502); nothing was recorded as done.
type ProviderError struct{ Err error }

func (e *ProviderError) Error() string { return "payment provider: " + e.Err.Error() }

func (e *ProviderError) Unwrap() error { return e.Err }

// resourceOperationTasks is the rentals task list resource (live events).
const resourceOperationTasks = "tenant.operation_tasks"

// DepositSortColumns are the sortable deposit columns.
var DepositSortColumns = map[string]string{"created_at": "created_at", "held_at": "held_at", "amount": "amount"}

// PreAuthAccounts is the integration accounts port (online pre-auth).
type PreAuthAccounts interface {
	ActiveAccount(ctx context.Context, orgID int64, kind string) (integrations.Account, error)
	AccountByID(ctx context.Context, orgID, id int64) (integrations.Account, error)
}

type preAuthProvider interface {
	payments.Provider
	payments.PreAuthorizer
}

type preAuthDeps struct {
	accounts  PreAuthAccounts
	providers map[string]preAuthProvider
	baseURL   string
}

// SetPreAuth wires online pre-authorizations: the organization's active
// payment account, the providers that implement payments.PreAuthorizer
// (others are ignored) and the browser origin of the provider callback.
func (s *Service) SetPreAuth(accounts PreAuthAccounts, publicBaseURL string, providers ...payments.Provider) {
	d := &preAuthDeps{accounts: accounts, providers: map[string]preAuthProvider{}, baseURL: strings.TrimRight(publicBaseURL, "/")}
	for _, p := range providers {
		if pp, ok := p.(preAuthProvider); ok {
			d.providers[p.Provider()] = pp
		}
	}
	s.preauth = d
}

// onlineAccount returns the active payment account when its provider can
// pre-authorize (ok false: the manual path only).
func (s *Service) onlineAccount(ctx context.Context, orgID int64) (integrations.Account, preAuthProvider, bool, error) {
	if s.preauth == nil || s.preauth.accounts == nil {
		return integrations.Account{}, nil, false, nil
	}
	acc, err := s.preauth.accounts.ActiveAccount(ctx, orgID, integrations.KindPayment)
	if errors.Is(err, accountsusecase.ErrNoActiveAccount) {
		return acc, nil, false, nil
	}
	if err != nil {
		return acc, nil, false, err
	}
	p, ok := s.preauth.providers[acc.Provider]
	return acc, p, ok, nil
}

// depositAccount is the account a deposit was opened on (online steps).
func (s *Service) depositAccount(ctx context.Context, orgID int64, d db.GetSecurityDepositRowRow) (integrations.Account, preAuthProvider, error) {
	if s.preauth == nil || s.preauth.accounts == nil || !d.IntegrationAccountID.Valid {
		return integrations.Account{}, nil, stateErr("online_unavailable", "online pre-authorization is not configured")
	}
	acc, err := s.preauth.accounts.AccountByID(ctx, orgID, d.IntegrationAccountID.Int64)
	if err != nil {
		return acc, nil, err
	}
	p, ok := s.preauth.providers[acc.Provider]
	if !ok {
		return acc, nil, stateErr("online_unavailable", "the provider cannot pre-authorize")
	}
	return acc, p, nil
}

// ------------------------------------------------------------- reading

// depositVisible applies the branch scope (the branch it was taken at).
func depositVisible(ctx context.Context, branchID int64) bool {
	return orgctx.CheckBranch(ctx, branchID) == nil
}

func (s *Service) depositRow(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetSecurityDepositRowRow, error) {
	if q == nil {
		q = s.q
	}
	d, err := q.GetSecurityDepositRow(ctx, db.GetSecurityDepositRowParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) || (err == nil && !depositVisible(ctx, d.BranchID)) {
		return d, ErrDepositNotFound
	}
	return d, err
}

func heldBefore(now time.Time, days int) time.Time {
	return now.Add(-time.Duration(days) * 24 * time.Hour)
}

// ListDeposits lists deposits in the caller's branch scope.
func (s *Service) ListDeposits(ctx context.Context, orgID int64, f model.DepositFilter) ([]model.Deposit, int64, error) {
	ve := &ValidationError{}
	for _, st := range f.Statuses {
		if !contains(model.DepositStatuses, st) {
			ve.add("status", "invalid", "unknown status "+st)
		}
	}
	for _, mt := range f.Methods {
		if !contains(model.DepositMethods, mt) {
			ve.add("method", "invalid", "unknown method "+mt)
		}
	}
	if f.HeldOverDays != nil && (*f.HeldOverDays < 0 || *f.HeldOverDays > 3650) {
		ve.add("held_over_days", "invalid", "held_over_days must be between 0 and 3650")
	}
	if err := ve.orNil(); err != nil {
		return nil, 0, err
	}
	filterBranches, err := s.branchIDs(ctx, orgID, "branch", f.BranchUUIDs)
	if err != nil {
		return nil, 0, err
	}
	sortField := f.SortField
	if _, ok := DepositSortColumns[sortField]; !ok {
		sortField, f.SortDesc = "created_at", true
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 25
	}
	p := db.ListSecurityDepositsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), Ids: nilIDs(f.IDs), Statuses: nilIfEmpty(f.Statuses),
		Methods: nilIfEmpty(f.Methods), FilterBranchIds: nilIDs(filterBranches), Q: textOf(f.Q), SortField: sortField,
		SortDesc: f.SortDesc, LimitCount: limit, OffsetCount: f.Offset,
	}
	if f.RentalUUID != nil {
		r, err := s.q.GetRentalRef(ctx, db.GetRentalRefParams{OrganizationID: orgID, Uuid: *f.RentalUUID})
		if notFound(err) {
			return nil, 0, validation("rental", "not_found", "unknown rental")
		}
		if err != nil {
			return nil, 0, err
		}
		p.RentalID = int8Of(r.ID)
	}
	if f.From != nil {
		p.DateFrom = istanbulMidnight(*f.From)
	}
	if f.To != nil {
		p.DateTo = istanbulMidnight(day0(*f.To).AddDate(0, 0, 1))
	}
	if f.HeldOverDays != nil {
		p.HeldBefore = pgtype.Timestamptz{Time: heldBefore(s.clock(), *f.HeldOverDays), Valid: true}
	}
	rows, err := s.q.ListSecurityDeposits(ctx, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]model.Deposit, 0, len(rows))
	var total int64
	for _, r := range rows {
		total = r.Total
		out = append(out, s.depositOf(ctx, r))
	}
	return out, total, nil
}

// istanbulMidnight is the instant a calendar day starts in Istanbul.
func istanbulMidnight(d time.Time) pgtype.Timestamptz {
	loc := rentaltime.Location()
	return pgtype.Timestamptz{Time: time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc).UTC(), Valid: true}
}

func (s *Service) depositOf(ctx context.Context, r db.ListSecurityDepositsRow) model.Deposit {
	amount, captured, returned := m(r.Amount), m(r.CapturedAmount), m(r.ReturnedAmount)
	d := model.Deposit{
		UUID:         r.Uuid,
		Rental:       model.Ref{UUID: r.RentalUuid, Code: r.RentalReference},
		RentalStatus: r.RentalStatus,
		Reservation:  model.Ref{UUID: r.ReservationUuid, Code: r.ReservationReference},
		Customer: model.Ref{UUID: r.CustomerUuid, Name: customerName(r.CustomerType, r.FirstName, r.LastName, r.CompanyTitle,
			r.AnonymizedAt.Valid)},
		Account:              refOf(r.AccountUuid, r.AccountCode, pgtype.Text{}),
		Branch:               model.Ref{UUID: r.BranchUuid, Code: r.BranchCode, Name: r.BranchName},
		Amount:               amount,
		Currency:             r.Currency,
		Method:               r.Method,
		Status:               r.Status,
		CashAccount:          refOf(r.CashAccountUuid, r.CashAccountCode, r.CashAccountName),
		CashAccountKind:      r.CashAccountKind.String,
		POSReference:         r.PosReference,
		Provider:             r.Provider,
		ProviderPaymentID:    r.ProviderPaymentID,
		PaymentPageURL:       r.PaymentPageUrl,
		AuthExpiresAt:        timePtr(r.AuthorizationExpiresAt),
		CapturedAmount:       captured,
		ReturnedAmount:       returned,
		OpenAmount:           money.Zero(),
		Note:                 r.Note,
		HeldAt:               timePtr(r.HeldAt),
		ResolvedAt:           timePtr(r.ResolvedAt),
		ResolutionNote:       r.ResolutionNote,
		CancelReason:         r.CancelReason,
		CustomerNoticeStatus: r.CustomerNoticeStatus,
		CustomerNotice:       map[string]any{},
		DeductionCount:       r.DeductionCount,
		Deductions:           []model.Deduction{},
		CreatedBy:            actorOf(r.CreatedByUuid, r.CreatedByName, r.CreatedBySurname, r.CreatedByEmail),
		ResolvedBy:           actorOf(r.ResolvedByUuid, r.ResolvedByName, r.ResolvedBySurname, r.ResolvedByEmail),
		CreatedAt:            r.CreatedAt.Time.UTC(),
		UpdatedAt:            r.UpdatedAt.Time.UTC(),
	}
	_ = json.Unmarshal(r.CustomerNotice, &d.CustomerNotice)
	open := r.Status == model.DepositHeld || r.Status == model.DepositPending
	if open {
		d.OpenAmount = amount.Sub(captured).Sub(returned)
	}
	if r.Status == model.DepositHeld && r.HeldAt.Valid {
		d.HeldDays = int(s.clock().Sub(r.HeldAt.Time) / (24 * time.Hour))
	}
	write, deduct := has(ctx, rbac.PermTenantFinanceDepositsWrite), has(ctx, rbac.PermTenantFinanceDepositsDeduct)
	d.Capabilities = model.DepositCapabilities{
		Verify:        write && r.Status == model.DepositPending && r.Method == model.DepositOnlinePreAuth,
		Release:       write && r.Status == model.DepositHeld,
		Deduct:        write && deduct && r.Status == model.DepositHeld,
		Cancel:        write && r.Status == model.DepositPending,
		OnlineCapture: r.Method == model.DepositOnlinePreAuth,
	}
	return d
}

// GetDeposit returns one deposit with its deductions.
func (s *Service) GetDeposit(ctx context.Context, orgID int64, id uuid.UUID) (model.Deposit, error) {
	row, err := s.depositRow(ctx, nil, orgID, id)
	if err != nil {
		return model.Deposit{}, err
	}
	rows, err := s.q.ListSecurityDeposits(ctx, db.ListSecurityDepositsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx), DepositUuid: uuidOf(id), SortField: "created_at",
		SortDesc: true, LimitCount: 1,
	})
	if err != nil {
		return model.Deposit{}, err
	}
	if len(rows) == 0 {
		return model.Deposit{}, ErrDepositNotFound
	}
	d := s.depositOf(ctx, rows[0])
	items, err := s.q.ListDepositDeductions(ctx, db.ListDepositDeductionsParams{OrganizationID: orgID, DepositID: row.ID})
	if err != nil {
		return model.Deposit{}, err
	}
	for _, x := range items {
		item := model.Deduction{
			UUID: x.Uuid, Amount: m(x.Amount), AppliesTo: x.AppliesTo, Reason: x.Reason,
			CreatedBy: actorOf(x.CreatedByUuid, x.CreatedByName, x.CreatedBySurname, x.CreatedByEmail), CreatedAt: x.CreatedAt.Time.UTC(),
		}
		if x.ChargeUuid.Valid {
			item.Charge = &model.Ref{UUID: x.ChargeUuid.Bytes, Code: x.ChargeKind.String, Name: m(x.ChargeAmount).String()}
		}
		if x.DamageUuid.Valid {
			item.Damage = &model.Ref{UUID: x.DamageUuid.Bytes, Code: x.DamageZone.String, Name: x.DamageType.String}
		}
		if x.HasDocument {
			item.Document = &model.Attachment{
				Name: x.DocumentName, ContentType: x.DocumentContentType, SizeBytes: x.DocumentSizeBytes,
				URL: "/v1/tenant/finance/deposits/" + id.String() + "/deductions/" + x.Uuid.String() + "/file",
			}
		}
		d.Deductions = append(d.Deductions, item)
	}
	return d, nil
}

// RentalDeposits is the deposit card of a rental.
type RentalDeposits struct {
	RentalUUID    uuid.UUID       `json:"rental_uuid"`
	Reference     string          `json:"reference"`
	RentalStatus  string          `json:"rental_status"`
	DefaultAmount money.Money     `json:"default_amount"`
	Currency      string          `json:"currency"`
	CanTake       bool            `json:"can_take"`
	OnlineEnabled bool            `json:"online_enabled"`
	Open          *uuid.UUID      `json:"open_deposit_uuid"`
	Items         []model.Deposit `json:"items"`
	// DepositTask is the live deposit operation task of the rental (the
	// return opened it; resolving the deposit completes it).
	DepositTask *uuid.UUID `json:"deposit_task_uuid"`
	// CashAccounts are the active tills / bank accounts of the caller's
	// scope a deposit can go into or be paid back from (counter staff need
	// no cash book access to pick one).
	CashAccounts []DepositCashAccount `json:"cash_accounts"`
}

// DepositCashAccount is a till / bank account option of the deposit card.
type DepositCashAccount struct {
	UUID     uuid.UUID  `json:"uuid"`
	Code     string     `json:"code"`
	Name     string     `json:"name"`
	Kind     string     `json:"kind"`
	Currency string     `json:"currency"`
	Branch   *model.Ref `json:"branch"`
}

// productDeposit reads the deposit amount of the reservation's product
// snapshot (rental_products.deposit_amount at the time of the sale).
func productDeposit(snapshot []byte) money.Money {
	var v struct {
		DepositAmount json.RawMessage `json:"deposit_amount"`
	}
	if json.Unmarshal(snapshot, &v) != nil || len(v.DepositAmount) == 0 {
		return money.Zero()
	}
	raw := strings.Trim(string(v.DepositAmount), `"`)
	amount, err := money.ParseRound(strings.TrimSpace(raw))
	if err != nil || amount.IsNegative() {
		return money.Zero()
	}
	return amount
}

func rentalVisible(ctx context.Context, r db.GetRentalForDepositRow) bool {
	return orgctx.CheckBranch(ctx, r.PickupBranchID) == nil || orgctx.CheckBranch(ctx, r.ReturnBranchID) == nil ||
		(r.ActualPickupBranchID.Valid && orgctx.CheckBranch(ctx, r.ActualPickupBranchID.Int64) == nil) ||
		(r.ActualReturnBranchID.Valid && orgctx.CheckBranch(ctx, r.ActualReturnBranchID.Int64) == nil)
}

func (s *Service) rentalForDeposit(ctx context.Context, q *db.Queries, orgID int64, id uuid.UUID) (db.GetRentalForDepositRow, error) {
	if q == nil {
		q = s.q
	}
	r, err := q.GetRentalForDeposit(ctx, db.GetRentalForDepositParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) || (err == nil && !rentalVisible(ctx, r)) {
		return r, ErrRentalNotFound
	}
	return r, err
}

// rentalTakesDeposit: a deposit is taken from the preparation (at the
// handover) until the return check; never on a closed rental.
var rentalTakesDeposit = map[string]bool{"preparation": true, "active": true, "return_check": true}

// DepositsForRental is the rental page's deposit card.
func (s *Service) DepositsForRental(ctx context.Context, orgID int64, rentalID uuid.UUID) (RentalDeposits, error) {
	r, err := s.rentalForDeposit(ctx, nil, orgID, rentalID)
	if err != nil {
		return RentalDeposits{}, err
	}
	items, _, err := s.ListDeposits(ctx, orgID, model.DepositFilter{RentalUUID: &rentalID, Limit: 50, SortDesc: true})
	if err != nil {
		return RentalDeposits{}, err
	}
	out := RentalDeposits{
		RentalUUID: r.Uuid, Reference: r.Reference, RentalStatus: r.Status, DefaultAmount: productDeposit(r.ProductSnapshot),
		Currency: r.Currency, Items: items,
	}
	for _, d := range items {
		if d.Status == model.DepositHeld || d.Status == model.DepositPending {
			id := d.UUID
			out.Open = &id
		}
	}
	out.CanTake = out.Open == nil && rentalTakesDeposit[r.Status] && has(ctx, rbac.PermTenantFinanceDepositsWrite)
	if _, _, ok, err := s.onlineAccount(ctx, orgID); err == nil && ok {
		out.OnlineEnabled = true
	}
	out.CashAccounts = []DepositCashAccount{}
	if has(ctx, rbac.PermTenantFinanceDepositsWrite) {
		cash, _, err := s.ListCashAccounts(ctx, orgID, model.CashAccountFilter{
			Statuses: []string{model.AccountActive}, Limit: 100, SortField: "code",
		})
		if err != nil {
			return out, err
		}
		for _, c := range cash {
			if c.Currency != r.Currency {
				continue
			}
			out.CashAccounts = append(out.CashAccounts, DepositCashAccount{
				UUID: c.UUID, Code: c.Code, Name: c.Name, Kind: c.Kind, Currency: c.Currency, Branch: c.Branch,
			})
		}
	}
	tasks, err := s.q.ListLiveDepositTasks(ctx, db.ListLiveDepositTasksParams{OrganizationID: orgID, RentalID: int8Of(r.ID)})
	if err != nil {
		return out, err
	}
	if len(tasks) > 0 {
		id := tasks[0].Uuid
		out.DepositTask = &id
	}
	return out, nil
}

// DepositTotals are the open deposits of the caller's scope.
func (s *Service) DepositTotals(ctx context.Context, orgID int64, heldOverDays int) (model.DepositTotals, error) {
	if heldOverDays <= 0 {
		heldOverDays = model.DefaultHeldOverDays
	}
	rows, err := s.q.SecurityDepositTotals(ctx, db.SecurityDepositTotalsParams{
		OrganizationID: orgID, BranchIds: orgctx.BranchScope(ctx),
		HeldBefore: pgtype.Timestamptz{Time: heldBefore(s.clock(), heldOverDays), Valid: true},
	})
	if err != nil {
		return model.DepositTotals{}, err
	}
	out := model.DepositTotals{
		Currency: string(money.TRY), HeldAmount: money.Zero(), HeldOverDays: heldOverDays, ByMethod: map[string]money.Money{},
	}
	for _, mt := range model.DepositMethods {
		out.ByMethod[mt] = money.Zero()
	}
	for _, r := range rows {
		if r.Status == model.DepositPending {
			out.PendingCount += r.DepositCount
			continue
		}
		out.HeldCount += r.DepositCount
		out.HeldAmount = out.HeldAmount.Add(m(r.OpenAmount))
		out.ByMethod[r.Method] = out.ByMethod[r.Method].Add(m(r.OpenAmount))
		out.HeldOver += r.HeldOver
	}
	return out, nil
}

// ---------------------------------------------------------------- taking

// cashHolder loads and locks the till / bank account a deposit's money
// goes into or comes out of (caller scope, active, same currency).
func (s *Service) cashHolder(ctx context.Context, q *db.Queries, orgID int64, field string, id uuid.UUID, currency, wantKind string) (lockedAccount, error) {
	locked, err := s.lockForMovement(ctx, q, orgID, cashTarget{field, id})
	if err != nil {
		return lockedAccount{}, err
	}
	a := locked[0]
	if a.row.Currency != currency {
		return a, validation(field, "currency_mismatch", "the cash account holds another currency")
	}
	if wantKind != "" && a.row.Kind != wantKind {
		if wantKind == model.CashKindCash {
			return a, validation(field, "kind", "a cash deposit goes into a till (cash account)")
		}
		return a, validation(field, "kind", "a bank transfer deposit goes into a bank account")
	}
	return a, nil
}

func (s *Service) depositMovement(ctx context.Context, q *db.Queries, orgID int64, a lockedAccount, kind string, amount money.Money,
	currency string, entryID int64, desc string) error {
	dir := model.DirIn
	if kind == model.MoveDepositOut {
		dir = model.DirOut
		if err := checkFunds("refund_cash_account_uuid", a, amount); err != nil {
			return err
		}
	}
	row, err := q.InsertCashMovement(ctx, db.InsertCashMovementParams{
		OrganizationID: orgID, CashAccountID: a.row.ID, Kind: kind, Direction: dir, Amount: amount.Numeric(), Currency: currency,
		MovementDate: dateOf(s.today()), Description: desc, BranchID: a.row.BranchID, LedgerEntryID: int8Of(entryID),
		CreatedBy: actorID(ctx),
	})
	if err != nil {
		return err
	}
	s.publish(ctx, model.ResourceMovements, "created", row.Uuid)
	s.publish(ctx, model.ResourceCashAccounts, "updated", a.row.Uuid)
	return nil
}

// TakeDeposit records a deposit for a rental (tenant.finance.deposits.write).
// Cash / bank transfer: held at once (liability + money in the account).
// POS pre-auth: held at once (reference typed in, nothing posted). Online
// pre-auth: pending until the customer completed the provider's page and
// the provider confirmed it (VerifyDeposit).
func (s *Service) TakeDeposit(ctx context.Context, orgID int64, in model.DepositInput) (model.Deposit, error) {
	if !has(ctx, rbac.PermTenantFinanceDepositsWrite) {
		return model.Deposit{}, ErrForbidden
	}
	ve := &ValidationError{}
	if in.RentalUUID == nil {
		ve.add("rental_uuid", "required", "rental_uuid is required")
	}
	if !contains(model.DepositMethods, in.Method) {
		ve.add("method", "invalid", "method must be cash, pos_preauth, online_preauth or bank_transfer")
	}
	pos := strings.TrimSpace(in.POSReference)
	if in.Method == model.DepositPOSPreAuth && pos == "" {
		ve.add("pos_reference", "required", "the POS provision number is required")
	}
	if len([]rune(pos)) > 64 {
		ve.add("pos_reference", "too_long", "pos_reference is at most 64 characters")
	}
	if (in.Method == model.DepositCash || in.Method == model.DepositBankTransfer) && in.CashAccountUUID == nil {
		ve.add("cash_account_uuid", "required", "choose the till / bank account that receives the deposit")
	}
	if len(in.Note) > 500 {
		ve.add("note", "too_long", "note is at most 500 characters")
	}
	if in.Amount != nil && !in.Amount.IsPositive() {
		ve.add("amount", "invalid", "amount must be positive")
	}
	if err := ve.orNil(); err != nil {
		return model.Deposit{}, err
	}
	r, err := s.rentalForDeposit(ctx, nil, orgID, *in.RentalUUID)
	if err != nil {
		if errors.Is(err, ErrRentalNotFound) {
			return model.Deposit{}, validation("rental_uuid", "not_found", "unknown rental")
		}
		return model.Deposit{}, err
	}
	if !rentalTakesDeposit[r.Status] {
		return model.Deposit{}, stateErr("rental_closed", "a deposit is taken for a rental in preparation, active or in return check")
	}
	amount := productDeposit(r.ProductSnapshot)
	if in.Amount != nil {
		amount = *in.Amount
	}
	if !amount.IsPositive() {
		return model.Deposit{}, validation("amount", "required", "the product has no deposit amount; enter one")
	}
	branchDef := r.PickupBranchID
	if r.ActualPickupBranchID.Valid {
		branchDef = r.ActualPickupBranchID.Int64
	}
	if orgctx.CheckBranch(ctx, branchDef) != nil {
		// A member of the return branch takes it there.
		branchDef = r.ReturnBranchID
	}
	branch, err := s.branchFor(ctx, orgID, "branch_uuid", nil, branchDef)
	if err != nil {
		return model.Deposit{}, err
	}
	var online integrations.Account
	var provider preAuthProvider
	if in.Method == model.DepositOnlinePreAuth {
		acc, p, ok, err := s.onlineAccount(ctx, orgID)
		if err != nil {
			return model.Deposit{}, err
		}
		if !ok {
			return model.Deposit{}, validation("method", "online_unavailable",
				"online pre-authorization needs an active payment account that supports it; take the deposit on the POS terminal")
		}
		online, provider = acc, p
	}
	customerID := r.CustomerID
	if r.InvoiceCustomerID.Valid {
		customerID = r.InvoiceCustomerID.Int64
	}
	desc := r.Reference + " · depozito"
	status, heldAt := model.DepositHeld, pgtype.Timestamptz{Time: s.clock(), Valid: true}
	if in.Method == model.DepositOnlinePreAuth {
		status, heldAt = model.DepositPending, pgtype.Timestamptz{}
	}
	var depID int64
	var depUUID uuid.UUID
	var accUUID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		var cash lockedAccount
		if in.CashAccountUUID != nil && (in.Method == model.DepositCash || in.Method == model.DepositBankTransfer) {
			kind := model.CashKindCash
			if in.Method == model.DepositBankTransfer {
				kind = model.CashKindBank
			}
			if cash, err = s.cashHolder(ctx, q, orgID, "cash_account_uuid", *in.CashAccountUUID, r.Currency, kind); err != nil {
				return err
			}
		}
		acc, err := s.ensureCustomerAccount(ctx, q, orgID, customerID, branch, r.Currency)
		if err != nil {
			return err
		}
		accUUID = acc.UUID
		var expires pgtype.Timestamptz
		if in.AuthorizationExpiresAt != nil && in.Method == model.DepositPOSPreAuth {
			expires = pgtype.Timestamptz{Time: in.AuthorizationExpiresAt.UTC(), Valid: true}
		}
		row, err := q.InsertSecurityDeposit(ctx, db.InsertSecurityDepositParams{
			OrganizationID: orgID, RentalID: r.ID, ReservationID: r.ReservationID, CustomerID: customerID,
			AccountID: int8Of(acc.ID), BranchID: branch, Amount: amount.Numeric(), Currency: r.Currency, Method: in.Method,
			Status: status, CashAccountID: int8Of(cash.row.ID), PosReference: pos, Provider: online.Provider,
			IntegrationAccountID: int8Of(online.ID), AuthorizationExpiresAt: expires, Note: strings.TrimSpace(in.Note),
			HeldAt: heldAt, CreatedBy: actorID(ctx),
		})
		if uniqueViolation(err) {
			return stateErr("deposit_open", "the rental already has an open deposit")
		}
		if err != nil {
			return err
		}
		depID, depUUID = row.ID, row.Uuid
		if in.Method != model.DepositCash && in.Method != model.DepositBankTransfer {
			return nil
		}
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: acc.ID}); err != nil {
			return err
		}
		e, err := q.InsertDepositLedgerEntry(ctx, db.InsertDepositLedgerEntryParams{
			OrganizationID: orgID, AccountID: acc.ID, Kind: model.KindDepositHold, Direction: model.Credit,
			Purpose: model.PurposeDeposit, Amount: amount.Numeric(), Currency: r.Currency, EntryDate: dateOf(s.today()),
			Description: desc, SourceUuid: uuidOf(depUUID), RentalID: int8Of(r.ID), ReservationID: int8Of(r.ReservationID),
			SecurityDepositID: int8Of(depID), BranchID: int8Of(branch), CreatedBy: actorID(ctx),
		})
		if err != nil {
			return err
		}
		s.publish(ctx, model.ResourceEntries, "created", e.Uuid)
		return s.depositMovement(ctx, q, orgID, cash, model.MoveDepositIn, amount, r.Currency, e.ID, desc)
	})
	if err != nil {
		return model.Deposit{}, err
	}
	s.publish(ctx, model.ResourceDeposits, "created", depUUID)
	s.publish(ctx, model.ResourceAccounts, "updated", accUUID)
	if in.Method == model.DepositOnlinePreAuth {
		if err := s.openPreAuth(ctx, orgID, depID, depUUID, r, amount, online, provider, in.ClientIP, in.Locale); err != nil {
			return model.Deposit{}, err
		}
	}
	return s.GetDeposit(ctx, orgID, depUUID)
}

// openPreAuth opens the provider's hosted pre-auth page for a pending
// deposit. A provider failure cancels the deposit (nothing is held).
func (s *Service) openPreAuth(ctx context.Context, orgID, depID int64, depUUID uuid.UUID, r db.GetRentalForDepositRow, amount money.Money,
	acc integrations.Account, p preAuthProvider, ip, locale string) error {
	name, surname := strings.TrimSpace(r.FirstName), strings.TrimSpace(r.LastName)
	if r.CustomerType == "corporate" || (name == "" && surname == "") {
		title := strings.TrimSpace(r.CompanyTitle)
		if title == "" {
			title = "Müşteri"
		}
		parts := strings.SplitN(title, " ", 2)
		name, surname = parts[0], parts[0]
		if len(parts) == 2 {
			surname = parts[1]
		}
	}
	if surname == "" {
		surname = name
	}
	if locale != "en" {
		locale = "tr"
	}
	if ip == "" {
		ip = "127.0.0.1"
	}
	co, err := p.PreAuth(ctx, acc, payments.CheckoutRequest{
		ConversationID: depUUID.String(), BasketID: depUUID.String(), Amount: amount, Currency: r.Currency,
		CallbackURL: s.preauth.baseURL + "/deposit-preauth/" + depUUID.String() + "/callback", Locale: locale,
		ItemID: r.Reference, ItemName: "Depozito " + r.Reference, ItemCategory: "Depozito",
		Buyer: payments.Buyer{
			ID: r.CustomerUuid.String(), Name: name, Surname: surname, Email: r.Email, Phone: r.Phone,
			// iyzico's documented placeholder (the national id is not sent).
			IdentityNumber: "11111111111", Address: "Türkiye", City: "Istanbul", Country: "Turkey", IP: ip,
		},
	})
	if err != nil {
		_, _ = s.q.CancelSecurityDeposit(ctx, db.CancelSecurityDepositParams{
			OrganizationID: orgID, ID: depID, ResolvedAt: pgtype.Timestamptz{Time: s.clock(), Valid: true},
			CancelReason: "system:preauth_failed", ResolvedBy: actorID(ctx),
		})
		s.publish(ctx, model.ResourceDeposits, "updated", depUUID)
		return &ProviderError{Err: err}
	}
	var expires pgtype.Timestamptz
	if co.ExpiresAt != nil {
		expires = pgtype.Timestamptz{Time: co.ExpiresAt.UTC(), Valid: true}
	}
	if err := s.q.SetSecurityDepositCheckout(ctx, db.SetSecurityDepositCheckoutParams{
		OrganizationID: orgID, ID: depID, ProviderToken: co.Token, PaymentPageUrl: co.PaymentPageURL, AuthorizationExpiresAt: expires,
	}); err != nil {
		return err
	}
	s.publish(ctx, model.ResourceDeposits, "updated", depUUID)
	return nil
}

// VerifyDeposit reads the provider's answer for a pending online
// pre-auth: confirmed (our reference, amount and currency match) → held;
// failed → cancelled; still waiting → unchanged. Only a provider retrieve
// confirms, never the browser return.
func (s *Service) VerifyDeposit(ctx context.Context, orgID int64, id uuid.UUID) (model.Deposit, error) {
	if !has(ctx, rbac.PermTenantFinanceDepositsWrite) {
		return model.Deposit{}, ErrForbidden
	}
	d, err := s.depositRow(ctx, nil, orgID, id)
	if err != nil {
		return model.Deposit{}, err
	}
	if d.Method != model.DepositOnlinePreAuth {
		return model.Deposit{}, stateErr("not_online", "only an online pre-authorization is verified with the provider")
	}
	if d.Status != model.DepositPending {
		return s.GetDeposit(ctx, orgID, id)
	}
	if d.ProviderToken == "" {
		return model.Deposit{}, stateErr("not_started", "the pre-authorization page was not opened")
	}
	acc, p, err := s.depositAccount(ctx, orgID, d)
	if err != nil {
		return model.Deposit{}, err
	}
	res, err := p.Retrieve(ctx, acc, payments.RetrieveRequest{Token: d.ProviderToken, ConversationID: d.Uuid.String()})
	if err != nil {
		return model.Deposit{}, &ProviderError{Err: err}
	}
	switch res.Status {
	case payments.StatusSucceeded:
		if res.ConversationID != d.Uuid.String() || (res.BasketID != "" && res.BasketID != d.Uuid.String()) ||
			!res.Price.Equal(m(d.Amount)) || (res.Currency != "" && res.Currency != d.Currency) || res.PaymentID == "" {
			s.log.Error("deposit_preauth_mismatch", "deposit", d.Uuid.String(), "payment_id", res.PaymentID)
			return model.Deposit{}, stateErr("verification_mismatch", "the provider answer does not match the deposit")
		}
		data, _ := json.Marshal(res.Data)
		if _, err := s.q.MarkSecurityDepositHeld(ctx, db.MarkSecurityDepositHeldParams{
			OrganizationID: orgID, ID: d.ID, HeldAt: pgtype.Timestamptz{Time: s.clock(), Valid: true},
			ProviderPaymentID: res.PaymentID, ProviderData: data,
		}); err != nil {
			return model.Deposit{}, err
		}
	case payments.StatusFailed:
		if _, err := s.q.CancelSecurityDeposit(ctx, db.CancelSecurityDepositParams{
			OrganizationID: orgID, ID: d.ID, ResolvedAt: pgtype.Timestamptz{Time: s.clock(), Valid: true},
			CancelReason: "system:preauth_declined", ResolvedBy: actorID(ctx),
		}); err != nil {
			return model.Deposit{}, err
		}
	}
	s.publish(ctx, model.ResourceDeposits, "updated", id)
	return s.GetDeposit(ctx, orgID, id)
}

// CancelDeposit cancels a pending deposit (an online pre-auth the customer
// did not complete). A pre-auth the provider already confirmed is not
// cancelled here: it is verified and then released.
func (s *Service) CancelDeposit(ctx context.Context, orgID int64, id uuid.UUID, in model.CancelDepositInput) (model.Deposit, error) {
	if !has(ctx, rbac.PermTenantFinanceDepositsWrite) {
		return model.Deposit{}, ErrForbidden
	}
	d, err := s.depositRow(ctx, nil, orgID, id)
	if err != nil {
		return model.Deposit{}, err
	}
	reason, err := requireReason("reason", in.Reason)
	if err != nil {
		return model.Deposit{}, err
	}
	if d.Status != model.DepositPending {
		return model.Deposit{}, stateErr("not_pending", "only a pending deposit is cancelled; a held one is released")
	}
	if d.Method == model.DepositOnlinePreAuth && d.ProviderToken != "" {
		if acc, p, err := s.depositAccount(ctx, orgID, d); err == nil {
			res, err := p.Retrieve(ctx, acc, payments.RetrieveRequest{Token: d.ProviderToken, ConversationID: d.Uuid.String()})
			if err == nil && res.Status == payments.StatusSucceeded {
				return model.Deposit{}, stateErr("preauth_completed", "the customer completed the pre-authorization; verify it, then release it")
			}
		}
	}
	n, err := s.q.CancelSecurityDeposit(ctx, db.CancelSecurityDepositParams{
		OrganizationID: orgID, ID: d.ID, ResolvedAt: pgtype.Timestamptz{Time: s.clock(), Valid: true}, CancelReason: reason,
		ResolvedBy: actorID(ctx),
	})
	if err != nil {
		return model.Deposit{}, err
	}
	if n == 0 {
		return model.Deposit{}, stateErr("not_pending", "only a pending deposit is cancelled")
	}
	s.publish(ctx, model.ResourceDeposits, "updated", id)
	return s.GetDeposit(ctx, orgID, id)
}

// ------------------------------------------------------------- resolving

type plannedDeduction struct {
	in       model.DeductionInput
	amount   money.Money
	applies  string
	reason   string
	chargeID int64
	damageID int64
}

// planDeductions validates the lines against the deposit and the rental.
func (s *Service) planDeductions(ctx context.Context, orgID int64, d db.GetSecurityDepositRowRow, in []model.DeductionInput) ([]plannedDeduction, money.Money, error) {
	ve := &ValidationError{}
	total := money.Zero()
	out := make([]plannedDeduction, 0, len(in))
	perCharge := map[int64]money.Money{}
	for i, x := range in {
		field := "deductions." + itoa(i)
		p := plannedDeduction{in: x, applies: x.AppliesTo}
		if x.Amount == nil || !x.Amount.IsPositive() {
			ve.add(field+".amount", "invalid", "amount must be positive")
		} else {
			p.amount = *x.Amount
			total = total.Add(p.amount)
		}
		r, err := requireReason(field+".reason", x.Reason)
		if err != nil {
			ve.add(field+".reason", "required", "a reason is required (3-500 characters)")
		}
		p.reason = r
		if p.applies == "" {
			p.applies = model.AppliesGeneral
			if x.RentalChargeUUID != nil {
				p.applies = model.AppliesCharge
			}
		}
		if !contains([]string{model.AppliesCharge, model.AppliesRent, model.AppliesGeneral}, p.applies) {
			ve.add(field+".applies_to", "invalid", "applies_to must be charge, rent or general")
		}
		if p.applies == model.AppliesCharge && x.RentalChargeUUID == nil {
			ve.add(field+".rental_charge_uuid", "required", "choose the charge the deduction settles")
		}
		if x.RentalChargeUUID != nil {
			ch, err := s.q.GetRentalChargeForDeposit(ctx, db.GetRentalChargeForDepositParams{OrganizationID: orgID, RentalID: d.RentalID, Uuid: *x.RentalChargeUUID})
			switch {
			case notFound(err):
				ve.add(field+".rental_charge_uuid", "not_found", "unknown charge of this rental")
			case err != nil:
				return nil, total, err
			case ch.Status != "approved" || ch.Reversed || ch.ReversalOfID.Valid || !m(ch.Amount).IsPositive():
				ve.add(field+".rental_charge_uuid", "not_approved", "only an approved charge is deducted (decide a proposed or disputed charge first)")
			default:
				p.chargeID = ch.ID
				used := perCharge[ch.ID]
				if used.IsZero() {
					used = m(ch.Deducted)
				}
				used = used.Add(p.amount)
				perCharge[ch.ID] = used
				if used.Cmp(m(ch.Amount)) > 0 {
					ve.add(field+".amount", "exceeds_charge", "the deductions exceed the charge ("+m(ch.Amount).String()+")")
				}
			}
		}
		if x.InspectionDmgUUID != nil {
			dm, err := s.q.GetInspectionDamageForDeposit(ctx, db.GetInspectionDamageForDepositParams{OrganizationID: orgID, RentalID: d.RentalID, Uuid: *x.InspectionDmgUUID})
			switch {
			case notFound(err):
				ve.add(field+".inspection_damage_uuid", "not_found", "unknown damage of this rental")
			case err != nil:
				return nil, total, err
			default:
				p.damageID = dm.ID
			}
		}
		out = append(out, p)
	}
	if len(in) > 50 {
		ve.add("deductions", "too_many", "at most 50 deductions")
	}
	if total.Cmp(m(d.Amount)) > 0 {
		ve.add("deductions", "exceeds_deposit", "the deductions exceed the deposit ("+m(d.Amount).String()+")")
	}
	return out, total, ve.orNil()
}

func itoa(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return itoa(i/10) + string(rune('0'+i%10))
}

func purposeOf(applies string) string {
	switch applies {
	case model.AppliesCharge:
		return model.PurposeCharge
	case model.AppliesRent:
		return model.PurposeRent
	default:
		return model.PurposeGeneral
	}
}

// ResolveDeposit closes a held deposit: every deduction is captured (it
// needs tenant.finance.deposits.deduct and a reason) and the remainder is
// released (pre-auth) or paid back (cash / transfer) in one transaction.
// Online pre-auths go through the provider: one post-auth of the
// deduction total, or a cancel when nothing is deducted. A replay with
// the same idempotency key answers the resolved deposit; the deposit
// operation task of the rental completes.
func (s *Service) ResolveDeposit(ctx context.Context, orgID int64, id uuid.UUID, in model.ResolveInput) (model.Deposit, error) {
	if !has(ctx, rbac.PermTenantFinanceDepositsWrite) {
		return model.Deposit{}, ErrForbidden
	}
	if len(in.Deductions) > 0 && !has(ctx, rbac.PermTenantFinanceDepositsDeduct) {
		return model.Deposit{}, ErrForbidden
	}
	d, err := s.depositRow(ctx, nil, orgID, id)
	if err != nil {
		return model.Deposit{}, err
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	if len(key) > 64 {
		return model.Deposit{}, validation("idempotency_key", "too_long", "idempotency_key is at most 64 characters")
	}
	if len(in.Note) > 500 {
		return model.Deposit{}, validation("note", "too_long", "note is at most 500 characters")
	}
	if err := resolvedReplay(d.Status, d.ResolutionKey, key); errors.Is(err, errReplay) {
		return s.GetDeposit(ctx, orgID, id)
	} else if err != nil {
		return model.Deposit{}, err
	}
	plan, total, err := s.planDeductions(ctx, orgID, d, in.Deductions)
	if err != nil {
		return model.Deposit{}, err
	}
	amount := m(d.Amount)
	remainder := amount.Sub(total)
	cashHeld := d.Method == model.DepositCash || d.Method == model.DepositBankTransfer
	var online integrations.Account
	var provider preAuthProvider
	if d.Method == model.DepositOnlinePreAuth {
		if online, provider, err = s.depositAccount(ctx, orgID, d); err != nil {
			return model.Deposit{}, err
		}
	}
	status := model.DepositReleased
	switch {
	case total.IsPositive():
		status = model.DepositPartiallyCaptured
	case cashHeld:
		status = model.DepositRefunded
	}
	var accUUID uuid.UUID
	err = s.tx(ctx, func(q *db.Queries) error {
		lock, err := q.LockSecurityDeposit(ctx, db.LockSecurityDepositParams{OrganizationID: orgID, ID: d.ID})
		if err != nil {
			return err
		}
		if lock.Status != model.DepositHeld {
			if key != "" && lock.ResolutionKey == key {
				return errReplay
			}
			if lock.Status == model.DepositPending {
				return stateErr("not_held", "the pre-authorization is not confirmed yet")
			}
			return stateErr("already_resolved", "the deposit is already resolved")
		}
		r, err := s.rentalForDeposit(ctx, q, orgID, d.RentalUuid)
		if err != nil {
			return err
		}
		acc, err := s.ensureCustomerAccount(ctx, q, orgID, d.CustomerID, d.BranchID, d.Currency)
		if err != nil {
			return err
		}
		accUUID = acc.UUID
		if !d.AccountID.Valid {
			if err := q.SetSecurityDepositAccount(ctx, db.SetSecurityDepositAccountParams{OrganizationID: orgID, ID: d.ID, AccountID: int8Of(acc.ID)}); err != nil {
				return err
			}
		}
		if _, err := q.LockAccount(ctx, db.LockAccountParams{OrganizationID: orgID, ID: acc.ID}); err != nil {
			return err
		}
		// Cash accounts first (validation before any posting / provider call).
		var refundCash lockedAccount
		if cashHeld && remainder.IsPositive() {
			field := "refund_cash_account_uuid"
			targetUUID := uuid.UUID(d.CashAccountUuid.Bytes)
			if in.RefundCashAccountUUID != nil {
				targetUUID = *in.RefundCashAccountUUID
			}
			if refundCash, err = s.cashHolder(ctx, q, orgID, field, targetUUID, d.Currency, ""); err != nil {
				return err
			}
			if err := checkFunds(field, refundCash, remainder); err != nil {
				return err
			}
		}
		var captureCash *lockedAccount
		if d.Method == model.DepositPOSPreAuth && total.IsPositive() && in.CaptureCashAccountUUID != nil {
			a, err := s.cashHolder(ctx, q, orgID, "capture_cash_account_uuid", *in.CaptureCashAccountUUID, d.Currency, "")
			if err != nil {
				return err
			}
			captureCash = &a
		}
		// The provider step runs under the deposit lock: a concurrent
		// resolution waits and then sees the deposit closed.
		if provider != nil {
			if total.IsPositive() {
				res, err := provider.Capture(ctx, online, payments.CaptureRequest{
					PaymentID: d.ProviderPaymentID, ConversationID: d.Uuid.String(), Amount: total, Currency: d.Currency, IP: in.ClientIP,
				})
				if err != nil {
					return &ProviderError{Err: err}
				}
				if !res.PaidPrice.IsZero() && !res.PaidPrice.Equal(total) {
					s.log.Error("deposit_postauth_amount_mismatch", "deposit", d.Uuid.String(), "captured", res.PaidPrice.String(), "want", total.String())
				}
			} else if err := provider.Void(ctx, online, payments.VoidRequest{
				PaymentID: d.ProviderPaymentID, ConversationID: d.Uuid.String(), IP: in.ClientIP,
			}); err != nil {
				return &ProviderError{Err: err}
			}
		}
		today := dateOf(s.today())
		notice := []map[string]any{}
		for _, p := range plan {
			x, err := q.InsertDepositDeduction(ctx, db.InsertDepositDeductionParams{
				OrganizationID: orgID, DepositID: d.ID, Amount: p.amount.Numeric(), AppliesTo: p.applies, Reason: p.reason,
				RentalChargeID: int8Of(p.chargeID), InspectionDamageID: int8Of(p.damageID), CreatedBy: actorID(ctx),
			})
			if err != nil {
				return err
			}
			desc := r.Reference + " · " + "depozito kesintisi"
			base := db.InsertDepositLedgerEntryParams{
				OrganizationID: orgID, AccountID: acc.ID, Amount: p.amount.Numeric(), Currency: d.Currency, EntryDate: today,
				Description: desc, Reason: p.reason, SourceUuid: uuidOf(d.Uuid), RentalID: int8Of(d.RentalID),
				ReservationID: int8Of(d.ReservationID), RentalChargeID: int8Of(p.chargeID), SecurityDepositID: int8Of(d.ID),
				DepositDeductionID: int8Of(x.ID), BranchID: int8Of(d.BranchID), CreatedBy: actorID(ctx),
			}
			if cashHeld {
				leg := base
				leg.Kind, leg.Direction, leg.Purpose = model.KindDepositCapture, model.Debit, model.PurposeDeposit
				e, err := q.InsertDepositLedgerEntry(ctx, leg)
				if err != nil {
					return err
				}
				s.publish(ctx, model.ResourceEntries, "created", e.Uuid)
			}
			col := base
			col.Kind, col.Direction, col.Purpose = model.KindCollection, model.Credit, purposeOf(p.applies)
			e, err := q.InsertDepositLedgerEntry(ctx, col)
			if err != nil {
				return err
			}
			s.publish(ctx, model.ResourceEntries, "created", e.Uuid)
			if captureCash != nil {
				if err := s.settleEntry(ctx, q, orgID, captureCash.row.Uuid, e.ID, model.MoveCollection, p.amount, d.Currency, s.today(), desc); err != nil {
					return err
				}
			}
			notice = append(notice, map[string]any{"amount": p.amount.String(), "reason": p.reason, "applies_to": p.applies})
		}
		if cashHeld && remainder.IsPositive() {
			desc := r.Reference + " · depozito iadesi"
			e, err := q.InsertDepositLedgerEntry(ctx, db.InsertDepositLedgerEntryParams{
				OrganizationID: orgID, AccountID: acc.ID, Kind: model.KindDepositReturn, Direction: model.Debit,
				Purpose: model.PurposeDeposit, Amount: remainder.Numeric(), Currency: d.Currency, EntryDate: today,
				Description: desc, Reason: strings.TrimSpace(in.Note), SourceUuid: uuidOf(d.Uuid), RentalID: int8Of(d.RentalID),
				ReservationID: int8Of(d.ReservationID), SecurityDepositID: int8Of(d.ID), BranchID: int8Of(d.BranchID),
				CreatedBy: actorID(ctx),
			})
			if err != nil {
				return err
			}
			s.publish(ctx, model.ResourceEntries, "created", e.Uuid)
			if err := s.depositMovement(ctx, q, orgID, refundCash, model.MoveDepositOut, remainder, d.Currency, e.ID, desc); err != nil {
				return err
			}
		}
		noticeStatus, noticeBody := "none", []byte("{}")
		if len(plan) > 0 {
			noticeStatus = "pending"
			noticeBody, _ = json.Marshal(map[string]any{
				"channel": "sms", "rental_reference": r.Reference, "currency": d.Currency, "deposit": amount.String(),
				"captured": total.String(), "returned": remainder.String(), "deductions": notice,
			})
		}
		now := pgtype.Timestamptz{Time: s.clock(), Valid: true}
		n, err := q.ResolveSecurityDeposit(ctx, db.ResolveSecurityDepositParams{
			OrganizationID: orgID, ID: d.ID, Status: status, CapturedAmount: total.Numeric(), ReturnedAmount: remainder.Numeric(),
			ResolvedAt: now, ResolutionKey: key, ResolutionNote: strings.TrimSpace(in.Note), ResolvedBy: actorID(ctx),
			CustomerNoticeStatus: noticeStatus, CustomerNotice: noticeBody,
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return stateErr("already_resolved", "the deposit is already resolved")
		}
		tasks, err := q.FinishDepositTasks(ctx, db.FinishDepositTasksParams{
			OrganizationID: orgID, RentalID: int8Of(d.RentalID), CompletedAt: now, ActorID: actorID(ctx),
			CompletionNote: "deposit:" + status,
		})
		if err != nil {
			return err
		}
		s.publish(ctx, resourceOperationTasks, "updated", tasks...)
		return nil
	})
	if errors.Is(err, errReplay) {
		return s.GetDeposit(ctx, orgID, id)
	}
	if err != nil {
		return model.Deposit{}, err
	}
	s.publish(ctx, model.ResourceDeposits, "updated", id)
	s.publish(ctx, model.ResourceAccounts, "updated", accUUID)
	return s.GetDeposit(ctx, orgID, id)
}

var errReplay = errors.New("finance: resolution replay")

// resolvedReplay answers a deposit that is not held: a closed one with
// the same idempotency key is a replay (errReplay: the caller reads the
// deposit), anything else is a state error.
func resolvedReplay(status, stored, key string) error {
	switch status {
	case model.DepositHeld:
		return nil
	case model.DepositPending:
		return stateErr("not_held", "the pre-authorization is not confirmed yet")
	case model.DepositCancelled:
		return stateErr("cancelled", "the deposit was cancelled")
	}
	if key != "" && stored == key {
		return errReplay
	}
	return stateErr("already_resolved", "the deposit is already resolved")
}
