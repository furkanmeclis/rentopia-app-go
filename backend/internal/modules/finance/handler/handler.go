// Package handler is the HTTP layer of finance (/v1/tenant/finance/*):
// accounts and their ledger, cash / bank accounts, movements, transfers,
// expense categories, expenses and reports. Every handler runs behind
// RequireOrganization; the organization comes from orgctx, never the client.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// CodeInvalidState answers a refused transition (409).
const CodeInvalidState = "INVALID_STATE"

// Handler serves /v1/tenant/finance.
type Handler struct {
	svc      *usecase.Service
	activity *activity.Recorder
}

// New builds the handler. rec may be nil.
func New(svc *usecase.Service, rec *activity.Recorder) *Handler {
	return &Handler{svc: svc, activity: rec}
}

// --------------------------------------------------------------- helpers

// flat turns query values into the engine query shape (repeated keys are
// comma-joined), so lists and exports share one filter mapping.
func flat(values url.Values) map[string]string {
	out := map[string]string{}
	for k, vs := range values {
		parts := []string{}
		for _, v := range vs {
			if v = strings.TrimSpace(v); v != "" {
				parts = append(parts, v)
			}
		}
		if len(parts) > 0 {
			out[k] = strings.Join(parts, ",")
		}
	}
	return out
}

func listQuery(w http.ResponseWriter, r *http.Request, sortable map[string]string) (apiquery.Query, bool) {
	values := r.URL.Query()
	if banned := apiquery.ForbiddenParams(values); len(banned) > 0 {
		response.ValidationError(w, r, []response.Detail{{Field: banned[0], Message: "use limit/offset/q instead of " + banned[0], Code: "forbidden"}})
		return apiquery.Query{}, false
	}
	q := apiquery.Parse(values)
	if sortable != nil {
		if err := apiquery.ValidateSort(q.Sort, apiquery.SortColumns(sortable)); err != nil {
			writeError(w, r, err)
			return q, false
		}
	}
	return q, true
}

func sortOf(q apiquery.Query) (string, bool) {
	if len(q.Sort) == 0 {
		return "", false
	}
	return q.Sort[0].Field, q.Sort[0].Desc
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: name, Message: name + " is invalid", Code: "invalid"}})
		return uuid.Nil, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(w, r, response.CodeValidationError, "Invalid request body")
		return false
	}
	return true
}

func dateParam(w http.ResponseWriter, r *http.Request, key string) (*time.Time, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: key, Message: key + " must be YYYY-MM-DD", Code: "invalid"}})
		return nil, false
	}
	return &t, true
}

