package model

import (
	"slices"
	"testing"
	"time"
)

func date(s string) *time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &t
}

func TestFullYears(t *testing.T) {
	at := time.Date(2026, 5, 1, 9, 0, 0, 0, Istanbul)
	cases := []struct {
		from string
		want int
	}{
		{"2000-05-01", 26}, // birthday today
		{"2000-05-02", 25}, // tomorrow
		{"2000-04-30", 26},
		{"2030-01-01", 0}, // future clamps to 0
	}
	for _, c := range cases {
		if got := FullYears(*date(c.from), at); got != c.want {
			t.Errorf("FullYears(%s) = %d, want %d", c.from, got, c.want)
		}
	}
	// 29 February: the anniversary is 1 March in a non-leap year.
	if got := FullYears(*date("2004-02-29"), time.Date(2025, 2, 28, 12, 0, 0, 0, Istanbul)); got != 20 {
		t.Errorf("leap day before anniversary = %d", got)
	}
	if got := FullYears(*date("2004-02-29"), time.Date(2025, 3, 1, 0, 30, 0, 0, Istanbul)); got != 21 {
		t.Errorf("leap day on 1 March = %d", got)
	}
	// 22:30 UTC on 30 April is already 1 May in İstanbul.
	if got := FullYears(*date("2000-05-01"), time.Date(2026, 4, 30, 22, 30, 0, 0, time.UTC)); got != 26 {
		t.Errorf("Istanbul calendar day = %d", got)
	}
}

func TestEligibleForConditions(t *testing.T) {
	at := time.Date(2026, 6, 1, 10, 0, 0, 0, Istanbul)
	cond := Conditions{MinDriverAge: 25, MinLicenseYears: 3}
	ok := DriverFacts{BirthDate: date("1990-01-01"), HasLicence: true, LicenceIssuedOn: date("2015-01-01")}
	if e := EligibleForConditions(ok, cond, at); !e.Eligible || len(e.Reasons) != 0 || *e.Age != 36 || *e.LicenceYears != 11 {
		t.Fatalf("eligible driver: %+v", e)
	}
	young := DriverFacts{BirthDate: date("2003-01-01"), HasLicence: true, LicenceIssuedOn: date("2024-01-01")}
	e := EligibleForConditions(young, cond, at)
	if e.Eligible || !slices.Equal(e.Reasons, []string{ReasonUnderMinAge, ReasonUnderLicenceYears}) {
		t.Fatalf("young driver: %+v", e)
	}
	missing := DriverFacts{}
	e = EligibleForConditions(missing, cond, at)
	if !slices.Equal(e.Reasons, []string{ReasonBirthDateMissing, ReasonLicenceMissing}) {
		t.Fatalf("missing data: %+v", e)
	}
	noDate := DriverFacts{BirthDate: date("1990-01-01"), HasLicence: true}
	if e := EligibleForConditions(noDate, cond, at); !slices.Equal(e.Reasons, []string{ReasonLicenceDateMissing}) {
		t.Fatalf("licence without issue date: %+v", e)
	}
	// Zero conditions check nothing but an expired licence.
	if e := EligibleForConditions(missing, Conditions{}, at); !e.Eligible {
		t.Fatalf("zero conditions: %+v", e)
	}
	expired := ok
	expired.LicenceExpiresOn = date("2026-05-31")
	if e := EligibleForConditions(expired, cond, at); e.Eligible || !slices.Equal(e.Reasons, []string{ReasonLicenceExpired}) {
		t.Fatalf("expired licence: %+v", e)
	}
	expired.LicenceExpiresOn = date("2026-06-01") // valid through the pickup day
	if e := EligibleForConditions(expired, cond, at); !e.Eligible {
		t.Fatalf("licence expiring on the pickup day: %+v", e)
	}
}
