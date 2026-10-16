package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
)

// TransitionNotifier hears every status change inside its transaction
// (q): the notification engine (TEC-733) records the customer message
// with the transition, so a rolled-back change never notifies anyone and
// a committed one always does.
type TransitionNotifier interface {
	ReservationChanged(ctx context.Context, q *db.Queries, orgID, reservationID int64, action, from, to string) error
}

// SetTransitionNotifier wires the notifier (nil = none).
func (s *Service) SetTransitionNotifier(n TransitionNotifier) { s.notifier = n }

func (s *Service) notifyTransition(ctx context.Context, q *db.Queries, orgID int64, t transition) error {
	if s.notifier == nil {
		return nil
	}
	return s.notifier.ReservationChanged(ctx, q, orgID, t.id, t.action, t.from, t.to)
}
