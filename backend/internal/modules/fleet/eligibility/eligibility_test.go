package eligibility

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

// Catalog ids of the worked examples (platform catalog rows).
const (
	modelClio      int64 = 101
	modelCorsa     int64 = 102
	modelI20       int64 = 103
	modelEgea      int64 = 104
	modelEgeaCross int64 = 105
	modelPolo      int64 = 106

	typeEgeaCross14Fire   int64 = 9001 // Fiat Egea Cross 1.4 Fire
	typeEgeaCross16Mjet   int64 = 9002 // Fiat Egea Cross 1.6 Multijet
	typeClio10TCe         int64 = 9101
	typeCorsa12           int64 = 9201
	typeI2014MPI          int64 = 9301
	typePolo10TSIAutoDisc int64 = 9401
)

// "Clio/Corsa/i20 veya benzeri": economy hatchback, manual, petrol, A/C,
// 5 seats, catalog criteria = the three models.
func ecoHatchGroup() GroupCriteria {
	return GroupCriteria{
		Transmission: "manual", Fuel: "petrol", BodyType: "hatchback",
		MinSeats: 5, MinLuggage: 2, AirConditioning: true,
		CatalogModelIDs: []int64{modelClio, modelCorsa, modelI20},
	}
}

func clio() VehicleAttributes {
	return VehicleAttributes{
		ID: uuid.New(), CatalogModelID: modelClio, CatalogTypeID: typeClio10TCe,
		Transmission: "manual", Fuel: "petrol", BodyType: "hatchback", Seats: 5, Luggage: 2, AirConditioning: true,
	}
}

func TestEligibleForGroup(t *testing.T) {
	t.Parallel()
	corsa := clio()
	corsa.CatalogModelID, corsa.CatalogTypeID = modelCorsa, typeCorsa12
	i20 := clio()
	i20.CatalogModelID, i20.CatalogTypeID = modelI20, typeI2014MPI
	i20.Luggage = 3

	clioAuto := clio()
	clioAuto.Transmission = "automatic"
	clioDiesel := clio()
	clioDiesel.Fuel = "diesel"
	polo := clio()
	polo.CatalogModelID, polo.CatalogTypeID = modelPolo, typePolo10TSIAutoDisc
	clioNoAC := clio()
	clioNoAC.AirConditioning = false
	clioSmallBoot := clio()
	clioSmallBoot.Luggage = 1
	clioFourSeats := clio()
	clioFourSeats.Seats = 4
	egeaSedan := clio()
	egeaSedan.CatalogModelID, egeaSedan.BodyType = modelEgea, "sedan"

	// Attributes only: any model with the protected attributes.
	anyAutomaticSUV := GroupCriteria{Transmission: "automatic", BodyType: "suv"}
	egeaCrossAuto := VehicleAttributes{
		ID: uuid.New(), CatalogModelID: modelEgeaCross, CatalogTypeID: typeEgeaCross14Fire,
		Transmission: "automatic", Fuel: "petrol", BodyType: "suv", Seats: 5, Luggage: 3, AirConditioning: true,
	}
	// Type-level criteria: only the 1.4 Fire variant of the Egea Cross.
	egeaCross14Only := GroupCriteria{CatalogTypeIDs: []int64{typeEgeaCross14Fire}}
	egeaCrossDiesel := egeaCrossAuto
	egeaCrossDiesel.CatalogTypeID = typeEgeaCross16Mjet

	cases := []struct {
		name    string
		vehicle VehicleAttributes
		group   GroupCriteria
		ok      bool
		reasons []string
	}{
		{"Clio is eligible for Clio/Corsa/i20 veya benzeri", clio(), ecoHatchGroup(), true, []string{}},
		{"Corsa is eligible for Clio/Corsa/i20 veya benzeri", corsa, ecoHatchGroup(), true, []string{}},
		{"i20 with a bigger boot is eligible", i20, ecoHatchGroup(), true, []string{}},
		{"automatic Clio breaks the manual guarantee", clioAuto, ecoHatchGroup(), false, []string{ReasonTransmission}},
		{"diesel Clio breaks the petrol guarantee", clioDiesel, ecoHatchGroup(), false, []string{ReasonFuel}},
		{"Polo is not in the catalog criteria", polo, ecoHatchGroup(), false, []string{ReasonCatalogNotAllowed}},
		{"no A/C", clioNoAC, ecoHatchGroup(), false, []string{ReasonAirConditioning}},
		{"luggage below minimum", clioSmallBoot, ecoHatchGroup(), false, []string{ReasonLuggage}},
		{"seats below minimum", clioFourSeats, ecoHatchGroup(), false, []string{ReasonSeats}},
		{"every failed check is reported", egeaSedan, ecoHatchGroup(), false, []string{ReasonBodyType, ReasonCatalogNotAllowed}},
		{"attribute-only group accepts any model", egeaCrossAuto, anyAutomaticSUV, true, []string{}},
		{"attribute-only group rejects a manual hatchback", clio(), anyAutomaticSUV, false, []string{ReasonTransmission, ReasonBodyType}},
		{"type criteria accept the listed variant", egeaCrossAuto, egeaCross14Only, true, []string{}},
		{"type criteria reject another variant of the model", egeaCrossDiesel, egeaCross14Only, false, []string{ReasonCatalogNotAllowed}},
		{"unknown catalog ids never match criteria", VehicleAttributes{Transmission: "manual"}, GroupCriteria{CatalogModelIDs: []int64{modelClio}}, false, []string{ReasonCatalogNotAllowed}},
		{"empty group accepts any vehicle", VehicleAttributes{}, GroupCriteria{}, true, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reasons := EligibleForGroup(tc.vehicle, tc.group)
			if ok != tc.ok || !reflect.DeepEqual(reasons, tc.reasons) {
				t.Fatalf("EligibleForGroup = %v %v, want %v %v", ok, reasons, tc.ok, tc.reasons)
			}
		})
	}
}

