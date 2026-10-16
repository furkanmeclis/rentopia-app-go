package usecase

import (
	"context"
	"errors"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerportal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PrincipalForReference is the renter principal of one reservation for a
// caller that verified the customer another way: the public booking API
// (TEC-728) checks the reservation access token handed to the booker. It
// never creates a portal session; requests it sends carry no session and
// label names the source in the masked contact field ("API", "Web sitesi").
// The reservation's own views and actions (Reservation, CreateRequest, Pay)
// then apply unchanged, scoped to that renter.
func (s *Service) PrincipalForReference(ctx context.Context, orgSlug, reference, label string) (Principal, uuid.UUID, error) {
	org, err := s.organization(ctx, orgSlug)
	if err != nil {
		return Principal{}, uuid.Nil, err
	}
	ref := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(reference), " ", ""))
	if ref == "" || len(ref) > 64 {
		return Principal{}, uuid.Nil, ErrNotFound
	}
	row, err := s.q.GetPortalReservationByReference(ctx, db.GetPortalReservationByReferenceParams{OrganizationID: org.ID, Reference: ref})
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, uuid.Nil, ErrNotFound
	}
	if err != nil {
		return Principal{}, uuid.Nil, err
	}
	if len([]rune(label)) > 64 {
		label = string([]rune(label)[:64])
	}
	return Principal{
		Org: org, CustomerID: row.CustomerID, Role: model.RoleRenter, ContactMasked: label, Channel: "api",
	}, row.Uuid, nil
}
