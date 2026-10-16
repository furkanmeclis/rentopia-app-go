// Package handler is the HTTP layer of the vehicle profitability report
// (TEC-727). The organization and the branch scope come from orgctx, never
// from the client.
package handler

import (
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/profitability/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Handler serves /v1/tenant/reports/vehicle-profitability.
type Handler struct {
	svc *usecase.Service
}

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	switch {
	case errors.As(err, &ve):
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		response.ValidationError(w, r, out)
	case errors.Is(err, usecase.ErrVehicleNotFound):
		response.NotFound(w, r, "Vehicle was not found")
	default:
		response.InternalErr(w, r, err, "could not compute the profitability report")
	}
}

// Report serves GET /v1/tenant/reports/vehicle-profitability.
func (h *Handler) Report(w http.ResponseWriter, r *http.Request) {
	f, err := usecase.ParseFilter(r.URL.Query().Get)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.svc.Report(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Vehicle serves GET /v1/tenant/reports/vehicle-profitability/vehicles/{uuid}.
func (h *Handler) Vehicle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("uuid"))
	if err != nil {
		response.NotFound(w, r, "Vehicle was not found")
		return
	}
	f, err := usecase.ParseFilter(r.URL.Query().Get)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, err := h.svc.Vehicle(r.Context(), orgctx.Must(r.Context()).ID, id, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}
