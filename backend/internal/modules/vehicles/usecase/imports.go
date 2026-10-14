package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	"github.com/google/uuid"
)

// ImportInput is one row of a fleet import (the İdes fleet XLSX columns:
// plaka, şasi, marka, model, tip / kasko kodu, model yılı, renk, vites,
// yakıt, şube, mülkiyet, km). Empty cells keep the current value of an
// existing vehicle.
type ImportInput struct {
	Plate        string
	VIN          string
	Brand        string
	Model        string
	TypeName     string
	TypeCode     string
	ModelYear    string
	Color        string
	Transmission string
	Fuel         string
	Branch       string
	Ownership    string
	SupplierName string
	Pool         string
	OdometerKm   string
}

// ImportChange is one field an import row would change.
type ImportChange struct {
	Field string
	From  string
	To    string
}

// ImportPreview is the dry-run result of one import row.
type ImportPreview struct {
	// Op is "create" or "update".
	Op      string
	Changes []ImportChange
	// Catalog tells how the vehicle was matched: "type_code", "names" or
	// "manual" ("" when the row has no catalog columns).
	Catalog string
}

// importPlan is a row resolved against the catalog, the branches and the
// existing vehicle with the same plate.
type importPlan struct {
	input        model.VehicleInput
	existing     *repository.VehicleRow
	catalog      string
	catalogLabel string
	branchCode   string
}

// ImportRow creates a vehicle from an import row, or updates the vehicle
// with the same plate. It returns the vehicle uuid, whether it was created
// and the previous state of an updated vehicle (for rollback).
func (s *Service) ImportRow(ctx context.Context, orgID int64, in ImportInput) (uuid.UUID, bool, *model.Vehicle, error) {
	plan, err := s.planImport(ctx, orgID, in)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	if plan.existing != nil {
		prev := MapVehicle(*plan.existing, s.now())
		updated, err := s.Patch(ctx, orgID, plan.existing.Vehicle.Uuid, plan.input)
		if err != nil {
			return uuid.Nil, false, nil, err
		}
		return updated.UUID, false, &prev, nil
	}
	created, err := s.Create(ctx, orgID, plan.input)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	return created.UUID, true, nil, nil
}

// PreviewImport validates an import row without writing and lists what it
// would change (dry-run diff).
func (s *Service) PreviewImport(ctx context.Context, orgID int64, in ImportInput) (ImportPreview, error) {
	plan, err := s.planImport(ctx, orgID, in)
	if err != nil {
		return ImportPreview{}, err
	}
	out := ImportPreview{Op: "create", Catalog: plan.catalog, Changes: []ImportChange{}}
	var before repository.VehicleFields
	if plan.existing != nil {
		out.Op = "update"
		before = fieldsFromVehicle(plan.existing.Vehicle)
	} else {
		before = repository.VehicleFields{
			FleetPool: model.DefaultPool, Ownership: model.OwnershipOwned, Status: model.StatusAvailable,
			AirConditioning: true, IsActive: true,
		}
	}
	after := before
	if err := s.applyInput(ctx, orgID, &after, plan.input, plan.existing); err != nil {
		return ImportPreview{}, err
	}
	var exclude *uuid.UUID
	if plan.existing != nil {
		id := plan.existing.Vehicle.Uuid
		exclude = &id
	}
	if after.IsActive && (plan.existing == nil || plateKey(after.Plate) != plateKey(before.Plate)) {
		if taken, err := s.repo.PlateTaken(ctx, orgID, after.Plate, exclude); err != nil {
			return ImportPreview{}, err
		} else if taken {
			return ImportPreview{}, ErrPlateTaken
		}
	}
	add := func(field, from, to string) {
		if from != to {
			out.Changes = append(out.Changes, ImportChange{Field: field, From: from, To: to})
		}
	}
	add("plate", before.Plate, after.Plate)
	add("vin", before.VIN, after.VIN)
	if plan.catalog != "" {
		from := ""
		if plan.existing != nil {
			from = MapVehicle(*plan.existing, s.now()).Label
		}
		add("vehicle", from, plan.catalogLabel)
	}
	add("model_year", yearString(before.ModelYear), yearString(after.ModelYear))
	add("color", before.Color, after.Color)
	add("transmission", before.Transmission.String, after.Transmission.String)
	add("fuel", before.Fuel.String, after.Fuel.String)
	if plan.branchCode != "" {
		from := ""
		if plan.existing != nil {
			from = plan.existing.HomeBranchCode
		}
		add("home_branch", from, plan.branchCode)
	}
	add("ownership", before.Ownership, after.Ownership)
	add("supplier_name", before.SupplierName, after.SupplierName)
	add("fleet_pool", before.FleetPool, after.FleetPool)
	add("odometer_km", strconv.Itoa(int(before.OdometerKm)), strconv.Itoa(int(after.OdometerKm)))
	return out, nil
}

