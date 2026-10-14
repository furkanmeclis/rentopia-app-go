// Package usecase holds the rules of vehicle documents, maintenance, expiry
// alerts and the daily reminder job (TEC-693).
//
// Every one-vehicle method resolves the vehicle through the vehicles use
// case (Lookup): another organization's vehicle and a vehicle outside the
// caller's branch scope (home and current branch both out) read as not
// found, exactly like GET /v1/tenant/vehicles/{uuid}. Org-wide reads
// (alerts, export) apply the same branch scope in SQL.
package usecase

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	notifmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehiclerecords/repository"
	vehiclemodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	vehicleusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/authctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Caps of the one-vehicle lists, the alerts view and exports.
const (
	MaxVehicleItems = 500
	MaxAlerts       = 2000
	MaxExportRows   = 10000
)

var (
	ErrVehicleNotFound     = errors.New("vehicle not found")
	ErrDocumentNotFound    = errors.New("vehicle document not found")
	ErrMaintenanceNotFound = errors.New("maintenance item not found")
	ErrNoFile              = errors.New("the document has no file")
	ErrStorageUnavailable  = errors.New("object storage is not configured")
)

// Detail is one field-level validation problem.
type Detail struct {
	Field   string
	Message string
	Code    string
}

// ValidationError carries field details; handlers answer 400.
type ValidationError struct {
	Details []Detail
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Details) == 0 {
		return "validation failed"
	}
	return e.Details[0].Field + ": " + e.Details[0].Message
}

func (e *ValidationError) add(field, code, msg string) {
	e.Details = append(e.Details, Detail{Field: field, Code: code, Message: msg})
}

func (e *ValidationError) orNil() error {
	if len(e.Details) == 0 {
		return nil
	}
	return e
}

func validation(field, code, msg string) error {
	return &ValidationError{Details: []Detail{{Field: field, Code: code, Message: msg}}}
}

// VehicleLookup resolves a vehicle in the caller's branch scope (the
// vehicles use case).
type VehicleLookup interface {
	Lookup(ctx context.Context, orgID int64, id uuid.UUID) (db.Vehicle, error)
}

// Notifier enqueues notifications (the notifications use case).
type Notifier interface {
	Enqueue(ctx context.Context, in notifmodel.EnqueueInput) ([]notifmodel.Notification, error)
}

// Service implements the use cases.
type Service struct {
	repo     *repository.Postgres
	vehicles VehicleLookup
	storage  storage.Driver
	notifier Notifier
	log      *slog.Logger
	now      func() time.Time
}

// New builds the service. store and notifier may be nil (uploads answer
// ErrStorageUnavailable; reminders are skipped).
func New(repo *repository.Postgres, vehicles VehicleLookup, store storage.Driver, notifier Notifier, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{repo: repo, vehicles: vehicles, storage: store, notifier: notifier, log: log, now: time.Now}
}

// SetClock replaces the clock (tests).
func (s *Service) SetClock(now func() time.Time) {
	s.now = now
}

// today is the operating calendar day (Türkiye).
func (s *Service) today() time.Time {
	return vehiclemodel.Today(s.now())
}

// vehicle resolves the vehicle of a sub-resource request.
func (s *Service) vehicle(ctx context.Context, orgID int64, id uuid.UUID) (db.Vehicle, error) {
	v, err := s.vehicles.Lookup(ctx, orgID, id)
	if errors.Is(err, vehicleusecase.ErrNotFound) {
		return db.Vehicle{}, ErrVehicleNotFound
	}
	return v, err
}

// actorArg is the signed-in user of a request context (null in workers).
func actorArg(ctx context.Context) pgtype.Int8 {
	if p, ok := authctx.PrincipalFrom(ctx); ok && p.UserInternal > 0 {
		return pgtype.Int8{Int64: p.UserInternal, Valid: true}
	}
	return pgtype.Int8{}
}

func mapNotFound(err, target error) error {
	if errors.Is(err, repository.ErrNotFound) {
		return target
	}
	return err
}

// --- field helpers ------------------------------------------------------------------

// parseDate parses a YYYY-MM-DD date.
func parseDate(v string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(v))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// applyDate applies an optional date field (null clears it).
func applyDate(ve *ValidationError, field string, in vehiclemodel.OptionalString, dst *pgtype.Date) {
	if !in.Set {
		return
	}
	if in.Value == nil || strings.TrimSpace(*in.Value) == "" {
		*dst = pgtype.Date{}
		return
	}
	t, ok := parseDate(*in.Value)
	if !ok {
		ve.add(field, "invalid_date", field+" must be a date (YYYY-MM-DD)")
		return
	}
	*dst = pgtype.Date{Time: t, Valid: true}
}

// applyKm applies an optional km field (null clears it).
func applyKm(ve *ValidationError, field string, in vehiclemodel.OptionalInt, dst *pgtype.Int4) {
	if !in.Set {
		return
	}
	if in.Value == nil {
		*dst = pgtype.Int4{}
		return
	}
	if *in.Value < 0 || *in.Value > 5000000 {
		ve.add(field, "out_of_range", field+" must be between 0 and 5000000")
		return
	}
	*dst = pgtype.Int4{Int32: int32(*in.Value), Valid: true}
}

// applyText trims an optional text field and checks its length.
func applyText(ve *ValidationError, field string, in *string, max int, dst *string) {
	if in == nil {
		return
	}
	v := strings.TrimSpace(*in)
	if utf8.RuneCountInString(v) > max {
		ve.add(field, "too_long", field+" is too long")
		return
	}
	*dst = v
}

var moneyRE = regexp.MustCompile(`^\d{1,12}(\.\d{1,2})?$`)

// applyCost applies an optional TRY amount ("1250.50"; null clears it).
func applyCost(ve *ValidationError, in vehiclemodel.OptionalString, dst *pgtype.Numeric) {
	if !in.Set {
		return
	}
	if in.Value == nil || strings.TrimSpace(*in.Value) == "" {
		*dst = pgtype.Numeric{}
		return
	}
	v := strings.ReplaceAll(strings.TrimSpace(*in.Value), ",", ".")
	if !moneyRE.MatchString(v) {
		ve.add("cost", "invalid", "cost must be a non-negative amount with at most 2 decimals")
		return
	}
	var n pgtype.Numeric
	if err := n.Scan(v); err != nil {
		ve.add("cost", "invalid", "cost must be a non-negative amount with at most 2 decimals")
		return
	}
	*dst = n
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	v := d.Time.Format("2006-01-02")
	return &v
}

func timePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}