// record writes a fail-soft activity event (resource = engine slug).
func (h *Handler) record(r *http.Request, action, resource string, id uuid.UUID, payload map[string]any) {
	if h.activity == nil {
		return
	}
	p := authctx.MustPrincipal(r.Context())
	h.activity.Record(r.Context(), &p.UserInternal, action, resource, &id, payload, r)
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	response.RecordFailure(w, err)
	var qve *apiquery.ValidationError
	if errors.As(err, &qve) {
		out := make([]response.Detail, 0, len(qve.Details))
		for _, d := range qve.Details {
			out = append(out, response.Detail{Field: d.Field, Message: d.Message, Code: d.Code})
		}
		response.ValidationError(w, r, out)
		return
	}
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		response.ValidationError(w, r, out)
		return
	}
	var se *usecase.StateError
	if errors.As(err, &se) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, se.Message, []response.Detail{{Code: se.Code, Message: se.Message}})
		return
	}
	var sve *smsusecase.ValidationError
	if errors.As(err, &sve) {
		response.ValidationError(w, r, []response.Detail{{Field: sve.Field, Code: sve.Code, Message: sve.Message}})
		return
	}
	var pve *paymentsusecase.ValidationError
	if errors.As(err, &pve) {
		out := make([]response.Detail, 0, len(pve.Details))
		for _, d := range pve.Details {
			out = append(out, response.Detail{Field: "pay_link." + d.Field, Code: d.Code, Message: d.Message})
		}
		response.ValidationError(w, r, out)
		return
	}
	var pse *paymentsusecase.StateError
	if errors.As(err, &pse) {
		response.ErrorWithDetails(w, r, http.StatusConflict, CodeInvalidState, pse.Message, []response.Detail{{Code: pse.Code, Message: pse.Message}})
		return
	}
	switch {
	case errors.Is(err, usecase.ErrAccountNotFound):
		response.NotFound(w, r, "Account was not found")
	case errors.Is(err, usecase.ErrEntryNotFound):
		response.NotFound(w, r, "Ledger entry was not found")
	case errors.Is(err, usecase.ErrRentalNotFound):
		response.NotFound(w, r, "Rental was not found")
	case errors.Is(err, usecase.ErrCustomerNotFound):
		response.NotFound(w, r, "Customer was not found")
	case errors.Is(err, usecase.ErrCashAccountNotFound):
		response.NotFound(w, r, "Cash account was not found")
	case errors.Is(err, usecase.ErrMovementNotFound):
		response.NotFound(w, r, "Cash movement was not found")
	case errors.Is(err, usecase.ErrCategoryNotFound):
		response.NotFound(w, r, "Expense category was not found")
	case errors.Is(err, usecase.ErrExpenseNotFound):
		response.NotFound(w, r, "Expense was not found")
	case errors.Is(err, usecase.ErrNoAttachment):
		response.NotFound(w, r, "The expense has no attachment")
	case errors.Is(err, usecase.ErrDepositNotFound):
		response.NotFound(w, r, "Security deposit was not found")
	case errors.Is(err, usecase.ErrDeductionNotFound):
		response.NotFound(w, r, "Deposit deduction was not found")
	case errors.Is(err, usecase.ErrNoDocument):
		response.NotFound(w, r, "The deduction has no document")
	case errors.As(err, new(*usecase.ProviderError)):
		code, msg := "provider_error", err.Error()
		var pe *payments.ProviderError
		if errors.As(err, &pe) && pe.Code != "" {
			code, msg = pe.Code, pe.Message
		}
		response.ErrorWithDetails(w, r, http.StatusBadGateway, "PROVIDER_ERROR", "The payment provider refused the request",
			[]response.Detail{{Code: code, Message: msg}})
	case errors.Is(err, usecase.ErrForbidden):
		response.Forbidden(w, r, "Missing permission")
	case errors.Is(err, usecase.ErrAllBranchesRequired), errors.Is(err, orgctx.ErrAllBranchesRequired):
		response.Forbidden(w, r, err.Error())
	case errors.Is(err, usecase.ErrCodeTaken):
		response.ErrorWithDetails(w, r, http.StatusConflict, response.CodeConflict, err.Error(), []response.Detail{{Field: "code", Message: err.Error(), Code: "taken"}})
	case errors.Is(err, usecase.ErrStorageUnavailable):
		response.ServiceUnavailable(w, r, "STORAGE_UNAVAILABLE", err.Error())
	case errors.Is(err, smsusecase.ErrNoAccount), errors.Is(err, paymentsusecase.ErrNoAccount):
		response.ErrorWithDetails(w, r, http.StatusUnprocessableEntity, "ACCOUNT_REQUIRED", err.Error(),
			[]response.Detail{{Code: "provider_account_required", Message: err.Error()}})
	case errors.Is(err, smsusecase.ErrRateLimited):
		response.TooManyRequests(w, r, err.Error())
	default:
		response.InternalErr(w, r, err, "Unexpected server error")
	}
}

// ----------------------------------------------------------------- ledger

