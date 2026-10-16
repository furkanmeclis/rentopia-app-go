package handler

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Security deposits (TEC-719): /v1/tenant/finance/deposits/*.

// DepositsMeta is the table contract of GET /v1/tenant/finance/deposits.
func DepositsMeta() resourcemeta.ResourceMeta {
	p := "security_deposits"
	return resourcemeta.ResourceMeta{
		Resource: model.ResourceDeposits, DefaultSort: "-created_at",
		DefaultFields: []string{"created_at", "rental", "customer", "branch", "method", "amount", "status", "held_at"},
		Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Update: true, Search: true, Filter: true, Sort: true,
			Export: true},
		SearchableFields: []string{"rental", "reservation", "customer", "pos_reference"},
		SortableFields:   []string{"created_at", "held_at", "amount"},
		FilterableFields: []string{"status", "method", "branch", "rental", "from", "to", "held_over_days"},
		Columns: []resourcemeta.Column{
			col(p, "created_at", dt, true, false, true), col(p, "rental", str, false, false, true),
			col(p, "customer", str, false, false, true), col(p, "branch", enum, false, true, true),
			col(p, "method", enum, false, true, true), col(p, "amount", str, true, false, true),
			col(p, "captured_amount", str, false, false, false), col(p, "returned_amount", str, false, false, false),
			col(p, "status", enum, false, true, true), col(p, "held_at", dt, true, false, true),
			col(p, "cash_account", str, false, false, false), col(p, "pos_reference", str, false, false, false),
			col(p, "resolved_at", dt, false, false, false), col(p, "uuid", id, false, false, false),
		},
		Filters: []resourcemeta.Filter{
			facet(p, "status", "finance.deposit_status"), facet(p, "method", "finance.deposit_method"), facet(p, "branch", "branches"),
		},
		Includes: []string{}, BulkActions: []bulkengine.BulkActionDef{},
	}
}

// clientIP is the caller's address for the provider's fraud checks.
func clientIP(r *http.Request) string {
	if fwd := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); fwd != "" {
		return fwd
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func depositPayload(d model.Deposit) map[string]any {
	return map[string]any{
		"rental": d.Rental.Code, "method": d.Method, "status": d.Status, "amount": d.Amount.String(),
		"captured": d.CapturedAmount.String(), "returned": d.ReturnedAmount.String(),
	}
}

// DepositsMeta serves GET /v1/tenant/finance/deposits/meta.
func (h *Handler) DepositsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, DepositsMeta())
}

// ListDeposits serves GET /v1/tenant/finance/deposits.
func (h *Handler) ListDeposits(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.DepositSortColumns)
	if !ok {
		return
	}
	from, ok := dateParam(w, r, "from")
	if !ok {
		return
	}
	to, ok := dateParam(w, r, "to")
	if !ok {
		return
	}
	f := adapters.DepositFilter(flat(r.URL.Query()))
	f.Limit, f.Offset, f.Q, f.From, f.To = q.Limit, q.Offset, q.Q, from, to
	f.SortField, f.SortDesc = sortOf(q)
	items, total, err := h.svc.ListDeposits(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// DepositTotals serves GET /v1/tenant/finance/deposits/totals.
func (h *Handler) DepositTotals(w http.ResponseWriter, r *http.Request) {
	days := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("held_over_days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 3650 {
			response.ValidationError(w, r, []response.Detail{{Field: "held_over_days", Code: "invalid", Message: "held_over_days must be 1-3650"}})
			return
		}
		days = n
	}
	out, err := h.svc.DepositTotals(r.Context(), orgctx.Must(r.Context()).ID, days)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// TakeDeposit serves POST /v1/tenant/finance/deposits.
func (h *Handler) TakeDeposit(w http.ResponseWriter, r *http.Request) {
	var in model.DepositInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.ClientIP = clientIP(r)
	in.Locale = r.Header.Get("Accept-Language")
	if strings.HasPrefix(strings.ToLower(in.Locale), "en") {
		in.Locale = "en"
	}
	d, err := h.svc.TakeDeposit(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.deposit_taken", model.ResourceDeposits, d.UUID, depositPayload(d))
	response.JSON(w, r, http.StatusCreated, d)
}

// GetDeposit serves GET /v1/tenant/finance/deposits/{uuid}.
func (h *Handler) GetDeposit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.GetDeposit(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, d)
}

// VerifyDeposit serves POST /v1/tenant/finance/deposits/{uuid}/verify.
func (h *Handler) VerifyDeposit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	d, err := h.svc.VerifyDeposit(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.deposit_verified", model.ResourceDeposits, d.UUID, depositPayload(d))
	response.JSON(w, r, http.StatusOK, d)
}

// ResolveDeposit serves POST /v1/tenant/finance/deposits/{uuid}/resolve.
func (h *Handler) ResolveDeposit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ResolveInput
	if !decodeJSON(w, r, &in) {
		return
	}
	in.ClientIP = clientIP(r)
	if in.IdempotencyKey == "" {
		in.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	}
	d, err := h.svc.ResolveDeposit(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := depositPayload(d)
	payload["deductions"] = len(d.Deductions)
	h.record(r, "finance.deposit_resolved", model.ResourceDeposits, d.UUID, payload)
	response.JSON(w, r, http.StatusOK, d)
}

// CancelDeposit serves POST /v1/tenant/finance/deposits/{uuid}/cancel.
func (h *Handler) CancelDeposit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CancelDepositInput
	if !decodeJSON(w, r, &in) {
		return
	}
	d, err := h.svc.CancelDeposit(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.deposit_cancelled", model.ResourceDeposits, d.UUID, map[string]any{"reason": in.Reason})
	response.JSON(w, r, http.StatusOK, d)
}

// RentalDeposits serves GET /v1/tenant/finance/rentals/{uuid}/deposits.
func (h *Handler) RentalDeposits(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	out, err := h.svc.DepositsForRental(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// UploadDeductionDocument serves PUT
// /v1/tenant/finance/deposits/{uuid}/deductions/{deduction}/file.
func (h *Handler) UploadDeductionDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	did, ok := pathUUID(w, r, "deduction")
	if !ok {
		return
	}
	// The deposit is looked up before the body is read: another
	// organization's uuid answers 404 whatever the body.
	if _, err := h.svc.GetDeposit(r.Context(), orgctx.Must(r.Context()).ID, id); err != nil {
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
	d, err := h.svc.UploadDeductionDocument(r.Context(), orgctx.Must(r.Context()).ID, id, did, usecase.FileUpload{
		Body: file, Size: header.Size, Filename: header.Filename,
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.deposit_document_uploaded", model.ResourceDeposits, d.UUID, map[string]any{"deduction_uuid": did.String()})
	response.JSON(w, r, http.StatusOK, d)
}

// DownloadDeductionDocument serves GET …/deductions/{deduction}/file.
func (h *Handler) DownloadDeductionDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	did, ok := pathUUID(w, r, "deduction")
	if !ok {
		return
	}
	f, err := h.svc.OpenDeductionDocument(r.Context(), orgctx.Must(r.Context()).ID, id, did)
	if err != nil {
		writeError(w, r, err)
		return
	}
	defer func() { _ = f.Body.Close() }()
	w.Header().Set("Content-Type", f.ContentType)
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", disposition+`; filename="`+strings.ReplaceAll(f.Name, `"`, "")+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if f.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f.Body)
}

// DeleteDeductionDocument serves DELETE …/deductions/{deduction}/file.
func (h *Handler) DeleteDeductionDocument(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	did, ok := pathUUID(w, r, "deduction")
	if !ok {
		return
	}
	d, err := h.svc.DeleteDeductionDocument(r.Context(), orgctx.Must(r.Context()).ID, id, did)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.deposit_document_deleted", model.ResourceDeposits, d.UUID, map[string]any{"deduction_uuid": did.String()})
	response.JSON(w, r, http.StatusOK, d)
}
