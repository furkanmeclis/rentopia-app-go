// Package delivery decides whether a service point can deliver (or
// collect) a vehicle at a time and an address, and what it costs
// (TEC-724). It is pure: the use case loads the point, its branch hours
// and the delivery extra's price, this package applies the rules.
//
// Rules, in order:
//   - the point is active, delivers (delivery_enabled) and, on an online
//     channel, is selectable online;
//   - the time is at least min_notice_minutes ahead;
//   - the time (Europe/Istanbul) is inside the point's working hours, else
//     the branch's, and the branch is not closed by a holiday exception;
//   - an address zone needs an address: its district must be one of the
//     served districts (when listed), and with a centre and radius the
//     coordinates (when given) must be within the radius.
package delivery

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	branchmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
)

// Problem codes (validation details on <side>_point_uuid / <side>_delivery.*).
const (
	CodeNotDelivery     = "not_delivery_point"
	CodeInactive        = "inactive"
	CodeNotOnline       = "not_online"
	CodeTooSoon         = "too_soon"
	CodeOutsideHours    = "outside_hours"
	CodeClosed          = "closed"
	CodeAddressRequired = "address_required"
	CodeOutsideArea     = "outside_area"
)

// Point is what the rules read of a service point.
type Point struct {
	Type             string
	Active           bool
	OnlineSelectable bool
	DeliveryEnabled  bool
	MinNoticeMinutes int32
	// Hours are the point's own hours; nil = the branch's.
	Hours            *branchmodel.WeeklyHours
	BranchHours      branchmodel.WeeklyHours
	Holidays         []branchmodel.HolidayException
	ServiceDistricts []string
	Latitude         *float64
	Longitude        *float64
	RadiusKm         *float64
}

// Problem is one refused rule.
type Problem struct {
	Field   string
	Code    string
	Message string
}

// ParseHours reads a stored weekly schedule (nil when empty).
func ParseHours(raw []byte) *branchmodel.WeeklyHours {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var h branchmodel.WeeklyHours
	if json.Unmarshal(raw, &h) != nil {
		return nil
	}
	return &h
}

// ParseHolidays reads stored holiday exceptions.
func ParseHolidays(raw []byte) []branchmodel.HolidayException {
	var out []branchmodel.HolidayException
	_ = json.Unmarshal(raw, &out)
	return out
}

// OnlineChannel reports whether a sales channel is customer facing (every
// channel but the branch desk): the point must then be selectable online.
func OnlineChannel(channel string) bool {
	return channel != "" && channel != "branch"
}

// Check applies the rules to one side. prefix is "pickup" or "return".
func Check(prefix string, p Point, at, now time.Time, d *model.Delivery, channel string) []Problem {
	out := []Problem{}
	add := func(field, code, msg string) { out = append(out, Problem{Field: field, Code: code, Message: msg}) }
	pointField := prefix + "_point_uuid"
	if !p.Active {
		add(pointField, CodeInactive, "the service point is not active")
		return out
	}
	if !p.DeliveryEnabled {
		if d != nil && d.Address != "" {
			add(prefix+"_delivery.address", CodeNotDelivery, "this service point does not deliver to an address")
		}
		return out
	}
	if OnlineChannel(channel) && !p.OnlineSelectable {
		add(pointField, CodeNotOnline, "the service point cannot be chosen online")
	}
	if p.MinNoticeMinutes > 0 && at.Before(now.Add(time.Duration(p.MinNoticeMinutes)*time.Minute)) {
		add(pointField, CodeTooSoon, "the delivery needs more notice")
	}
	if code := hoursProblem(p, at); code != "" {
		msg := "the delivery time is outside the service hours"
		if code == CodeClosed {
			msg = "the service point is closed on that day"
		}
		add(pointField, code, msg)
	}
	if p.Type == branchmodel.ServicePointAddressZone {
		if d == nil || d.Address == "" {
			add(prefix+"_delivery.address", CodeAddressRequired, "an address is required for this delivery zone")
			return out
		}
		if len(p.ServiceDistricts) > 0 && !districtServed(p.ServiceDistricts, d.District) {
			add(prefix+"_delivery.district", CodeOutsideArea, "the district is outside the delivery area")
		}
		if p.RadiusKm != nil && p.Latitude != nil && p.Longitude != nil && d.Latitude != nil && d.Longitude != nil {
			if Distance(*p.Latitude, *p.Longitude, *d.Latitude, *d.Longitude) > *p.RadiusKm {
				add(prefix+"_delivery.address", CodeOutsideArea, "the address is outside the delivery radius")
			}
		}
	}
	return out
}

// hoursProblem checks the local time against the schedule of that day.
func hoursProblem(p Point, at time.Time) string {
	local := at.In(rentaltime.Location())
	date := rentaltime.LocalDate(at).String()
	for _, h := range p.Holidays {
		if h.Date != date {
			continue
		}
		if h.Closed {
			return CodeClosed
		}
		if !inside(h.Open, h.Close, local) {
			return CodeOutsideHours
		}
		return ""
	}
	week := p.BranchHours
	if p.Hours != nil {
		week = *p.Hours
	}
	day := dayOf(&week, local.Weekday())
	if day == nil {
		return ""
	}
	if day.Closed {
		return CodeClosed
	}
	if day.Open == "" && day.Close == "" {
		return ""
	}
	if !inside(day.Open, day.Close, local) {
		return CodeOutsideHours
	}
	return ""
}

func dayOf(w *branchmodel.WeeklyHours, wd time.Weekday) *branchmodel.DayHours {
	switch wd {
	case time.Monday:
		return &w.Mon
	case time.Tuesday:
		return &w.Tue
	case time.Wednesday:
		return &w.Wed
	case time.Thursday:
		return &w.Thu
	case time.Friday:
		return &w.Fri
	case time.Saturday:
		return &w.Sat
	case time.Sunday:
		return &w.Sun
	}
	return nil
}

// inside: open <= hh:mm <= close ("24:00" and an overnight close wrap).
func inside(open, closeAt string, local time.Time) bool {
	o, ok1 := minutes(open)
	c, ok2 := minutes(closeAt)
	if !ok1 || !ok2 {
		return true
	}
	m := local.Hour()*60 + local.Minute()
	if c <= o { // overnight (22:00 - 06:00) or 00:00 - 00:00 (all day)
		return m >= o || m <= c
	}
	return m >= o && m <= c
}

func minutes(hhmm string) (int, bool) {
	t, err := time.Parse("15:04", strings.TrimSpace(hhmm))
	if err != nil {
		if strings.TrimSpace(hhmm) == "24:00" {
			return 24 * 60, true
		}
		return 0, false
	}
	return t.Hour()*60 + t.Minute(), true
}

// districtServed compares districts case-insensitively (Turkish dotted /
// dotless i folded).
func districtServed(served []string, district string) bool {
	d := fold(district)
	if d == "" {
		return false
	}
	for _, s := range served {
		if fold(s) == d {
			return true
		}
	}
	return false
}

func fold(s string) string {
	r := strings.NewReplacer("İ", "i", "I", "i", "ı", "i", "Ş", "s", "ş", "s", "Ğ", "g", "ğ", "g",
		"Ü", "u", "ü", "u", "Ö", "o", "ö", "o", "Ç", "c", "ç", "c")
	return strings.ToLower(r.Replace(strings.TrimSpace(s)))
}

// Distance is the great-circle distance in km (haversine).
func Distance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthKm = 6371.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthKm * math.Asin(math.Min(1, math.Sqrt(a)))
}