// Summary serves GET /v1/tenant/finance/summary.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Summary(r.Context(), orgctx.Must(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// RentalBalances serves GET /v1/tenant/finance/rental-balances.
func (h *Handler) RentalBalances(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, nil)
	if !ok {
		return
	}
	open, _ := strconv.ParseBool(r.URL.Query().Get("open"))
	items, total, err := h.svc.ListRentalBalances(r.Context(), orgctx.Must(r.Context()).ID, q.Q, open, q.Limit, q.Offset)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// AccountsMeta serves GET /v1/tenant/finance/accounts/meta.
func (h *Handler) AccountsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, AccountsMeta())
}

// ListAccounts serves GET /v1/tenant/finance/accounts.
func (h *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.AccountSortColumns)
	if !ok {
		return
	}
	f := adapters.AccountFilter(flat(r.URL.Query()))
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	f.SortField, f.SortDesc = sortOf(q)
	items, total, err := h.svc.ListAccounts(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateAccount serves POST /v1/tenant/finance/accounts.
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var in model.AccountCreateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	a, created, err := h.svc.CreateAccount(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		h.record(r, "finance.account_created", model.ResourceAccounts, a.UUID, map[string]any{"code": a.Code})
	}
	response.JSON(w, r, status, a)
}

// GetAccount serves GET /v1/tenant/finance/accounts/{uuid}.
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	a, err := h.svc.GetAccount(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

// PatchAccount serves PATCH /v1/tenant/finance/accounts/{uuid}.
func (h *Handler) PatchAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.AccountPatch
	if !decodeJSON(w, r, &in) {
		return
	}
	a, err := h.svc.PatchAccount(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.account_updated", model.ResourceAccounts, a.UUID, map[string]any{"code": a.Code})
	response.JSON(w, r, http.StatusOK, a)
}

func entryFilter(w http.ResponseWriter, r *http.Request) (model.EntryFilter, bool) {
	q, ok := listQuery(w, r, nil)
	if !ok {
		return model.EntryFilter{}, false
	}
	f := adapters.EntryFilter(flat(r.URL.Query()))
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	f.SortDesc = true
	if len(q.Sort) > 0 {
		f.SortDesc = q.Sort[0].Desc
	}
	return f, true
}

// Statement serves GET /v1/tenant/finance/accounts/{uuid}/statement
// (oldest first by default, with the running balance).
func (h *Handler) Statement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	f, ok := entryFilter(w, r)
	if !ok {
		return
	}
	f.SortDesc = strings.HasPrefix(r.URL.Query().Get("sort"), "-")
	f.AccountUUID = nil
	st, err := h.svc.Statement(r.Context(), orgctx.Must(r.Context()).ID, id, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, st)
}

// Collect serves POST /v1/tenant/finance/accounts/{uuid}/collections.
func (h *Handler) Collect(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CollectionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	e, err := h.svc.Collect(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.collection_recorded", model.ResourceAccounts, id, map[string]any{
		"entry_uuid": e.UUID.String(), "amount": e.Amount.String(), "purpose": e.Purpose,
	})
	response.JSON(w, r, http.StatusCreated, e)
}

// Refund serves POST /v1/tenant/finance/accounts/{uuid}/refunds.
func (h *Handler) Refund(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.RefundInput
	if !decodeJSON(w, r, &in) {
		return
	}
	e, err := h.svc.Refund(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.refund_recorded", model.ResourceAccounts, id, map[string]any{
		"entry_uuid": e.UUID.String(), "amount": e.Amount.String(), "reason": e.Reason,
	})
	response.JSON(w, r, http.StatusCreated, e)
}

// Adjust serves POST /v1/tenant/finance/accounts/{uuid}/adjustments.
func (h *Handler) Adjust(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.AdjustmentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	e, err := h.svc.Adjust(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.adjustment_posted", model.ResourceAccounts, id, map[string]any{
		"entry_uuid": e.UUID.String(), "amount": e.Amount.String(), "direction": e.Direction, "reason": e.Reason,
	})
	response.JSON(w, r, http.StatusCreated, e)
}

