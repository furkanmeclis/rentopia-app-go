package model

import (
	"time"
)

// Rental condition check of a driver (TEC-705 → reservations, TEC-706).
// Fleet product versions carry the conditions (min_driver_age,
// min_license_years); reservations call EligibleForConditions with the
// driver facts and the pickup time and show the reasons at the counter.

// Ineligibility reasons (stable codes for the UI and the reservations API).
const (
	ReasonBirthDateMissing   = "birth_date_missing"
	ReasonUnderMinAge        = "under_min_age"
	ReasonLicenceMissing     = "licence_missing"
	ReasonLicenceDateMissing = "licence_issue_date_missing"
	ReasonUnderLicenceYears  = "under_min_licence_years"
	ReasonLicenceExpired     = "licence_expired"
)

// Conditions are the driver-related rental conditions of a product.
type Conditions struct {
	MinDriverAge    int `json:"min_driver_age"`
	MinLicenseYears int `json:"min_license_years"`
}

// DriverFacts are the plain driver fields the check needs (no identity
// values: the check never decrypts anything).
type DriverFacts struct {
	BirthDate        *time.Time
	HasLicence       bool
	LicenceIssuedOn  *time.Time
	LicenceExpiresOn *time.Time
}

// Eligibility is the outcome of EligibleForConditions.
type Eligibility struct {
	Eligible     bool     `json:"eligible"`
	Reasons      []string `json:"reasons"`
	Age          *int     `json:"age"`
	LicenceYears *int     `json:"licence_years"`
	// At is the calendar day the check ran for (Europe/Istanbul).
	At string `json:"at"`
}

// Istanbul is the calendar of age and licence-year computations (rental
// contracts are Turkish local dates).
var Istanbul = loadIstanbul()

func loadIstanbul() *time.Location {
	if loc, err := time.LoadLocation("Europe/Istanbul"); err == nil {
		return loc
	}
	return time.FixedZone("TRT", 3*60*60)
}

// FullYears returns the number of full years from the calendar date `from`
// (its own year / month / day, as read from a DATE column) to the calendar
// day of `at` in Europe/Istanbul. A 29 February anniversary counts from
// 1 March in non-leap years.
func FullYears(from, at time.Time) int {
	a := at.In(Istanbul)
	years := a.Year() - from.Year()
	anniversary := time.Date(a.Year(), from.Month(), from.Day(), 0, 0, 0, 0, Istanbul)
	day := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, Istanbul)
	if day.Before(anniversary) {
		years--
	}
	if years < 0 {
		return 0
	}
	return years
}

// EligibleForConditions checks a driver against product conditions at a
// point in time (the pickup). Missing data counts as not eligible when the
// condition needs it; a zero condition is not checked. A licence that
// expires before `at` is always a reason.
func EligibleForConditions(d DriverFacts, c Conditions, at time.Time) Eligibility {
	out := Eligibility{Reasons: []string{}, At: at.In(Istanbul).Format("2006-01-02")}
	if d.BirthDate != nil {
		age := FullYears(*d.BirthDate, at)
		out.Age = &age
	}
	if d.LicenceIssuedOn != nil {
		years := FullYears(*d.LicenceIssuedOn, at)
		out.LicenceYears = &years
	}
	if c.MinDriverAge > 0 {
		switch {
		case out.Age == nil:
			out.Reasons = append(out.Reasons, ReasonBirthDateMissing)
		case *out.Age < c.MinDriverAge:
			out.Reasons = append(out.Reasons, ReasonUnderMinAge)
		}
	}
	if c.MinLicenseYears > 0 {
		switch {
		case !d.HasLicence:
			out.Reasons = append(out.Reasons, ReasonLicenceMissing)
		case out.LicenceYears == nil:
			out.Reasons = append(out.Reasons, ReasonLicenceDateMissing)
		case *out.LicenceYears < c.MinLicenseYears:
			out.Reasons = append(out.Reasons, ReasonUnderLicenceYears)
		}
	}
	if d.LicenceExpiresOn != nil {
		exp := time.Date(d.LicenceExpiresOn.Year(), d.LicenceExpiresOn.Month(), d.LicenceExpiresOn.Day(), 0, 0, 0, 0, Istanbul)
		a := at.In(Istanbul)
		if time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, Istanbul).After(exp) {
			out.Reasons = append(out.Reasons, ReasonLicenceExpired)
		}
	}
	out.Eligible = len(out.Reasons) == 0
	return out
}
