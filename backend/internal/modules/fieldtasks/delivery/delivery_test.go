package delivery

import (
	"testing"
	"time"

	branchmodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/fieldtasks/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/rentaltime"
)

func local(y int, m time.Month, d, hh, mm int) time.Time {
	return time.Date(y, m, d, hh, mm, 0, 0, rentaltime.Location())
}

func codes(ps []Problem) map[string]string {
	out := map[string]string{}
	for _, p := range ps {
		out[p.Field] = p.Code
	}
	return out
}

func TestCheckRules(t *testing.T) {
	day := branchmodel.DayHours{Open: "09:00", Close: "18:00"}
	week := branchmodel.WeeklyHours{Mon: day, Tue: day, Wed: day, Thu: day, Fri: day, Sat: day, Sun: branchmodel.DayHours{Closed: true}}
	lat, lng, radius := 40.99, 29.03, 10.0
	zone := Point{
		Type: branchmodel.ServicePointAddressZone, Active: true, DeliveryEnabled: true, OnlineSelectable: false,
		BranchHours: week, ServiceDistricts: []string{"Kadıköy"}, Latitude: &lat, Longitude: &lng, RadiusKm: &radius,
		MinNoticeMinutes: 120, Holidays: []branchmodel.HolidayException{{Date: "2030-04-23", Closed: true}},
	}
	now := local(2030, 4, 20, 8, 0)   // Saturday
	mon := local(2030, 4, 22, 10, 30) // Monday
	ok := &model.Delivery{Address: "Bağdat Cd.", District: "KADIKÖY"}
	if p := Check("pickup", zone, mon, now, ok, "branch"); len(p) != 0 {
		t.Fatalf("inside: %+v", p)
	}
	cases := []struct {
		name    string
		at      time.Time
		d       *model.Delivery
		channel string
		field   string
		code    string
	}{
		{"evening", local(2030, 4, 22, 19, 0), ok, "branch", "pickup_point_uuid", CodeOutsideHours},
		{"sunday", local(2030, 4, 21, 11, 0), ok, "branch", "pickup_point_uuid", CodeClosed},
		{"holiday", local(2030, 4, 23, 11, 0), ok, "branch", "pickup_point_uuid", CodeClosed},
		{"notice", local(2030, 4, 20, 9, 30), ok, "branch", "pickup_point_uuid", CodeTooSoon},
		{"online", mon, ok, "web", "pickup_point_uuid", CodeNotOnline},
		{"district", mon, &model.Delivery{Address: "x", District: "Beşiktaş"}, "branch", "pickup_delivery.district", CodeOutsideArea},
		{"radius", mon, &model.Delivery{Address: "x", District: "Kadıköy", Latitude: fp(41.4), Longitude: fp(29.03)}, "branch", "pickup_delivery.address", CodeOutsideArea},
		{"address", mon, nil, "branch", "pickup_delivery.address", CodeAddressRequired},
	}
	for _, c := range cases {
		if got := codes(Check("pickup", zone, c.at, now, c.d, c.channel)); got[c.field] != c.code {
			t.Fatalf("%s: %v", c.name, got)
		}
	}
	// A point's own hours win over the branch's; an overnight schedule
	// wraps midnight.
	night := branchmodel.DayHours{Open: "22:00", Close: "06:00"}
	airport := Point{Type: branchmodel.ServicePointAirport, Active: true, DeliveryEnabled: true, BranchHours: week,
		Hours: &branchmodel.WeeklyHours{Mon: night, Tue: night, Wed: night, Thu: night, Fri: night, Sat: night, Sun: night}}
	if p := Check("return", airport, local(2030, 4, 22, 2, 0), now, nil, "branch"); len(p) != 0 {
		t.Fatalf("02:00 overnight: %+v", p)
	}
	if got := codes(Check("return", airport, local(2030, 4, 22, 12, 0), now, nil, "branch")); got["return_point_uuid"] != CodeOutsideHours {
		t.Fatalf("noon at a night point: %v", got)
	}
	office := Point{Type: branchmodel.ServicePointOffice, Active: true}
	if got := codes(Check("pickup", office, mon, now, ok, "branch")); got["pickup_delivery.address"] != CodeNotDelivery {
		t.Fatalf("office address: %v", got)
	}
	if got := codes(Check("pickup", Point{}, mon, now, nil, "branch")); got["pickup_point_uuid"] != CodeInactive {
		t.Fatalf("inactive: %v", got)
	}
	if d := Distance(41.0082, 28.9784, 40.9909, 29.0303); d < 4 || d > 6 {
		t.Fatalf("Sultanahmet - Kadıköy %.2f km", d)
	}
}

func fp(v float64) *float64 { return &v }
