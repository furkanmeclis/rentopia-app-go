package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Supplier invoices and payments (TEC-721). Activity is recorded on the
// invoice / payment resource and, for the supplier detail timeline, the
// payload carries the supplier uuid.

// SupplierInvoicesMeta serves GET /v1/tenant/finance/supplier-invoices/meta.
func (h *Handler) SupplierInvoicesMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, SupplierInvoicesMeta())
}

// ListSupplierInvoices serves GET /v1/tenant/finance/supplier-invoices.
func (h *Handler) ListSupplierInvoices(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.SupplierInvoiceSortColumns)
	if !ok {
		return
	}
	f := adapters.SupplierInvoiceFilter(flat(r.URL.Query()))
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	f.SortField, f.SortDesc = sortOf(q)
	items, total, err := h.svc.ListSupplierInvoices(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateSupplierInvoice serves POST /v1/tenant/finance/supplier-invoices.
func (h *Handler) CreateSupplierInvoice(w http.ResponseWriter, r *http.Request) {
	var in model.SupplierInvoiceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	inv, err := h.svc.CreateSupplierInvoice(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.supplier_invoice_created", model.ResourceSupplierInvoices, inv.UUID, map[string]any{
		"invoice_no": inv.InvoiceNo, "total_amount": inv.TotalAmount.String(), "supplier_uuid": inv.Supplier.UUID.String(),
	})
	response.JSON(w, r, http.StatusCreated, inv)
}

// GetSupplierInvoice serves GET /v1/tenant/finance/supplier-invoices/{uuid}.
func (h *Handler) GetSupplierInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	inv, err := h.svc.GetSupplierInvoice(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, inv)
}

// PatchSupplierInvoice serves PATCH /v1/tenant/finance/supplier-invoices/{uuid}.
func (h *Handler) PatchSupplierInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.SupplierInvoiceInput
	if !decodeJSON(w, r, &in) {
		return
	}
	inv, err := h.svc.PatchSupplierInvoice(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.supplier_invoice_updated", model.ResourceSupplierInvoices, inv.UUID, map[string]any{
		"invoice_no": inv.InvoiceNo, "due_date": inv.DueDate, "supplier_uuid": inv.Supplier.UUID.String(),
	})
	response.JSON(w, r, http.StatusOK, inv)
}

// VoidSupplierInvoice serves POST /v1/tenant/finance/supplier-invoices/{uuid}/void.
func (h *Handler) VoidSupplierInvoice(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.VoidInput
	if !decodeJSON(w, r, &in) {
		return
	}
	inv, err := h.svc.VoidSupplierInvoice(r.Context(), orgctx.Must(r.Context()).ID, id, in.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.supplier_invoice_voided", model.ResourceSupplierInvoices, inv.UUID, map[string]any{
		"invoice_no": inv.InvoiceNo, "reason": inv.VoidReason, "supplier_uuid": inv.Supplier.UUID.String(),
	})
	response.JSON(w, r, http.StatusOK, inv)
}

// UploadInvoiceAttachment serves PUT /v1/tenant/finance/supplier-invoices/{uuid}/file
// (multipart "file").
func (h *Handler) UploadInvoiceAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	scope := orgctx.Must(r.Context())
	// Looked up before the body is read: another organization's uuid
	// answers 404 whatever the body.
	if _, err := h.svc.GetSupplierInvoice(r.Context(), scope.ID, id); err != nil {
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
	inv, err := h.svc.UploadInvoiceAttachment(r.Context(), scope.ID, id, usecase.FileUpload{Body: file, Size: header.Size, Filename: header.Filename})
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.supplier_invoice_attachment_uploaded", model.ResourceSupplierInvoices, inv.UUID, map[string]any{"name": inv.Attachment.Name})
	response.JSON(w, r, http.StatusOK, inv)
}

// DownloadInvoiceAttachment serves GET /v1/tenant/finance/supplier-invoices/{uuid}/file:
// streamed by the API, never a storage URL.
func (h *Handler) DownloadInvoiceAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	f, err := h.svc.OpenInvoiceAttachment(r.Context(), orgctx.Must(r.Context()).ID, id)
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

// DeleteInvoiceAttachment serves DELETE /v1/tenant/finance/supplier-invoices/{uuid}/file.
func (h *Handler) DeleteInvoiceAttachment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	inv, err := h.svc.DeleteInvoiceAttachment(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.supplier_invoice_attachment_deleted", model.ResourceSupplierInvoices, inv.UUID, map[string]any{})
	response.JSON(w, r, http.StatusOK, inv)
}

// PayablesSummary serves GET /v1/tenant/finance/payables/summary
// (?window=<days>, default 7).
func (h *Handler) PayablesSummary(w http.ResponseWriter, r *http.Request) {
	window, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("window")))
	out, err := h.svc.PayablesSummary(r.Context(), orgctx.Must(r.Context()).ID, window)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// SupplierPaymentsMeta serves GET /v1/tenant/finance/supplier-payments/meta.
func (h *Handler) SupplierPaymentsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, SupplierPaymentsMeta())
}

// ListSupplierPayments serves GET /v1/tenant/finance/supplier-payments.
func (h *Handler) ListSupplierPayments(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.SupplierPaymentSortColumns)
	if !ok {
		return
	}
	f := adapters.SupplierPaymentFilter(flat(r.URL.Query()))
	f.Limit, f.Offset, f.Q = q.Limit, q.Offset, q.Q
	f.SortField, f.SortDesc = sortOf(q)
	items, total, err := h.svc.ListSupplierPayments(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateSupplierPayment serves POST /v1/tenant/finance/supplier-payments.
func (h *Handler) CreateSupplierPayment(w http.ResponseWriter, r *http.Request) {
	var in model.SupplierPaymentInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := h.svc.CreateSupplierPayment(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.supplier_payment_recorded", model.ResourceSupplierPayments, p.UUID, map[string]any{
		"amount": p.Amount.String(), "allocated_amount": p.AllocatedAmount.String(), "supplier_uuid": p.Supplier.UUID.String(),
		"cash_account": p.CashAccount.Code,
	})
	response.JSON(w, r, http.StatusCreated, p)
}

// GetSupplierPayment serves GET /v1/tenant/finance/supplier-payments/{uuid}.
func (h *Handler) GetSupplierPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	p, err := h.svc.GetSupplierPayment(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, p)
}

// VoidSupplierPayment serves POST /v1/tenant/finance/supplier-payments/{uuid}/void.
func (h *Handler) VoidSupplierPayment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.VoidInput
	if !decodeJSON(w, r, &in) {
		return
	}
	p, err := h.svc.VoidSupplierPayment(r.Context(), orgctx.Must(r.Context()).ID, id, in.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.supplier_payment_voided", model.ResourceSupplierPayments, p.UUID, map[string]any{
		"amount": p.Amount.String(), "reason": p.VoidReason, "supplier_uuid": p.Supplier.UUID.String(),
	})
	response.JSON(w, r, http.StatusOK, p)
}
