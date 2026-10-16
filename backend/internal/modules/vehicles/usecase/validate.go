package usecase

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	plateRE   = regexp.MustCompile(`^[A-Z0-9][A-Z0-9 -]{0,18}[A-Z0-9]$`)
	plateKeyR = regexp.MustCompile(`[^A-Za-z0-9]`)
	vinRE     = regexp.MustCompile(`^[A-HJ-NPR-Z0-9]{17}$`)
	poolRE    = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,31}$`)
	spacesRE  = regexp.MustCompile(`\s+`)
)

// NormalizePlate upper-cases a plate and collapses its whitespace
// ("34 abc  123" → "34 ABC 123").
func NormalizePlate(v string) string {
	return strings.ToUpper(spacesRE.ReplaceAllString(strings.TrimSpace(v), " "))
}

// plateKey is the uniqueness / search key of a plate (letters and digits,
// upper-case), the same expression as the generated vehicles.plate_key.
func plateKey(v string) string {
	return strings.ToUpper(plateKeyR.ReplaceAllString(v, ""))
}

// applyInput validates in and writes it into f. current is nil on create.
func (s *Service) applyInput(ctx context.Context, orgID int64, f *repository.VehicleFields, in model.VehicleInput, current *repository.VehicleRow) error {
	create := current == nil
	ve := &ValidationError{}

	if in.Plate != nil || create {
		plate := ""
		if in.Plate != nil {
			plate = NormalizePlate(*in.Plate)
		}
		switch {
		case plate == "":
			ve.add("plate", "required", "plate is required")
		case !plateRE.MatchString(plate) || len(plateKey(plate)) < 2:
			ve.add("plate", "invalid", "plate must be 2-20 characters: letters, digits, spaces or -")
		default:
			f.Plate = plate
		}
	}
	if in.VIN != nil {
		vin := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(*in.VIN), " ", ""))
		if vin != "" && !vinRE.MatchString(vin) {
			ve.add("vin", "invalid", "vin must be 17 characters (letters except I, O, Q and digits)")
		} else {
			f.VIN = vin
		}
	}

	s.applyCatalog(ctx, ve, f, in, current)

	if in.ModelYear != nil || create {
		year := int16(0)
		if in.ModelYear != nil {
			year = *in.ModelYear
		}
		maxYear := int16(s.now().Year() + 1)
		switch {
		case year == 0:
			ve.add("model_year", "required", "model_year is required")
		case year < 1950 || year > maxYear:
			ve.add("model_year", "out_of_range", fmt.Sprintf("model_year must be between 1950 and %d", maxYear))
		default:
			f.ModelYear = year
		}
	}

	applyEnum(ve, "transmission", in.Transmission, model.Transmissions, &f.Transmission)
	applyEnum(ve, "fuel", in.Fuel, model.Fuels, &f.Fuel)
	applyEnum(ve, "body_type", in.BodyType, model.BodyTypes, &f.BodyType)
	applyInt2(ve, "seats", in.Seats, 1, 60, &f.Seats)
	applyInt2(ve, "luggage", in.Luggage, 0, 30, &f.Luggage)
	if in.AirConditioning != nil {
		f.AirConditioning = *in.AirConditioning
	}
	if in.AttributesVerified != nil {
		f.AttributesVerified = *in.AttributesVerified
	}
	applyText(ve, "color", in.Color, 64, &f.Color)

	s.applyBranches(ctx, orgID, ve, f, in, current)

	if in.FleetPool != nil {
		pool := strings.ToUpper(strings.TrimSpace(*in.FleetPool))
		switch {
		case pool == "":
			f.FleetPool = model.DefaultPool
		case !poolRE.MatchString(pool):
			ve.add("fleet_pool", "invalid", "fleet_pool must be 1-32 characters: letters, digits, - or _")
		default:
			f.FleetPool = pool
		}
	}
	if in.Ownership != nil {
		v := strings.TrimSpace(*in.Ownership)
		if !model.Contains(model.Ownerships, v) {
			ve.add("ownership", "invalid", "ownership must be one of owned, supplied")
		} else {
			f.Ownership = v
		}
	}
	applyText(ve, "supplier_name", in.SupplierName, 255, &f.SupplierName)
	switch {
	case in.SupplierUUID.Set && in.SupplierUUID.Value == nil:
		f.SupplierID = pgtype.Int8{}
	case in.SupplierUUID.Set:
		sp, err := s.repo.GetSupplier(ctx, orgID, *in.SupplierUUID.Value)
		if err != nil {
			ve.add("supplier_uuid", "not_found", "supplier was not found")
		} else {
			f.SupplierID, f.SupplierName = pgtype.Int8{Int64: sp.ID, Valid: true}, sp.Title
		}
	case in.SupplierName != nil:
		// A free-text name links the supplier card with that title (TEC-721).
		f.SupplierID = s.repo.SupplierByTitle(ctx, orgID, f.SupplierName)
	}
	if f.Ownership == model.OwnershipOwned {
		f.SupplierName = ""
		f.SupplierID = pgtype.Int8{}
	} else if f.Ownership == model.OwnershipSupplied && f.SupplierName == "" && !ve.has("ownership") {
		ve.add("supplier_name", "required", "supplier_name is required for supplied vehicles")
	}

	applyDate(ve, "in_service_date", in.InServiceDate, &f.InServiceDate)
	applyDate(ve, "out_of_service_date", in.OutOfServiceDate, &f.OutOfServiceDate)
	if f.InServiceDate.Valid && f.OutOfServiceDate.Valid && f.OutOfServiceDate.Time.Before(f.InServiceDate.Time) &&
		!ve.has("in_service_date") && !ve.has("out_of_service_date") {
		ve.add("out_of_service_date", "before_start", "out_of_service_date must be on or after in_service_date")
	}

	if in.OdometerKm != nil {
		if *in.OdometerKm < 0 || *in.OdometerKm > 5000000 {
			ve.add("odometer_km", "out_of_range", "odometer_km must be between 0 and 5000000")
		} else {
			f.OdometerKm = *in.OdometerKm
		}
	}
	applyInt2(ve, "fuel_level", in.FuelLevel, 0, 100, &f.FuelLevel)
	if in.Status != nil {
		v := strings.TrimSpace(*in.Status)
		if !model.Contains(model.Statuses, v) {
			ve.add("status", "invalid", "status must be one of "+strings.Join(model.Statuses, ", "))
		} else {
			f.Status = v
		}
	}
	if in.IsActive != nil {
		f.IsActive = *in.IsActive
	}
	return ve.orNil()
}

// applyCatalog resolves the catalog type or the manual entry and checks the
// model year against the type's valid years.
func (s *Service) applyCatalog(ctx context.Context, ve *ValidationError, f *repository.VehicleFields, in model.VehicleInput, current *repository.VehicleRow) {
	create := current == nil
	if in.ManualEntry != nil {
		f.ManualEntry = *in.ManualEntry
	}
	if f.ManualEntry {
		f.CatalogTypeID = pgtype.Int8{}
		applyText(ve, "manual_brand", in.ManualBrand, 128, &f.ManualBrand)
		applyText(ve, "manual_model", in.ManualModel, 128, &f.ManualModel)
		applyText(ve, "manual_type", in.ManualType, 255, &f.ManualType)
		if f.ManualBrand == "" && !ve.has("manual_brand") {
			ve.add("manual_brand", "required", "manual_brand is required for a manual entry")
		}
		if f.ManualModel == "" && !ve.has("manual_model") {
			ve.add("manual_model", "required", "manual_model is required for a manual entry")
		}
		return
	}
	f.ManualBrand, f.ManualModel, f.ManualType = "", "", ""
	var years []int32
	switch {
	case in.CatalogTypeUUID.Set && in.CatalogTypeUUID.Value != nil:
		t, err := s.repo.GetCatalogType(ctx, *in.CatalogTypeUUID.Value)
		sameType := current != nil && current.Vehicle.CatalogTypeID.Valid && current.Vehicle.CatalogTypeID.Int64 == t.ID
		if err != nil || (!t.IsActive && !sameType) {
			ve.add("catalog_type_uuid", "not_found", "catalog type was not found")
			return
		}
		f.CatalogTypeID = pgtype.Int8{Int64: t.ID, Valid: true}
		years = t.Years
		// Attributes the catalog knows fill the empty ones (create only).
		if create {
			if !in.Transmission.Set && t.SuggestedTransmission.Valid {
				f.Transmission = t.SuggestedTransmission
			}
			if !in.Fuel.Set && t.SuggestedFuel.Valid {
				f.Fuel = t.SuggestedFuel
			}
		}
	case f.CatalogTypeID.Valid && current != nil && !in.CatalogTypeUUID.Set:
		years = current.TypeYears
	default:
		ve.add("catalog_type_uuid", "required", "catalog_type_uuid is required unless manual_entry is set")
		return
	}
	year := f.ModelYear
	if in.ModelYear != nil {
		year = *in.ModelYear
	}
	if year != 0 && len(years) > 0 && !slices.Contains(years, int32(year)) {
		ve.add("model_year", "invalid_year", fmt.Sprintf("model_year %d is not a valid year of the catalog type", year))
	}
}

// applyBranches resolves the home / current branch. The (new) home branch
// must be in the caller's scope; the current branch may be any branch of the
// organization. An unknown branch and one outside the scope answer the same.
func (s *Service) applyBranches(ctx context.Context, orgID int64, ve *ValidationError, f *repository.VehicleFields, in model.VehicleInput, current *repository.VehicleRow) {
	create := current == nil
	if in.HomeBranchUUID != nil {
		b, err := s.repo.GetBranch(ctx, orgID, *in.HomeBranchUUID)
		changed := current == nil || current.Vehicle.HomeBranchID != b.ID
		switch {
		case err != nil:
			ve.add("home_branch_uuid", "not_found", "branch was not found")
		case changed && orgctx.CheckBranch(ctx, b.ID) != nil:
			ve.add("home_branch_uuid", "not_found", "branch was not found")
		default:
			f.HomeBranchID = b.ID
		}
	} else if create {
		ve.add("home_branch_uuid", "required", "home_branch_uuid is required")
	}
	if in.CurrentBranchUUID != nil {
		b, err := s.repo.GetBranch(ctx, orgID, *in.CurrentBranchUUID)
		if err != nil {
			ve.add("current_branch_uuid", "not_found", "branch was not found")
		} else {
			f.CurrentBranchID = b.ID
		}
	} else if create {
		f.CurrentBranchID = f.HomeBranchID
	}
}

func applyEnum(ve *ValidationError, field string, in model.OptionalString, allowed []string, dst *pgtype.Text) {
	if !in.Set {
		return
	}
	if in.Value == nil || strings.TrimSpace(*in.Value) == "" {
		*dst = pgtype.Text{}
		return
	}
	v := strings.TrimSpace(*in.Value)
	if !model.Contains(allowed, v) {
		ve.add(field, "invalid", field+" must be one of "+strings.Join(allowed, ", "))
		return
	}
	*dst = pgtype.Text{String: v, Valid: true}
}

func applyInt2(ve *ValidationError, field string, in model.OptionalInt, lo, hi int64, dst *pgtype.Int2) {
	if !in.Set {
		return
	}
	if in.Value == nil {
		*dst = pgtype.Int2{}
		return
	}
	if *in.Value < lo || *in.Value > hi {
		ve.add(field, "out_of_range", fmt.Sprintf("%s must be between %d and %d", field, lo, hi))
		return
	}
	*dst = pgtype.Int2{Int16: int16(*in.Value), Valid: true}
}

func applyText(ve *ValidationError, field string, in *string, maxLen int, dst *string) {
	if in == nil {
		return
	}
	v := strings.TrimSpace(*in)
	if utf8.RuneCountInString(v) > maxLen {
		ve.add(field, "too_long", fmt.Sprintf("%s must be at most %d characters", field, maxLen))
		return
	}
	*dst = v
}

func applyDate(ve *ValidationError, field string, in model.OptionalString, dst *pgtype.Date) {
	if !in.Set {
		return
	}
	if in.Value == nil || strings.TrimSpace(*in.Value) == "" {
		*dst = pgtype.Date{}
		return
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(*in.Value))
	if err != nil {
		ve.add(field, "invalid", field+" must be yyyy-MM-dd")
		return
	}
	*dst = pgtype.Date{Time: t, Valid: true}
}

func fieldsFromVehicle(v db.Vehicle) repository.VehicleFields {
	return repository.VehicleFields{
		Plate: v.Plate, VIN: v.Vin, CatalogTypeID: v.CatalogTypeID, ManualEntry: v.ManualEntry,
		ManualBrand: v.ManualBrand, ManualModel: v.ManualModel, ManualType: v.ManualType, ModelYear: v.ModelYear,
		Transmission: v.Transmission, Fuel: v.Fuel, BodyType: v.BodyType, Seats: v.Seats, Luggage: v.Luggage,
		AirConditioning: v.AirConditioning, AttributesVerified: v.AttributesVerified, Color: v.Color,
		HomeBranchID: v.HomeBranchID, CurrentBranchID: v.CurrentBranchID, FleetPool: v.FleetPool,
		Ownership: v.Ownership, SupplierName: v.SupplierName, SupplierID: v.SupplierID, InServiceDate: v.InServiceDate,
		OutOfServiceDate: v.OutOfServiceDate, OdometerKm: v.OdometerKm, FuelLevel: v.FuelLevel,
		Status: v.Status, IsActive: v.IsActive,
	}
}