// Offset serves POST /v1/tenant/finance/accounts/{uuid}/offsets.
func (h *Handler) Offset(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.OffsetInput
	if !decodeJSON(w, r, &in) {
		return
	}
	items, err := h.svc.Offset(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.deposit_offset", model.ResourceAccounts, id, map[string]any{"amount": items[0].Amount.String(), "reason": items[0].Reason})
	response.JSON(w, r, http.StatusCreated, map[string]any{"items": items})
}

// SendStatement serves POST /v1/tenant/finance/accounts/{uuid}/send-statement.
func (h *Handler) SendStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.StatementSendInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.SendStatement(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"phone": out.PhoneMasked, "balance": out.Balance.String(), "pay_link": out.PayLinkUUID != nil}
	if out.PayLinkUUID != nil {
		payload["pay_link_uuid"] = out.PayLinkUUID.String()
	}
	h.record(r, "finance.statement_sent", model.ResourceAccounts, id, payload)
	response.JSON(w, r, http.StatusOK, out)
}

// EntriesMeta serves GET /v1/tenant/finance/entries/meta.
func (h *Handler) EntriesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, EntriesMeta())
}

// ListEntries serves GET /v1/tenant/finance/entries.
func (h *Handler) ListEntries(w http.ResponseWriter, r *http.Request) {
	f, ok := entryFilter(w, r)
	if !ok {
		return
	}
	items, total, err := h.svc.ListEntries(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, f.Limit, f.Offset))
}

// GetEntry serves GET /v1/tenant/finance/entries/{uuid}.
func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	e, err := h.svc.GetEntry(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, e)
}

// ReverseEntry serves POST /v1/tenant/finance/entries/{uuid}/reverse.
func (h *Handler) ReverseEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	e, err := h.svc.Reverse(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.entry_reversed", model.ResourceAccounts, e.Account.UUID, map[string]any{
		"entry_uuid": id.String(), "reversal_uuid": e.UUID.String(), "amount": e.Amount.String(), "reason": e.Reason,
	})
	response.JSON(w, r, http.StatusCreated, e)
}

// RentalLedger serves GET /v1/tenant/finance/rentals/{uuid}.
func (h *Handler) RentalLedger(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.RentalLedger(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CustomerLedger serves GET /v1/tenant/finance/customers/{uuid}.
func (h *Handler) CustomerLedger(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.CustomerLedger(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// ------------------------------------------------------------------- cash

// CashAccountsMeta serves GET /v1/tenant/finance/cash-accounts/meta.
func (h *Handler) CashAccountsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, CashAccountsMeta())
}

// ListCashAccounts serves GET /v1/tenant/finance/cash-accounts.
func (h *Handler) ListCashAccounts(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.CashAccountSortColumns)
	if !ok {
		return
	}
	f := adapters.CashFilter(flat(r.URL.Query()))
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	f.SortField, f.SortDesc = sortOf(q)
	items, total, err := h.svc.ListCashAccounts(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateCashAccount serves POST /v1/tenant/finance/cash-accounts.
func (h *Handler) CreateCashAccount(w http.ResponseWriter, r *http.Request) {
	var in model.CashAccountInput
	if !decodeJSON(w, r, &in) {
		return
	}
	a, err := h.svc.CreateCashAccount(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.cash_account_created", model.ResourceCashAccounts, a.UUID, map[string]any{"code": a.Code, "kind": a.Kind})
	response.JSON(w, r, http.StatusCreated, a)
}

// GetCashAccount serves GET /v1/tenant/finance/cash-accounts/{uuid}.
func (h *Handler) GetCashAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	a, err := h.svc.GetCashAccount(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, a)
}

// PatchCashAccount serves PATCH /v1/tenant/finance/cash-accounts/{uuid}.
func (h *Handler) PatchCashAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CashAccountInput
	if !decodeJSON(w, r, &in) {
		return
	}
	a, err := h.svc.PatchCashAccount(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.cash_account_updated", model.ResourceCashAccounts, a.UUID, map[string]any{"code": a.Code})
	response.JSON(w, r, http.StatusOK, a)
}

// RecordMovement serves POST /v1/tenant/finance/cash-accounts/{uuid}/movements.
func (h *Handler) RecordMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.MovementInput
	if !decodeJSON(w, r, &in) {
		return
	}
	mv, err := h.svc.RecordMovement(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.movement_recorded", model.ResourceCashAccounts, id, map[string]any{
		"movement_uuid": mv.UUID.String(), "kind": mv.Kind, "amount": mv.Amount.String(),
	})
	response.JSON(w, r, http.StatusCreated, mv)
}

// Transfer serves POST /v1/tenant/finance/transfers.
func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	var in model.TransferInput
	if !decodeJSON(w, r, &in) {
		return
	}
	t, err := h.svc.Transfer(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.transfer_made", model.ResourceCashAccounts, t.Out.Account.UUID, map[string]any{
		"to_account_uuid": t.In.Account.UUID.String(), "amount": t.Out.Amount.String(), "transfer_group": t.Group.String(),
	})
	response.JSON(w, r, http.StatusCreated, t)
}