func (s *Service) planImport(ctx context.Context, orgID int64, in ImportInput) (importPlan, error) {
	ve := &ValidationError{}
	plan := importPlan{}
	plate := NormalizePlate(in.Plate)
	if plate == "" {
		return plan, validation("plate", "required", "plate is required")
	}
	plan.input.Plate = &plate
	if existing, err := s.repo.GetVehicleByPlate(ctx, orgID, plate); err == nil {
		row, err := s.getVehicle(ctx, orgID, existing.Uuid)
		if err != nil {
			// Outside the branch scope: same answer as a missing vehicle.
			return plan, err
		}
		plan.existing = &row
		// Matched by plate key: keep the stored display format.
		plan.input.Plate = nil
	} else if !errors.Is(err, repository.ErrNotFound) {
		return plan, err
	}
	create := plan.existing == nil

	var year int16
	if v := strings.TrimSpace(in.ModelYear); v != "" {
		n, err := strconv.Atoi(strings.TrimSuffix(v, ".0"))
		if err != nil {
			ve.add("model_year", "invalid", "model_year is not a year")
		} else {
			year = int16(n)
			plan.input.ModelYear = &year
		}
	} else if plan.existing != nil {
		year = plan.existing.Vehicle.ModelYear
	}

	if err := s.planCatalog(ctx, &plan, in, year); err != nil {
		var cve *ValidationError
		if !errors.As(err, &cve) {
			return plan, err
		}
		ve.Details = append(ve.Details, cve.Details...)
	}

	setText := func(src string, dst **string) {
		if v := strings.TrimSpace(src); v != "" {
			*dst = &v
		}
	}
	setText(in.VIN, &plan.input.VIN)
	setText(in.Color, &plan.input.Color)
	setText(in.Pool, &plan.input.FleetPool)
	if v := strings.TrimSpace(in.Transmission); v != "" {
		if code, ok := TransmissionCode(v); ok {
			plan.input.Transmission = model.String(code)
		} else {
			ve.add("transmission", "invalid", fmt.Sprintf("unknown transmission %q", v))
		}
	}
	if v := strings.TrimSpace(in.Fuel); v != "" {
		if code, ok := FuelCode(v); ok {
			plan.input.Fuel = model.String(code)
		} else {
			ve.add("fuel", "invalid", fmt.Sprintf("unknown fuel %q", v))
		}
	}
	if v := strings.TrimSpace(in.Branch); v != "" {
		b, err := s.repo.FindBranch(ctx, orgID, v)
		switch {
		case errors.Is(err, repository.ErrNotFound):
			ve.add("home_branch_uuid", "not_found", fmt.Sprintf("branch %q was not found", v))
		case err != nil:
			return plan, err
		default:
			plan.input.HomeBranchUUID = &b.Uuid
			plan.branchCode = b.Code
		}
	} else if create {
		ve.add("home_branch_uuid", "required", "branch is required")
	}
	if v := strings.TrimSpace(in.Ownership); v != "" {
		if code, ok := OwnershipCode(v); ok {
			plan.input.Ownership = &code
		} else {
			ve.add("ownership", "invalid", fmt.Sprintf("unknown ownership %q", v))
		}
	}
	setText(in.SupplierName, &plan.input.SupplierName)
	if v := strings.TrimSpace(in.OdometerKm); v != "" {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, strings.TrimSuffix(v, ".0"))
		n, err := strconv.Atoi(digits)
		if err != nil || digits == "" {
			ve.add("odometer_km", "invalid", "km is not a number")
		} else {
			km := int32(n)
			plan.input.OdometerKm = &km
		}
	}
	return plan, ve.orNil()
}

