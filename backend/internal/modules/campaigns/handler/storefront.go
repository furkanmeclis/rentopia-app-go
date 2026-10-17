package handler

import (
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/campaigns/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"net/http"
)

func (h *Handler) GetStorefront(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	out, err := h.svc.GetStorefront(r.Context(), orgctx.Must(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
func (h *Handler) PatchStorefront(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r)
	if !ok {
		return
	}
	var in model.StorefrontInput
	if !decodeJSON(w, r, &in) {
		return
	}
	out, err := h.svc.PatchStorefront(r.Context(), orgctx.Must(r.Context()).ID, id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	h.record(r, "campaigns.storefront_updated", id, resourceevents.ActionUpdated, map[string]any{"show_on_site": out.ShowOnSite})
	response.JSON(w, r, http.StatusOK, out)
}