// MovementsMeta serves GET /v1/tenant/finance/movements/meta.
func (h *Handler) MovementsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, MovementsMeta())
}

// ListMovements serves GET /v1/tenant/finance/movements.
func (h *Handler) ListMovements(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.MovementSortColumns)
	if !ok {
		return
	}
	f := adapters.MovementFilter(flat(r.URL.Query()))
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	f.SortField, f.SortDesc = sortOf(q)
	items, total, err := h.svc.ListMovements(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// GetMovement serves GET /v1/tenant/finance/movements/{uuid}.
func (h *Handler) GetMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	mv, err := h.svc.GetMovement(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, mv)
}

// ReverseMovement serves POST /v1/tenant/finance/movements/{uuid}/reverse.
func (h *Handler) ReverseMovement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	items, err := h.svc.ReverseMovement(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.movement_reversed", model.ResourceCashAccounts, items[0].Account.UUID, map[string]any{
		"movement_uuid": id.String(), "reason": items[0].Reason,
	})
	response.JSON(w, r, http.StatusCreated, map[string]any{"items": items})
}

// --------------------------------------------------------------- expenses

// ListCategories serves GET /v1/tenant/finance/expense-categories.
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	active, _ := strconv.ParseBool(r.URL.Query().Get("active"))
	items, err := h.svc.ListCategories(r.Context(), orgctx.Must(r.Context()).ID, strings.TrimSpace(r.URL.Query().Get("q")), active)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// CreateCategory serves POST /v1/tenant/finance/expense-categories.
func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	var in model.CategoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.CreateCategory(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.category_created", model.ResourceCategories, c.UUID, map[string]any{"code": c.Code, "name": c.Name})
	response.JSON(w, r, http.StatusCreated, c)
}

// PatchCategory serves PATCH /v1/tenant/finance/expense-categories/{uuid}.
func (h *Handler) PatchCategory(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CategoryInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.PatchCategory(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.category_updated", model.ResourceCategories, c.UUID, map[string]any{"code": c.Code, "name": c.Name})
	response.JSON(w, r, http.StatusOK, c)
}

// ExpensesMeta serves GET /v1/tenant/finance/expenses/meta.
func (h *Handler) ExpensesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, ExpensesMeta())
}

// ListExpenses serves GET /v1/tenant/finance/expenses.
func (h *Handler) ListExpenses(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.ExpenseSortColumns)
	if !ok {
		return
	}
	f := adapters.ExpenseFilter(flat(r.URL.Query()))
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	f.SortField, f.SortDesc = sortOf(q)
	items, total, err := h.svc.ListExpenses(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateExpense serves POST /v1/tenant/finance/expenses.
func (h *Handler) CreateExpense(w http.ResponseWriter, r *http.Request) {
	var in model.ExpenseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	e, err := h.svc.CreateExpense(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.expense_created", model.ResourceExpenses, e.UUID, map[string]any{
		"category": e.Category.Code, "total_amount": e.TotalAmount.String(),
	})
	response.JSON(w, r, http.StatusCreated, e)
}

