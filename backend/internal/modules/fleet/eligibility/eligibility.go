// Package eligibility decides whether a physical vehicle can serve a rental
// group or a product guarantee (TEC-691). It is pure: the vehicles module
// (and later the capacity engine) loads the rows and calls these functions.
//
// Capacity is never stored; it is derived from the vehicles these checks
// accept, so product resource sets may overlap.
package eligibility

import (
	"github.com/google/uuid"
)

// Reason codes returned when a vehicle is not eligible. Stable strings:
// clients translate them.
const (
	ReasonTransmission       = "transmission_mismatch"
	ReasonFuel               = "fuel_mismatch"
	ReasonBodyType           = "body_type_mismatch"
	ReasonSeats              = "seats_below_minimum"
	ReasonLuggage            = "luggage_below_minimum"
	ReasonAirConditioning    = "air_conditioning_required"
	ReasonCatalogNotAllowed  = "catalog_not_allowed"
	ReasonVehicleMismatch    = "vehicle_mismatch"
	ReasonVehicleUnset       = "guaranteed_vehicle_not_set"
	ReasonModelMismatch      = "model_mismatch"
	ReasonTypeMismatch       = "type_mismatch"
	ReasonVariantUnset       = "guaranteed_variant_not_set"
	ReasonUnknownGuarantee   = "unknown_guarantee_type"
	ReasonGroupCriteriaUnset = "group_not_set"
)

// VehicleAttributes are the facts of one physical vehicle that guarantees
// are checked against. Catalog ids are the platform catalog row ids (0 =
// unknown). Seats / Luggage are counts; empty strings mean unknown.
type VehicleAttributes struct {
	ID              uuid.UUID
	CatalogModelID  int64
	CatalogTypeID   int64
	Transmission    string
	Fuel            string
	BodyType        string
	Seats           int
	Luggage         int
	AirConditioning bool
}

// GroupCriteria are the protected attributes and membership criteria of a
// rental group. Empty strings / zero minimums are not guaranteed; empty
// catalog lists accept any model or type.
type GroupCriteria struct {
	Transmission    string
	Fuel            string
	BodyType        string
	MinSeats        int
	MinLuggage      int
	AirConditioning bool
	// CatalogModelIDs / CatalogTypeIDs: a vehicle matches when its model is
	// in CatalogModelIDs or its type is in CatalogTypeIDs.
	CatalogModelIDs []int64
	CatalogTypeIDs  []int64
}

// EligibleForGroup reports whether vehicle has every protected attribute of
// the group and matches its catalog criteria. reasons lists every failed
// check (empty when ok).
func EligibleForGroup(vehicle VehicleAttributes, group GroupCriteria) (bool, []string) {
	reasons := []string{}
	if group.Transmission != "" && vehicle.Transmission != group.Transmission {
		reasons = append(reasons, ReasonTransmission)
	}
	if group.Fuel != "" && vehicle.Fuel != group.Fuel {
		reasons = append(reasons, ReasonFuel)
	}
	if group.BodyType != "" && vehicle.BodyType != group.BodyType {
		reasons = append(reasons, ReasonBodyType)
	}
	if group.MinSeats > 0 && vehicle.Seats < group.MinSeats {
		reasons = append(reasons, ReasonSeats)
	}
	if group.MinLuggage > 0 && vehicle.Luggage < group.MinLuggage {
		reasons = append(reasons, ReasonLuggage)
	}
	if group.AirConditioning && !vehicle.AirConditioning {
		reasons = append(reasons, ReasonAirConditioning)
	}
	if len(group.CatalogModelIDs) > 0 || len(group.CatalogTypeIDs) > 0 {
		if !containsID(group.CatalogModelIDs, vehicle.CatalogModelID) &&
			!containsID(group.CatalogTypeIDs, vehicle.CatalogTypeID) {
			reasons = append(reasons, ReasonCatalogNotAllowed)
		}
	}
	return len(reasons) == 0, reasons
}

// Guarantee is the promise of a product version, in the terms the checks
// need. Group is required for GROUP.
type Guarantee struct {
	Type           string // EXACT_VEHICLE | EXACT_VARIANT | GROUP
	VehicleID      *uuid.UUID
	VariantLevel   string // model | type
	CatalogModelID int64
	CatalogTypeID  int64
	Group          *GroupCriteria
}

// SatisfiesGuarantee reports whether vehicle keeps the promise of a product
// version:
//   - EXACT_VEHICLE: it is the guaranteed vehicle;
//   - EXACT_VARIANT: it is of the guaranteed catalog model (level "model")
//     or type (level "type");
//   - GROUP: it is eligible for the guaranteed rental group.
func SatisfiesGuarantee(vehicle VehicleAttributes, g Guarantee) (bool, []string) {
	switch g.Type {
	case "EXACT_VEHICLE":
		if g.VehicleID == nil || *g.VehicleID == uuid.Nil {
			return false, []string{ReasonVehicleUnset}
		}
		if vehicle.ID != *g.VehicleID {
			return false, []string{ReasonVehicleMismatch}
		}
		return true, []string{}
	case "EXACT_VARIANT":
		switch g.VariantLevel {
		case "model":
			if g.CatalogModelID == 0 {
				return false, []string{ReasonVariantUnset}
			}
			if vehicle.CatalogModelID != g.CatalogModelID {
				return false, []string{ReasonModelMismatch}
			}
			return true, []string{}
		case "type":
			if g.CatalogTypeID == 0 {
				return false, []string{ReasonVariantUnset}
			}
			if vehicle.CatalogTypeID != g.CatalogTypeID {
				return false, []string{ReasonTypeMismatch}
			}
			return true, []string{}
		}
		return false, []string{ReasonVariantUnset}
	case "GROUP":
		if g.Group == nil {
			return false, []string{ReasonGroupCriteriaUnset}
		}
		return EligibleForGroup(vehicle, *g.Group)
	}
	return false, []string{ReasonUnknownGuarantee}
}

func containsID(ids []int64, id int64) bool {
	if id == 0 {
		return false
	}
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
