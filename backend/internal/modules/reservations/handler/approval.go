package handler

import (
	"net/http"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/response"
	"github.com/google/uuid"
)

// Accept serves POST …/{uuid}/accept (staff approval queue, TEC-708).
func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "uuid")
	if !ok {
		return
	}
	var in model.AcceptInput
	if !decodeJSON(w, r, &in, true) {
		return
	}
	out, err := h.svc.Accept(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, r, err)
		return
	}
	payload := map[string]any{"status": out.Reservation.Status, "send_payment_link": in.SendPaymentLink}
	if out.PaymentLink != nil {
		payload["payment_link_uuid"] = out.PaymentLink.UUID
	}
	h.audit(r, model.ResourceReservations, "reservations.accepted", id, payload)
	response.JSON(w, r, http.StatusOK, out)
}

// Reject serves POST …/{uuid}/reject (reason required).
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	h.reasonAction(w, r, "rejected", func(r *http.Request, id uuid.UUID, reason string) (model.Reservation, error) {
		return h.svc.Reject(r.Context(), orgID(r), id, reason)
	})
}
