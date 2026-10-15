package model

// Catalog is the placeholder catalog: every {{key}} a template may use,
// grouped for the editor's picker, with the sample value the preview
// renders. The panel labels them through i18n (contracts.placeholders.<key>).
var Catalog = []Placeholder{
	{Key: "company.legal_name", Group: "company", Sample: "Örnek Oto Kiralama A.Ş."},
	{Key: "company.tax_number", Group: "company", Sample: "1234567890"},
	{Key: "company.tax_office", Group: "company", Sample: "Kadıköy"},
	{Key: "company.address", Group: "company", Sample: "Caferağa Mah. Moda Cad. No:1 Kadıköy / İstanbul"},
	{Key: "company.phone", Group: "company", Sample: "+90 216 000 00 00"},
	{Key: "company.email", Group: "company", Sample: "info@ornek.com.tr"},

	{Key: "reservation.reference", Group: "reservation", Sample: "ORN-2026-000123"},
	{Key: "reservation.channel", Group: "reservation", Sample: "branch"},
	{Key: "rental.reference", Group: "reservation", Sample: "ORN-2026-000123"},

	{Key: "renter.full_name", Group: "renter", Sample: "Ayşe Yılmaz"},
	{Key: "renter.identity", Group: "renter", Sample: "1•••••••••01"},
	{Key: "renter.phone", Group: "renter", Sample: "+90 532 000 00 00"},
	{Key: "renter.email", Group: "renter", Sample: "ayse@example.com"},
	{Key: "invoice_customer.full_name", Group: "invoice_customer", Sample: "Yılmaz Lojistik Ltd. Şti."},

	{Key: "drivers.primary_name", Group: "drivers", Sample: "Ayşe Yılmaz"},
	{Key: "drivers.primary_licence", Group: "drivers", Sample: "B •••• 4821"},
	{Key: "drivers.table", Group: "drivers", HTML: true, Sample: ""},

	{Key: "vehicle.plate", Group: "vehicle", Sample: "34 ABC 123"},
	{Key: "vehicle.label", Group: "vehicle", Sample: "Renault Clio"},
	{Key: "vehicle.km_out", Group: "vehicle", Sample: "45.210"},
	{Key: "vehicle.fuel_out", Group: "vehicle", Sample: "%75"},
	{Key: "vehicle.km_in", Group: "vehicle", Sample: "46.050"},
	{Key: "vehicle.fuel_in", Group: "vehicle", Sample: "%50"},

	{Key: "product.title", Group: "product", Sample: "Ekonomik Hatchback"},
	{Key: "product.code", Group: "product", Sample: "EKO-HB"},
	{Key: "product.guarantee", Group: "product", Sample: "Renault Clio veya benzeri"},
	{Key: "product.deposit", Group: "product", Sample: "5.000,00 TRY"},
	{Key: "product.included_km_per_day", Group: "product", Sample: "300"},
	{Key: "product.extra_km_fee", Group: "product", Sample: "4,50 TRY"},
	{Key: "product.fuel_policy", Group: "product", Sample: "same_level"},

	{Key: "dates.pickup_at", Group: "dates", Sample: "15.10.2026 10:00"},
	{Key: "dates.return_at", Group: "dates", Sample: "18.10.2026 10:00"},
	{Key: "dates.actual_pickup_at", Group: "dates", Sample: "15.10.2026 10:12"},
	{Key: "dates.actual_return_at", Group: "dates", Sample: "18.10.2026 09:40"},
	{Key: "dates.billing_days", Group: "dates", Sample: "3"},

	{Key: "points.pickup_branch", Group: "points", Sample: "Kadıköy Şubesi"},
	{Key: "points.return_branch", Group: "points", Sample: "Sabiha Gökçen Havalimanı"},
	{Key: "points.pickup_point", Group: "points", Sample: "Otel teslim noktası"},
	{Key: "points.return_point", Group: "points", Sample: ""},
	{Key: "points.actual_return_branch", Group: "points", Sample: "Sabiha Gökçen Havalimanı"},

	{Key: "amounts.total", Group: "amounts", Sample: "4.500,00 TRY"},
	{Key: "amounts.prepayment", Group: "amounts", Sample: "1.000,00 TRY"},
	{Key: "amounts.paid", Group: "amounts", Sample: "1.000,00 TRY"},
	{Key: "amounts.balance", Group: "amounts", Sample: "3.500,00 TRY"},
	{Key: "amounts.extensions_total", Group: "amounts", Sample: "0,00 TRY"},
	{Key: "amounts.currency", Group: "amounts", Sample: "TRY"},

	{Key: "extras.table", Group: "extras", HTML: true, Sample: ""},

	{Key: "policies.cancellation", Group: "policies", Sample: "Teslimden 24 saat öncesine kadar ücretsiz iptal."},
	{Key: "policies.no_show", Group: "policies", Sample: "Gelmeme durumunda ön ödeme iade edilmez."},
	{Key: "policies.early_return", Group: "policies", Sample: "Erken iadede kullanılmayan günler iade edilmez."},
	{Key: "policies.extension", Group: "policies", Sample: "Uzatma, müsaitlik ve güncel fiyatla yapılır."},
	{Key: "policies.fuel", Group: "policies", Sample: "Araç aynı yakıt seviyesiyle iade edilir."},
	{Key: "policies.mileage", Group: "policies", Sample: "Günlük 300 km dahildir."},
	{Key: "policies.late_return_grace_minutes", Group: "policies", Sample: "59"},
	{Key: "policies.extra_hour_fee", Group: "policies", Sample: "150,00 TRY"},

	{Key: "document.version", Group: "document", Sample: "1"},
	{Key: "document.date", Group: "document", Sample: "15.10.2026"},
}

// PlaceholderGroups in picker order.
var PlaceholderGroups = []string{
	"company", "reservation", "renter", "invoice_customer", "drivers", "vehicle", "product", "dates", "points",
	"amounts", "extras", "policies", "document",
}

var catalogIndex = func() map[string]Placeholder {
	m := make(map[string]Placeholder, len(Catalog))
	for _, p := range Catalog {
		m[p.Key] = p
	}
	return m
}()

// LookupPlaceholder returns the catalog entry of a key.
func LookupPlaceholder(key string) (Placeholder, bool) {
	p, ok := catalogIndex[key]
	return p, ok
}

// VolatileKeys are rendered per version and never decide whether a new
// version is needed.
var VolatileKeys = map[string]bool{"document.version": true, "document.date": true}
