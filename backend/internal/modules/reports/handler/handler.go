// Package handler is the HTTP layer of the reports (TEC-738). The
// organization and the branch scope come from orgctx, never from the
// client.
package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reports/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
)

// Handler serves /v1/tenant/reports/*.
type Handler struct {
	svc *usecase.Service
}

// New builds the handler.
func New(svc *usecase.Service) *Handler { return &Handler{svc: svc} }

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var ve *usecase.ValidationError
	if errors.As(err, &ve) {
		out := make([]response.Detail, 0, len(ve.Details))
		for _, d := range ve.Details {
			out = append(out, response.Detail{Field: d.Field, Code: d.Code, Message: d.Message})
		}
		response.ValidationError(w, r, out)
		return
	}
	response.InternalErr(w, r, err, "could not compute the report")
}

// serve parses the filters, runs one report and writes it.
func serve[T any](w http.ResponseWriter, r *http.Request, run func(context.Context, int64, model.Filter) (T, error)) {
	f, err := usecase.ParseFilter(r.URL.Query().Get)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out, err := run(r.Context(), orgctx.Must(r.Context()).ID, f)
	if err != nil {
		writeError(w, r, err)
		return
	}
	response.JSON(w, r, http.StatusOK, out)
}

// Occupancy serves GET /v1/tenant/reports/occupancy.
func (h *Handler) Occupancy(w http.ResponseWriter, r *http.Request) { serve(w, r, h.svc.Occupancy) }

// IncomeExpense serves GET /v1/tenant/reports/income-expense.
func (h *Handler) IncomeExpense(w http.ResponseWriter, r *http.Request) {
	serve(w, r, h.svc.IncomeExpense)
}

// Channels serves GET /v1/tenant/reports/channels.
func (h *Handler) Channels(w http.ResponseWriter, r *http.Request) { serve(w, r, h.svc.Channels) }

// Aging serves GET /v1/tenant/reports/aging.
func (h *Handler) Aging(w http.ResponseWriter, r *http.Request) { serve(w, r, h.svc.Aging) }

// Branches serves GET /v1/tenant/reports/branches.
func (h *Handler) Branches(w http.ResponseWriter, r *http.Request) { serve(w, r, h.svc.Branches) }