// GetExpense serves GET /v1/tenant/finance/expenses/{uuid}.
func (h *Handler) GetExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	e, err := h.svc.GetExpense(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, e)
}

// PatchExpense serves PATCH /v1/tenant/finance/expenses/{uuid}.
func (h *Handler) PatchExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ExpenseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	e, err := h.svc.PatchExpense(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.expense_updated", model.ResourceExpenses, e.UUID, map[string]any{"category": e.Category.Code})
	response.JSON(w, r, http.StatusOK, e)
}

// VoidExpense serves POST /v1/tenant/finance/expenses/{uuid}/void.
func (h *Handler) VoidExpense(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ReverseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	e, err := h.svc.VoidExpense(r.Context(), orgctx.Must(r.Context()).ID, id, in.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.expense_voided", model.ResourceExpenses, e.UUID, map[string]any{"reason": e.VoidReason})
	response.JSON(w, r, http.StatusOK, e)
}

// UploadAttachment serves PUT /v1/tenant/finance/expenses/{uuid}/file
// (multipart "file").
func (h *Handler) UploadAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	scope := orgctx.Must(r.Context())
	// The expense is looked up before the body is read: another
	// organization's uuid answers 404 whatever the body.
	if _, err := h.svc.GetExpense(r.Context(), scope.ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, usecase.MaxAttachmentBytes+(1<<20))
	if err := r.ParseMultipartForm(usecase.MaxAttachmentBytes); err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file must be at most 10 MiB", Code: "too_large"}})
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.ValidationError(w, r, []response.Detail{{Field: "file", Message: "file is required", Code: "required"}})
		return
	}
	defer func() { _ = file.Close() }()
	e, err := h.svc.UploadAttachment(r.Context(), scope.ID, id, usecase.FileUpload{Body: file, Size: header.Size, Filename: header.Filename})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.expense_attachment_uploaded", model.ResourceExpenses, e.UUID, map[string]any{"name": e.Attachment.Name})
	response.JSON(w, r, http.StatusOK, e)
}

// DownloadAttachment serves GET /v1/tenant/finance/expenses/{uuid}/file:
// streamed by the API, never a storage URL.
func (h *Handler) DownloadAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	f, err := h.svc.OpenAttachment(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Disposition", disposition+`; filename="`+strings.ReplaceAll(f.Name, `"`, "")+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

// DeleteAttachment serves DELETE /v1/tenant/finance/expenses/{uuid}/file.
func (h *Handler) DeleteAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	e, err := h.svc.DeleteAttachment(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.expense_attachment_deleted", model.ResourceExpenses, e.UUID, map[string]any{})
	response.JSON(w, r, http.StatusOK, e)
}

// ---------------------------------------------------------------- reports

// ExpenseReport serves GET /v1/tenant/finance/reports/expenses.
func (h *Handler) ExpenseReport(w http.ResponseWriter, r *http.Request) {
	from, ok := dateParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := dateParam(w, r, "to")
	if !ok {
		return
	}
	group := r.URL.Query().Get("group_by")
	if group == "" {
		group = "category"
	}
	out, err := h.svc.ExpenseReport(r.Context(), orgctx.Must(r.Context()).ID, group, from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// CashFlowReport serves GET /v1/tenant/finance/reports/cash-flow.
func (h *Handler) CashFlowReport(w http.ResponseWriter, r *http.Request) {
	from, ok := dateParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := dateParam(w, r, "to")
	if !ok {
		return
	}
	group := r.URL.Query().Get("group_by")
	if group == "" {
		group = "account"
	}
	out, err := h.svc.CashFlowReport(r.Context(), orgctx.Must(r.Context()).ID, group, from, to)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
