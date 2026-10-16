package handler

// Collections (TEC-718): /v1/tenant/finance/collections and the open items
// of an account.

import (
	"net/http"
	"strconv"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/httpkit"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourcemeta"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// CollectionsMeta is the table contract of GET /v1/tenant/finance/collections.
func CollectionsMeta() resourcemeta.ResourceMeta {
	p := "finance_collections"
	return resourcemeta.ResourceMeta{
		Resource: model.ResourceCollections, DefaultSort: "-collection_date",
		DefaultFields: []string{"code", "collection_date", "account", "method", "branch", "cash_account", "amount",
			"unallocated_amount", "status"},
		Capabilities: resourcemeta.Capabilities{Create: true, Read: true, Search: true, Filter: true, Sort: true, Export: true,
			Bulk: true},
		SearchableFields: []string{"code", "reference_no", "payer_name", "account", "rental"},
		SortableFields:   []string{"collection_date", "amount", "code", "created_at"},
		FilterableFields: []string{"method", "status", "allocation_status", "branch", "cash_account", "account", "rental", "from", "to"},
		Columns: []resourcemeta.Column{
			col(p, "code", str, true, false, true), col(p, "collection_date", dt, true, false, true),
			col(p, "account", str, false, false, true), col(p, "method", enum, false, true, true),
			col(p, "reference_no", str, false, false, false), col(p, "payer_name", str, false, false, false),
			col(p, "branch", enum, false, true, true), col(p, "cash_account", enum, false, true, true),
			col(p, "rental", str, false, false, false), col(p, "amount", str, true, false, true),
			col(p, "commission_amount", str, false, false, false), col(p, "net_amount", str, false, false, false),
			col(p, "allocated_amount", str, false, false, false), col(p, "unallocated_amount", str, false, false, true),
			col(p, "refunded_amount", str, false, false, false), col(p, "status", enum, false, true, true),
			col(p, "allocation_status", enum, false, true, false), col(p, "settlement_status", enum, false, false, false),
			col(p, "created_at", dt, true, false, false), col(p, "uuid", id, false, false, false),
		},
		Filters: []resourcemeta.Filter{
			facet(p, "method", "finance.method"), facet(p, "status", "finance.collection_status"),
			facet(p, "allocation_status", "finance.allocation_status"), facet(p, "branch", "branches"),
			facet(p, "cash_account", "finance.cash_accounts"),
		},
		Includes: []string{}, BulkActions: adapters.NewCollectionsBulk(nil).BulkActions(),
	}
}

// CollectionsMeta serves GET /v1/tenant/finance/collections/meta.
func (h *Handler) CollectionsMeta(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, r, http.StatusOK, CollectionsMeta())
}

// ListCollections serves GET /v1/tenant/finance/collections.
func (h *Handler) ListCollections(w http.ResponseWriter, r *http.Request) {
	q, ok := listQuery(w, r, usecase.CollectionSortColumns)
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
	f := adapters.CollectionFilter(flat(r.URL.Query()))
	f.Limit, f.Offset, f.Q, f.From, f.To = q.Limit, q.Offset, q.Q, from, to
	f.SortField, f.SortDesc = sortOf(q)
	items, total, err := h.svc.ListCollections(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, apiquery.NewPage(items, total, q.Limit, q.Offset))
}

// CreateCollection serves POST /v1/tenant/finance/collections (201, or
// 200 with the first collection for a repeated idempotency key).
func (h *Handler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	var in model.CollectionCreateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, created, err := h.svc.CreateCollection(r.Context(), orgctx.Must(r.Context()).ID, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
		h.record(r, "finance.collection_recorded", model.ResourceCollections, c.UUID, map[string]any{
			"code": c.Code, "amount": c.Amount.String(), "method": c.Method, "account_uuid": c.Account.UUID.String(),
			"allocated": c.AllocatedAmount.String(),
		})
	}
	response.JSON(w, r, status, c)
}

// GetCollection serves GET /v1/tenant/finance/collections/{uuid}.
func (h *Handler) GetCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	c, err := h.svc.GetCollection(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, c)
}

// AllocateCollection serves POST /v1/tenant/finance/collections/{uuid}/allocations.
func (h *Handler) AllocateCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.AllocateInput
	if !decodeJSON(w, r, &in) {
		return
	}
	before, _ := h.svc.GetCollection(r.Context(), orgctx.Must(r.Context()).ID, id)
	c, err := h.svc.Allocate(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if applied := c.AllocatedAmount.Sub(before.AllocatedAmount); applied.IsPositive() {
		h.record(r, "finance.collection_allocated", model.ResourceCollections, c.UUID, map[string]any{
			"code": c.Code, "amount": applied.String(), "mode": in.Mode,
		})
	}
	response.JSON(w, r, http.StatusOK, c)
}

// ReleaseAllocation serves POST /v1/tenant/finance/collections/{uuid}/allocations/release.
func (h *Handler) ReleaseAllocation(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.ReleaseInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.Release(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"code": c.Code, "reason": in.Reason}
	if in.EntryUUID != nil {
		payload["entry_uuid"] = in.EntryUUID.String()
	}
	h.record(r, "finance.collection_allocation_released", model.ResourceCollections, c.UUID, payload)
	response.JSON(w, r, http.StatusOK, c)
}

// RefundCollection serves POST /v1/tenant/finance/collections/{uuid}/refund.
func (h *Handler) RefundCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CollectionRefundInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.RefundCollection(r.Context(), orgctx.Must(r.Context()).ID, id, in, httpkit.ClientIP(r))
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"code": c.Code, "reason": in.Reason, "refunded": c.RefundedAmount.String()}
	if in.Amount != nil {
		payload["amount"] = in.Amount.String()
	}
	h.record(r, "finance.collection_refunded", model.ResourceCollections, c.UUID, payload)
	response.JSON(w, r, http.StatusOK, c)
}

// VoidCollection serves POST /v1/tenant/finance/collections/{uuid}/void.
func (h *Handler) VoidCollection(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.CollectionVoidInput
	if !decodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.VoidCollection(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "finance.collection_voided", model.ResourceCollections, c.UUID, map[string]any{
		"code": c.Code, "amount": c.Amount.String(), "reason": in.Reason,
	})
	response.JSON(w, r, http.StatusOK, c)
}

// OpenItems serves GET /v1/tenant/finance/accounts/{uuid}/open-items.
func (h *Handler) OpenItems(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var rental *uuid.UUID
	if raw := r.URL.Query().Get("rental"); raw != "" {
		v, err := uuid.Parse(raw)
		if err != nil {
			response.ValidationError(w, r, []response.Detail{{Field: "rental", Message: "rental is invalid", Code: "invalid"}})
			return
		}
		rental = &v
	}
	open, _ := strconv.ParseBool(r.URL.Query().Get("open"))
	out, err := h.svc.OpenItems(r.Context(), orgctx.Must(r.Context()).ID, id, rental, open)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