func TestSatisfiesGuarantee(t *testing.T) {
	t.Parallel()
	// "Fiat Egea Cross 1.4 Fire (model garantili)" at type level.
	egeaCross14 := VehicleAttributes{
		ID: uuid.New(), CatalogModelID: modelEgeaCross, CatalogTypeID: typeEgeaCross14Fire,
		Transmission: "manual", Fuel: "petrol", BodyType: "crossover", Seats: 5, Luggage: 3, AirConditioning: true,
	}
	egeaCross16 := egeaCross14
	egeaCross16.ID, egeaCross16.CatalogTypeID, egeaCross16.Fuel = uuid.New(), typeEgeaCross16Mjet, "diesel"
	egeaSedan := egeaCross14
	egeaSedan.ID, egeaSedan.CatalogModelID, egeaSedan.CatalogTypeID = uuid.New(), modelEgea, 0

	exactType := Guarantee{Type: "EXACT_VARIANT", VariantLevel: "type", CatalogModelID: modelEgeaCross, CatalogTypeID: typeEgeaCross14Fire}
	exactModel := Guarantee{Type: "EXACT_VARIANT", VariantLevel: "model", CatalogModelID: modelEgeaCross}
	group := ecoHatchGroup()
	orSimilar := Guarantee{Type: "GROUP", Group: &group}
	thisCar := egeaCross14.ID
	exactVehicle := Guarantee{Type: "EXACT_VEHICLE", VehicleID: &thisCar}

	cases := []struct {
		name    string
		vehicle VehicleAttributes
		g       Guarantee
		ok      bool
		reasons []string
	}{
		{"Egea Cross 1.4 Fire keeps the type guarantee", egeaCross14, exactType, true, []string{}},
		{"Egea Cross 1.6 Multijet breaks the type guarantee", egeaCross16, exactType, false, []string{ReasonTypeMismatch}},
		{"any Egea Cross keeps the model guarantee", egeaCross16, exactModel, true, []string{}},
		{"Egea sedan breaks the Egea Cross model guarantee", egeaSedan, exactModel, false, []string{ReasonModelMismatch}},
		{"variant level without a reference", egeaCross14, Guarantee{Type: "EXACT_VARIANT", VariantLevel: "type"}, false, []string{ReasonVariantUnset}},
		{"Clio keeps Clio/Corsa/i20 veya benzeri", clio(), orSimilar, true, []string{}},
		{"Egea Cross does not keep Clio/Corsa/i20 veya benzeri", egeaCross14, orSimilar, false, []string{ReasonBodyType, ReasonCatalogNotAllowed}},
		{"group guarantee without criteria", clio(), Guarantee{Type: "GROUP"}, false, []string{ReasonGroupCriteriaUnset}},
		{"the guaranteed vehicle", egeaCross14, exactVehicle, true, []string{}},
		{"another vehicle of the same type", egeaCross16, exactVehicle, false, []string{ReasonVehicleMismatch}},
		{"vehicle placeholder not set yet", egeaCross14, Guarantee{Type: "EXACT_VEHICLE"}, false, []string{ReasonVehicleUnset}},
		{"unknown guarantee type", egeaCross14, Guarantee{Type: "STOCK"}, false, []string{ReasonUnknownGuarantee}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reasons := SatisfiesGuarantee(tc.vehicle, tc.g)
			if ok != tc.ok || !reflect.DeepEqual(reasons, tc.reasons) {
				t.Fatalf("SatisfiesGuarantee = %v %v, want %v %v", ok, reasons, tc.ok, tc.reasons)
			}
		})
	}
}
