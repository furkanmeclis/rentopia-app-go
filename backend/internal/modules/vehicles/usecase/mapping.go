package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/vehicles/repository"
	pkgmoney "github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// hydrate maps rows and loads their rental group memberships.
func (s *Service) hydrate(ctx context.Context, orgID int64, rows []repository.VehicleRow) ([]model.Vehicle, error) {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Vehicle.ID)
	}
	groups, err := s.repo.MembershipsByVehicle(ctx, orgID, ids)
	if err != nil {
		return nil, err
	}
	today := s.now()
	out := make([]model.Vehicle, 0, len(rows))
	for _, row := range rows {
		v := MapVehicle(row, today)
		for _, g := range groups[row.Vehicle.ID] {
			v.RentalGroups = append(v.RentalGroups, membershipRef(g))
		}
		out = append(out, v)
	}
	return out, nil
}

// MapVehicle converts a joined row to the API shape. today decides
// in_service.
func MapVehicle(row repository.VehicleRow, today time.Time) model.Vehicle {
	v := row.Vehicle
	out := model.Vehicle{
		UUID: v.Uuid, Plate: v.Plate, VIN: v.Vin, ManualEntry: v.ManualEntry, Catalog: catalogRef(row),
		ModelYear: v.ModelYear, Transmission: textPtr(v.Transmission), Fuel: textPtr(v.Fuel),
		BodyType: textPtr(v.BodyType), Seats: int2Ptr(v.Seats), Luggage: int2Ptr(v.Luggage),
		AirConditioning: v.AirConditioning, AttributesVerified: v.AttributesVerified, Color: v.Color,
		HomeBranch:    model.BranchRef{UUID: row.HomeBranchUuid, Code: row.HomeBranchCode, Name: row.HomeBranchName},
		CurrentBranch: model.BranchRef{UUID: row.CurrentBranchUuid, Code: row.CurrentBranchCode, Name: row.CurrentBranchName},
		FleetPool:     v.FleetPool, Ownership: v.Ownership, SupplierName: v.SupplierName,
		InServiceDate: datePtr(v.InServiceDate), OutOfServiceDate: datePtr(v.OutOfServiceDate),
		OdometerKm: v.OdometerKm, FuelLevel: int2Ptr(v.FuelLevel), Status: v.Status,
		StatusChangedAt: v.StatusChangedAt.Time.UTC(), RentalGroups: []model.GroupRef{},
		IsActive: v.IsActive, CreatedAt: v.CreatedAt.Time.UTC(), UpdatedAt: v.UpdatedAt.Time.UTC(),
	}
	if v.ManualEntry {
		out.Brand, out.Model, out.TypeName = v.ManualBrand, v.ManualModel, v.ManualType
	} else if out.Catalog != nil {
		out.Brand, out.Model, out.TypeName = out.Catalog.BrandName, out.Catalog.ModelName, out.Catalog.TypeName
	}
	out.Label = joinNonEmpty(out.Brand, out.Model, out.TypeName)
	if row.KaskoValue.Valid && row.KaskoPeriod != "" {
		value := FormatMoney(row.KaskoValue)
		period := row.KaskoPeriod
		out.KaskoValue, out.KaskoPeriod = &value, &period
	}
	out.InService = inService(v, today)
	day := model.Today(today)
	out.InspectionUntil = datePtr(row.InspectionUntil)
	out.InsuranceUntil = datePtr(row.InsuranceUntil)
	out.KaskoUntil = datePtr(row.KaskoUntil)
	out.DocumentExpiresAt = datePtr(row.DocumentExpiresAt)
	out.DocumentStatus = model.DocumentState(timePtr(row.DocumentExpiresAt), day)
	out.NextMaintenanceDate = datePtr(row.NextMaintenanceDate)
	var nextKm *int32
	if row.NextMaintenanceKm >= 0 {
		km := row.NextMaintenanceKm
		nextKm = &km
	}
	out.NextMaintenanceKm = nextKm
	out.MaintenanceStatus = model.MaintenanceState(timePtr(row.NextMaintenanceDate), nextKm, v.OdometerKm, day)
	return out
}

func timePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}

// inService: active and today within [in_service_date, out_of_service_date].
func inService(v db.Vehicle, today time.Time) bool {
	if !v.IsActive {
		return false
	}
	day := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	if v.InServiceDate.Valid && day.Before(v.InServiceDate.Time) {
		return false
	}
	if v.OutOfServiceDate.Valid && day.After(v.OutOfServiceDate.Time) {
		return false
	}
	return true
}

func catalogRef(row repository.VehicleRow) *model.CatalogRef {
	if !row.TypeUuid.Valid {
		return nil
	}
	ref := &model.CatalogRef{
		TypeUUID: row.TypeUuid.Bytes, TypeName: row.TypeName.String, TypeCode: row.TypeCode.String,
		ModelName: row.ModelName.String, BrandName: row.BrandName.String, Years: row.TypeYears,
	}
	if ref.Years == nil {
		ref.Years = []int32{}
	}
	if row.ModelGroupID != 0 {
		id := row.ModelGroupUuid
		ref.ModelUUID = &id
	}
	if row.BrandUuid.Valid {
		id := uuid.UUID(row.BrandUuid.Bytes)
		ref.BrandUUID = &id
	}
	return ref
}

func membershipRef(g db.ListVehicleGroupsByVehicleIDsRow) model.GroupRef {
	return model.GroupRef{
		UUID: g.Uuid, Code: g.Code, Name: g.Name, IsActive: g.IsActive,
		Segment: model.SegmentRef{UUID: g.SegmentUuid, Code: g.SegmentCode, Name: g.SegmentName},
	}
}

func groupRef(g repository.GroupRow) model.GroupRef {
	return model.GroupRef{
		UUID: g.Uuid, Code: g.Code, Name: g.Name, IsActive: g.IsActive,
		Segment: model.SegmentRef{UUID: g.SegmentUuid, Code: g.SegmentCode, Name: g.SegmentName},
	}
}

func joinNonEmpty(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func int2Ptr(v pgtype.Int2) *int16 {
	if !v.Valid {
		return nil
	}
	n := v.Int16
	return &n
}

func int4Ptr(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}

func datePtr(d pgtype.Date) *string {
	if !d.Valid {
		return nil
	}
	v := d.Time.Format("2006-01-02")
	return &v
}

// FormatMoney renders a numeric as a decimal string with two fraction digits
// (pkg/money: one rounding rule for every amount).
func FormatMoney(n pgtype.Numeric) string {
	return pkgmoney.FormatNumeric(n)
}
