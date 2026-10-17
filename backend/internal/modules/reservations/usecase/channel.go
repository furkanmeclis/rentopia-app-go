package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// SetCommission records the channel's current commission on a reservation
// collected by its sales channel (channel manager, TEC-736): a modified
// channel booking may carry a new commission. The net receivable follows
// the current total. Reservations collected by the organization are left
// as they are (ErrNotFound).
func (s *Service) SetCommission(ctx context.Context, orgID int64, id uuid.UUID, commission money.Money) (model.Reservation, error) {
	c, err := s.core(ctx, orgID, id)
	if err != nil {
		return model.Reservation{}, err
	}
	if commission.IsNegative() {
		return model.Reservation{}, validation("commission_amount", "invalid", "commission must be a non-negative amount")
	}
	n, err := s.q.SetReservationCommission(ctx, db.SetReservationCommissionParams{
		OrganizationID: orgID, ID: c.ID, CommissionAmount: commission.Numeric(),
	})
	if err != nil {
		return model.Reservation{}, err
	}
	if n != 1 {
		return model.Reservation{}, ErrNotFound
	}
	s.publish(ctx, model.ResourceReservations, "updated", id)
	return s.Get(ctx, orgID, id)
}
