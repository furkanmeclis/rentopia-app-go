// Package usecase is support access (TEC-678): an organization opens a
// time-boxed grant (read_only or full) that lets platform support staff
// enter it through impersonation, lists its grants and revokes them at any
// time. The guard (supportaccess.GuardResolver) and the impersonation gate
// (TargetHasSupportAccess) read the active grant at request time, so an
// expired or revoked grant stops access on the next request.
package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/supportaccess/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/apiquery"
	"github.com/google/uuid"
)

// ErrNotFound: unknown grant in the organization, or no longer active
// (revoke of an expired / revoked grant).
var ErrNotFound = errors.New("support grant not found")

// Repository is the persistence port.
type Repository interface {
	Open(ctx context.Context, orgID, actorID int64, scope string, expiresAt time.Time, note string, now time.Time) (int64, uuid.UUID, error)
	Revoke(ctx context.Context, orgID, actorID int64, id uuid.UUID, now time.Time) error
	Get(ctx context.Context, orgID int64, id uuid.UUID, now time.Time) (model.Grant, error)
	List(ctx context.Context, orgID int64, f model.ListFilter, now time.Time) ([]model.Grant, int64, error)
	Active(ctx context.Context, orgID int64, now time.Time) (model.Grant, error)
	UserSummary(ctx context.Context, userID int64, now time.Time) (memberships, granted int64, err error)
	ListActivePlatform(ctx context.Context, now time.Time) ([]model.PlatformGrant, error)
}

// Service implements support access.
type Service struct {
	repo Repository
	now  func() time.Time
}

// New builds the service.
func New(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func notFound(err error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// Validate checks an open request.
func Validate(in model.OpenInput) error {
	var details []apiquery.Detail
	if in.Scope != model.ScopeReadOnly && in.Scope != model.ScopeFull {
		details = append(details, apiquery.Detail{Field: "scope", Message: "scope must be read_only or full", Code: "invalid"})
	}
	if in.DurationHours < model.MinDurationHours || in.DurationHours > model.MaxDurationHours {
		details = append(details, apiquery.Detail{Field: "duration_hours", Message: "duration must be between 1 hour and 7 days", Code: "out_of_range"})
	}
	if utf8.RuneCountInString(strings.TrimSpace(in.Note)) > model.MaxNoteLength {
		details = append(details, apiquery.Detail{Field: "note", Message: "note is too long", Code: "too_long"})
	}
	if len(details) > 0 {
		return &apiquery.ValidationError{Details: details}
	}
	return nil
}

// Open stores a new grant for the organization; it replaces the active one.
func (s *Service) Open(ctx context.Context, orgID, actorID int64, in model.OpenInput) (model.Grant, error) {
	if err := Validate(in); err != nil {
		return model.Grant{}, err
	}
	now := s.now()
	expires := now.Add(time.Duration(in.DurationHours) * time.Hour)
	_, id, err := s.repo.Open(ctx, orgID, actorID, in.Scope, expires, strings.TrimSpace(in.Note), now)
	if err != nil {
		return model.Grant{}, err
	}
	g, err := s.repo.Get(ctx, orgID, id, now)
	return g, notFound(err)
}

// Revoke closes an active grant of the organization.
func (s *Service) Revoke(ctx context.Context, orgID, actorID int64, id uuid.UUID) (model.Grant, error) {
	now := s.now()
	if err := s.repo.Revoke(ctx, orgID, actorID, id, now); err != nil {
		return model.Grant{}, notFound(err)
	}
	g, err := s.repo.Get(ctx, orgID, id, now)
	return g, notFound(err)
}

// List pages the organization's grants (statuses: active, expired, revoked).
func (s *Service) List(ctx context.Context, orgID int64, f model.ListFilter) ([]model.Grant, int64, error) {
	for _, st := range f.Statuses {
		switch st {
		case model.StatusActive, model.StatusExpired, model.StatusRevoked:
		default:
			return nil, 0, &apiquery.ValidationError{Details: []apiquery.Detail{{Field: "status", Message: "unknown status " + st, Code: "invalid"}}}
		}
	}
	return s.repo.List(ctx, orgID, f, s.now())
}

// ActiveGrant implements the guard lookup: the organization's active grant
// as an orgctx.SupportGrant, or nil when there is none.
func (s *Service) ActiveGrant(ctx context.Context, orgID int64) (*orgctx.SupportGrant, error) {
	g, err := s.repo.Active(ctx, orgID, s.now())
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &orgctx.SupportGrant{ID: g.ID, UUID: g.UUID, Scope: g.Scope, ExpiresAt: g.ExpiresAt}, nil
}

// TargetHasSupportAccess is the impersonation gate: a user who is a member
// of at least one organization may be impersonated only while one of those
// organizations has an active grant. A user without memberships enters no
// organization, so nothing has to be granted. No super admin bypass.
func (s *Service) TargetHasSupportAccess(ctx context.Context, userID int64) (bool, error) {
	memberships, granted, err := s.repo.UserSummary(ctx, userID, s.now())
	if err != nil {
		return false, err
	}
	return memberships == 0 || granted > 0, nil
}

// ListActivePlatform lists the active grants of every organization.
func (s *Service) ListActivePlatform(ctx context.Context) ([]model.PlatformGrant, error) {
	return s.repo.ListActivePlatform(ctx, s.now())
}
