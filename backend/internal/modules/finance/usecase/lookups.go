package usecase

import (
	"context"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/google/uuid"
)

// Natural-key lookups of the import adapters (always inside the
// organization; unknown keys answer a validation error on the field).

// BranchUUIDByCode resolves a branch code.
func (s *Service) BranchUUIDByCode(ctx context.Context, orgID int64, code string) (uuid.UUID, error) {
	b, err := s.q.GetBranchRefByCode(ctx, db.GetBranchRefByCodeParams{OrganizationID: orgID, Code: code})
	if notFound(err) {
		return uuid.Nil, validation("branch_code", "not_found", "unknown branch_code")
	}
	return b.Uuid, err
}

// CashAccountUUIDByCode resolves a cash account code.
func (s *Service) CashAccountUUIDByCode(ctx context.Context, orgID int64, code string) (uuid.UUID, error) {
	c, err := s.q.GetCashAccountByCode(ctx, db.GetCashAccountByCodeParams{OrganizationID: orgID, Code: code})
	if notFound(err) {
		return uuid.Nil, ErrCashAccountNotFound
	}
	return c.Uuid, err
}

// CategoryUUIDByCode resolves an expense category code.
func (s *Service) CategoryUUIDByCode(ctx context.Context, orgID int64, code string) (uuid.UUID, error) {
	if _, err := s.ListCategories(ctx, orgID, "", false); err != nil { // seeds the defaults on first use
		return uuid.Nil, err
	}
	c, err := s.q.GetExpenseCategoryByCode(ctx, db.GetExpenseCategoryByCodeParams{OrganizationID: orgID, Code: code})
	if notFound(err) {
		return uuid.Nil, validation("category_code", "not_found", "unknown category_code")
	}
	return c.Uuid, err
}

// VehicleUUIDByPlate resolves a plate (spaces and case ignored).
func (s *Service) VehicleUUIDByPlate(ctx context.Context, orgID int64, plate string) (uuid.UUID, error) {
	v, err := s.q.GetVehicleRefByPlate(ctx, db.GetVehicleRefByPlateParams{OrganizationID: orgID, Plate: plate})
	if notFound(err) {
		return uuid.Nil, validation("plate", "not_found", "unknown plate")
	}
	return v.Uuid, err
}

// CategoryActive reports whether id is an active category of the
// organization.
func (s *Service) CategoryActive(ctx context.Context, orgID int64, id uuid.UUID) bool {
	c, err := s.q.GetExpenseCategory(ctx, db.GetExpenseCategoryParams{OrganizationID: orgID, Uuid: id})
	return err == nil && c.IsActive
}