// planCatalog matches the catalog: TSB type code first, then display brand +
// model (+ type name), else a manual entry. A row without catalog columns
// keeps the vehicle's current catalog data.
func (s *Service) planCatalog(ctx context.Context, plan *importPlan, in ImportInput, year int16) error {
	brand, mdl := strings.TrimSpace(in.Brand), strings.TrimSpace(in.Model)
	typeName, code := strings.TrimSpace(in.TypeName), strings.TrimSpace(in.TypeCode)
	if brand == "" && mdl == "" && typeName == "" && code == "" {
		if plan.existing == nil {
			return validation("brand", "required", "brand and model (or a kasko type code) are required")
		}
		return nil
	}
	yearOK := func(years []int32) bool { return year == 0 || len(years) == 0 || slices.Contains(years, int32(year)) }
	pick := func(id uuid.UUID, label, how string) {
		manual := false
		plan.input.ManualEntry = &manual
		plan.input.CatalogTypeUUID = model.OptionalUUID{Set: true, Value: &id}
		plan.catalog, plan.catalogLabel = how, label
	}
	if code != "" {
		rows, err := s.repo.CatalogTypesByCode(ctx, code)
		if err != nil {
			return err
		}
		var fallback *uuid.UUID
		for _, row := range rows {
			if brand != "" && !strings.EqualFold(row.BrandName, brand) {
				continue
			}
			if yearOK(row.Years) {
				pick(row.Uuid, joinNonEmpty(row.BrandName, row.RawName), "type_code")
				return nil
			}
			if fallback == nil {
				id := row.Uuid
				fallback = &id
			}
		}
		if fallback != nil {
			// The type exists but not for this year: model_year validation
			// reports it.
			pick(*fallback, code, "type_code")
			return nil
		}
	}
	if brand != "" && mdl != "" {
		rows, err := s.repo.CatalogTypesByNames(ctx, brand, mdl, typeName)
		if err != nil {
			return err
		}
		candidates := rows[:0:0]
		for _, row := range rows {
			if yearOK(row.Years) {
				candidates = append(candidates, row)
			}
		}
		if typeName != "" {
			for _, row := range candidates {
				if strings.EqualFold(strings.TrimSpace(row.RawName), typeName) {
					pick(row.Uuid, joinNonEmpty(brand, mdl, row.RawName), "names")
					return nil
				}
			}
		}
		if len(candidates) == 1 {
			pick(candidates[0].Uuid, joinNonEmpty(brand, mdl, candidates[0].RawName), "names")
			return nil
		}
	}
	if brand == "" || mdl == "" {
		return validation("brand", "required", "brand and model are required when the kasko type code is unknown")
	}
	manual := true
	plan.input.ManualEntry = &manual
	plan.input.ManualBrand, plan.input.ManualModel = &brand, &mdl
	plan.input.ManualType = &typeName
	plan.input.CatalogTypeUUID = model.OptionalUUID{}
	plan.catalog, plan.catalogLabel = "manual", joinNonEmpty(brand, mdl, typeName)
	return nil
}

func yearString(y int16) string {
	if y == 0 {
		return ""
	}
	return strconv.Itoa(int(y))
}

// foldTR lower-cases with Turkish dotted / dotless i folded to ASCII.
func foldTR(v string) string {
	r := strings.NewReplacer("İ", "i", "I", "i", "ı", "i", "Ş", "s", "ş", "s", "Ğ", "g", "ğ", "g",
		"Ü", "u", "ü", "u", "Ö", "o", "ö", "o", "Ç", "c", "ç", "c")
	return strings.ToLower(strings.TrimSpace(r.Replace(v)))
}

// TransmissionCode maps an import value (Turkish or code) to a transmission.
func TransmissionCode(v string) (string, bool) {
	switch f := foldTR(v); {
	case f == "manual" || f == "manuel" || f == "duz" || f == "duz vites" || f == "mekanik":
		return "manual", true
	case f == "automatic" || strings.HasPrefix(f, "otomatik") || f == "yari otomatik" || f == "auto" ||
		f == "cvt" || f == "dsg" || f == "edc":
		return "automatic", true
	}
	return "", false
}

// FuelCode maps an import value (Turkish or code) to a fuel.
func FuelCode(v string) (string, bool) {
	switch f := foldTR(v); {
	case f == "petrol" || f == "benzin" || f == "benzinli":
		return "petrol", true
	case f == "diesel" || f == "dizel" || f == "motorin":
		return "diesel", true
	case strings.Contains(f, "hibrit") || strings.Contains(f, "hybrid"):
		return "hybrid", true
	case f == "electric" || f == "elektrik" || f == "elektrikli":
		return "electric", true
	case strings.Contains(f, "lpg"):
		return "lpg", true
	}
	return "", false
}

// OwnershipCode maps an import value (Turkish or code) to an ownership.
func OwnershipCode(v string) (string, bool) {
	switch f := foldTR(v); f {
	case "owned", "ozmal", "oz mal", "kendi", "sirket", "mulk":
		return model.OwnershipOwned, true
	case "supplied", "tedarikci", "tedarik", "kiralik", "operasyonel kiralama", "leasing":
		return model.OwnershipSupplied, true
	}
	return "", false
}
