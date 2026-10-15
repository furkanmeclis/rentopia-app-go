package usecase

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/contracts/model"
	resmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/pricing"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Snapshot is the source data of a document version: the display value of
// every placeholder and the records it came from.
type Snapshot struct {
	Values map[string]string `json:"values"`
	Refs   map[string]any    `json:"refs"`
}

// formatAmount renders a decimal amount the Turkish way: 4.500,00 TRY.
func formatAmount(m money.Money, currency string) string {
	s := m.String()
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	whole, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := b.String() + "," + frac
	if neg {
		out = "-" + out
	}
	if currency = strings.TrimSpace(currency); currency != "" {
		out += " " + currency
	}
	return out
}

func formatAmountString(v, currency string) string {
	if strings.TrimSpace(v) == "" {
		return ""
	}
	m, err := money.Parse(v)
	if err != nil {
		return v
	}
	return formatAmount(m, currency)
}

func formatTime(t time.Time) string { return rentaltime.Local(t).Format("02.01.2006 15:04") }

func formatDate(t time.Time) string { return rentaltime.Local(t).Format("02.01.2006") }

func formatInt(v int64) string {
	s := strconv.FormatInt(v, 10)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func displayName(typ, first, last, company string) string {
	if typ == "corporate" && strings.TrimSpace(company) != "" {
		return company
	}
	return strings.TrimSpace(first + " " + last)
}

// sampleSnapshot is the data of a preview without a reservation.
func sampleSnapshot() Snapshot {
	v := map[string]string{}
	for _, p := range model.Catalog {
		v[p.Key] = p.Sample
	}
	v["drivers.table"] = table([]string{"Sürücü", "Rol", "Ehliyet"}, [][]string{
		{"Ayşe Yılmaz", "Asıl sürücü", "B •••• 4821"}, {"Mehmet Yılmaz", "Ek sürücü", "B •••• 1290"},
	})
	v["extras.table"] = table([]string{"Ek hizmet", "Adet", "Tutar"}, [][]string{
		{"Bebek koltuğu", "1", "450,00 TRY"}, {"Ek sürücü", "1", "300,00 TRY"},
	})
	return Snapshot{Values: v, Refs: map[string]any{"sample": true}}
}

// snapshot collects the placeholder values of a reservation (and its live
// rental) now.
func (s *Service) snapshot(ctx context.Context, orgID, reservationID int64, resUUID uuid.UUID) (Snapshot, error) {
	res, err := s.q.GetReservationView(ctx, db.GetReservationViewParams{OrganizationID: orgID, Uuid: resUUID})
	if err != nil {
		return Snapshot{}, err
	}
	v := map[string]string{}
	refs := map[string]any{"reservation_uuid": res.Uuid}
	cur := strings.TrimSpace(res.Currency)

	// Company (current settings version).
	if s.settings != nil {
		if ver, err := s.settings.Current(ctx, orgID); err == nil {
			c := ver.Settings.Company
			v["company.legal_name"], v["company.tax_number"], v["company.tax_office"] = c.LegalName, c.TaxNumber, c.TaxOffice
			v["company.address"], v["company.phone"], v["company.email"] = c.Address, c.Phone, c.Email
		}
	}
	if v["company.legal_name"] == "" {
		if o, err := s.q.GetDocumentOrganization(ctx, orgID); err == nil {
			v["company.legal_name"] = o.Name
		}
	}

	v["reservation.reference"], v["reservation.channel"] = res.Reference, res.Channel
	v["renter.full_name"] = displayName(res.CustomerType, res.CustomerFirstName, res.CustomerLastName, res.CustomerCompanyTitle)
	v["renter.identity"], v["renter.phone"], v["renter.email"] = res.CustomerIdentityHint, res.CustomerPhone, res.CustomerEmail
	refs["customer_uuid"] = res.CustomerUuid
	if res.InvoiceCustomerUuid.Valid {
		v["invoice_customer.full_name"] = displayName(res.InvoiceCustomerType.String, res.InvoiceFirstName.String,
			res.InvoiceLastName.String, res.InvoiceCompanyTitle.String)
	} else {
		v["invoice_customer.full_name"] = v["renter.full_name"]
	}

	// Drivers.
	drivers, err := s.q.ListReservationDrivers(ctx, db.ListReservationDriversParams{OrganizationID: orgID, ReservationID: reservationID})
	if err != nil {
		return Snapshot{}, err
	}
	rows := [][]string{}
	for _, d := range drivers {
		var snap struct {
			Name        string `json:"name"`
			LicenceHint string `json:"licence_hint"`
		}
		_ = json.Unmarshal(d.Snapshot, &snap)
		name := strings.TrimSpace(d.FirstName.String + " " + d.LastName.String)
		if name == "" {
			name = snap.Name
		}
		lic := strings.TrimSpace(d.LicenceClass.String + " " + d.LicenceHint.String)
		if lic == "" {
			lic = snap.LicenceHint
		}
		role := "Ek sürücü"
		if d.Role == "primary" {
			role = "Asıl sürücü"
			if v["drivers.primary_name"] == "" {
				v["drivers.primary_name"], v["drivers.primary_licence"] = name, lic
			}
		}
		rows = append(rows, []string{name, role, lic})
	}
	v["drivers.table"] = table([]string{"Sürücü", "Rol", "Ehliyet"}, rows)

	// Vehicle and product.
	v["vehicle.plate"], v["vehicle.label"] = res.VehiclePlate.String, res.VehicleLabel
	refs["vehicle_uuid"] = uuidOrNil(res.VehicleUuid)
	var conds resmodel.Conditions
	_ = json.Unmarshal(res.ProductSnapshot, &conds)
	v["product.title"], v["product.code"], v["product.guarantee"] = res.ProductTitle, res.ProductCode, conds.GuaranteeLabel
	v["product.deposit"] = formatAmountString(conds.DepositAmount, cur)
	v["product.extra_km_fee"] = formatAmountString(conds.ExtraKmFee, cur)
	v["product.fuel_policy"] = conds.FuelPolicy
	if conds.IncludedKmPerDay != nil {
		v["product.included_km_per_day"] = strconv.Itoa(int(*conds.IncludedKmPerDay))
	}

	// Dates and places.
	v["dates.pickup_at"], v["dates.return_at"] = formatTime(res.StartsAt.Time), formatTime(res.EndsAt.Time)
	if r, err := rentaltime.NewRange(res.StartsAt.Time, res.EndsAt.Time); err == nil {
		v["dates.billing_days"] = strconv.Itoa(r.BillingDays())
	}
	v["points.pickup_branch"], v["points.return_branch"] = res.PickupBranchName, res.ReturnBranchName
	v["points.pickup_point"], v["points.return_point"] = res.PickupPointName.String, res.ReturnPointName.String

	// Amounts and extras (the price snapshot; extensions included).
	total := money.FromNumericOrZero(res.TotalAmount)
	paid := money.FromNumericOrZero(res.PaidAmount)
	v["amounts.total"] = formatAmount(total, cur)
	v["amounts.prepayment"] = formatAmount(money.FromNumericOrZero(res.PrepaymentAmount), cur)
	v["amounts.paid"] = formatAmount(paid, cur)
	v["amounts.balance"] = formatAmount(total.Sub(paid), cur)
	v["amounts.currency"] = cur
	var price resmodel.PriceSnapshot
	_ = json.Unmarshal(res.QuoteSnapshot, &price)
	ext := money.Zero()
	for _, e := range price.Extensions {
		ext = ext.Add(e.Amount)
	}
	v["amounts.extensions_total"] = formatAmount(ext, cur)
	extras := [][]string{}
	if price.Pricing != nil {
		for _, l := range price.Pricing.Lines {
			if l.Kind == pricing.LineExtra {
				extras = append(extras, []string{l.Name, strconv.Itoa(l.Quantity), formatAmount(l.Gross, cur)})
			}
		}
	}
	v["extras.table"] = table([]string{"Ek hizmet", "Adet", "Tutar"}, extras)

	// Policies of the reservation's settings version.
	var pol struct {
		SettingsVersion int `json:"settings_version"`
		Policies        struct {
			Cancellation           string `json:"cancellation"`
			NoShow                 string `json:"no_show"`
			EarlyReturn            string `json:"early_return"`
			Extension              string `json:"extension"`
			LateReturnGraceMinutes *int   `json:"late_return_grace_minutes"`
			ExtraHourFee           string `json:"extra_hour_fee"`
			Fuel                   string `json:"fuel"`
			Mileage                string `json:"mileage"`
		} `json:"policies"`
	}
	_ = json.Unmarshal(res.PolicySnapshot, &pol)
	v["policies.cancellation"], v["policies.no_show"] = pol.Policies.Cancellation, pol.Policies.NoShow
	v["policies.early_return"], v["policies.extension"] = pol.Policies.EarlyReturn, pol.Policies.Extension
	v["policies.fuel"], v["policies.mileage"] = pol.Policies.Fuel, pol.Policies.Mileage
	if pol.Policies.LateReturnGraceMinutes != nil {
		v["policies.late_return_grace_minutes"] = strconv.Itoa(*pol.Policies.LateReturnGraceMinutes)
	}
	v["policies.extra_hour_fee"] = formatAmountString(pol.Policies.ExtraHourFee, cur)
	refs["settings_version"] = pol.SettingsVersion

	// The live rental: actual facts and the vehicle legs.
	if r, err := s.q.GetDocumentRentalFacts(ctx, db.GetDocumentRentalFactsParams{OrganizationID: orgID, ReservationID: reservationID}); err == nil {
		refs["rental_uuid"] = r.Uuid
		v["rental.reference"] = r.Reference
		if r.ActualPickupAt.Valid {
			v["dates.actual_pickup_at"] = formatTime(r.ActualPickupAt.Time)
		}
		if r.ActualReturnAt.Valid {
			v["dates.actual_return_at"] = formatTime(r.ActualReturnAt.Time)
		}
		v["points.actual_return_branch"] = r.ActualReturnBranchName.String
		legs, err := s.q.ListRentalVehicles(ctx, db.ListRentalVehiclesParams{OrganizationID: orgID, RentalID: r.ID})
		if err == nil && len(legs) > 0 {
			cur := legs[len(legs)-1]
			v["vehicle.plate"], v["vehicle.label"] = cur.VehiclePlate, cur.VehicleLabel
			refs["vehicle_uuid"] = cur.VehicleUuid
			v["vehicle.km_out"], v["vehicle.fuel_out"] = intText(cur.OutKm), fuelText(cur.OutFuel)
			v["vehicle.km_in"], v["vehicle.fuel_in"] = intText(cur.InKm), fuelText(cur.InFuel)
		}
	}
	return Snapshot{Values: v, Refs: refs}, nil
}

func uuidOrNil(v pgtype.UUID) any {
	if !v.Valid {
		return nil
	}
	return uuid.UUID(v.Bytes)
}

func intText(v pgtype.Int4) string {
	if !v.Valid {
		return ""
	}
	return formatInt(int64(v.Int32))
}

func fuelText(v pgtype.Int2) string {
	if !v.Valid {
		return ""
	}
	return "%" + strconv.Itoa(int(v.Int16))
}
