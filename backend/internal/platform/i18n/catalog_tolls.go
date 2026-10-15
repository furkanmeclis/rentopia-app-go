package i18n

// Toll passes (TEC-725): export / import labels, enum values and the
// resource names. Kept in their own file and merged into the catalogs.

var trTolls = map[string]string{
	"tolls.passed_at": "Geçiş zamanı", "tolls.plate": "Plaka", "tolls.tag_number": "Etiket no",
	"tolls.station": "İstasyon / giriş", "tolls.exit_station": "Çıkış", "tolls.amount": "Tutar",
	"tolls.currency": "Para birimi", "tolls.fee_amount": "İşlem ücreti", "tolls.status": "Durum",
	"tolls.review_reason": "İnceleme nedeni", "tolls.rental": "Kiralama", "tolls.vehicle": "Araç",
	"tolls.provider": "Sağlayıcı", "tolls.external_id": "İşlem no", "tolls.source": "Kaynak",
	"tolls.revision": "Revizyon", "tolls.uuid": "UUID",
	"tolls.status.unmatched": "Eşleşmedi", "tolls.status.needs_review": "İnceleme bekliyor",
	"tolls.status.matched": "Eşleşti", "tolls.status.charged": "Masraf yansıtıldı", "tolls.status.ignored": "Yok sayıldı",
	"tolls.source.api": "Sağlayıcı", "tolls.source.file": "Dosya",
	"tolls.review_reason.vehicle_unknown":       "Araç bulunamadı",
	"tolls.review_reason.vehicle_conflict":      "Plaka ve etiket farklı araçlar",
	"tolls.review_reason.no_rental":             "Kiralama dışında",
	"tolls.review_reason.overlap":               "Birden çok kiralama",
	"tolls.review_reason.missing_actual_times":  "Teslim zamanı girilmemiş",
	"tolls.review_reason.outside_actual_window": "Gerçek teslim–iade dışında",
	"tolls.review_reason.in_transfer":           "Araç transferde",
	"tolls.review_reason.rental_cancelled":      "Kiralama iptal edildi",
	"tolls.review_reason.unassigned":            "Atama kaldırıldı",
	"export.title.tenant.toll_passes":           "HGS/OGS geçişleri dışa aktarma",
	"resources.tenant.toll_passes":              "HGS/OGS geçişleri",
}

var enTolls = map[string]string{
	"tolls.passed_at": "Pass time", "tolls.plate": "Plate", "tolls.tag_number": "Tag no",
	"tolls.station": "Station / entry", "tolls.exit_station": "Exit", "tolls.amount": "Amount",
	"tolls.currency": "Currency", "tolls.fee_amount": "Processing fee", "tolls.status": "Status",
	"tolls.review_reason": "Review reason", "tolls.rental": "Rental", "tolls.vehicle": "Vehicle",
	"tolls.provider": "Provider", "tolls.external_id": "Transaction no", "tolls.source": "Source",
	"tolls.revision": "Revision", "tolls.uuid": "UUID",
	"tolls.status.unmatched": "Unmatched", "tolls.status.needs_review": "Needs review",
	"tolls.status.matched": "Matched", "tolls.status.charged": "Charged", "tolls.status.ignored": "Ignored",
	"tolls.source.api": "Provider", "tolls.source.file": "File",
	"tolls.review_reason.vehicle_unknown":       "Vehicle not found",
	"tolls.review_reason.vehicle_conflict":      "Plate and tag name different vehicles",
	"tolls.review_reason.no_rental":             "Outside any rental",
	"tolls.review_reason.overlap":               "Several rentals",
	"tolls.review_reason.missing_actual_times":  "Handover time missing",
	"tolls.review_reason.outside_actual_window": "Outside the actual handover–return",
	"tolls.review_reason.in_transfer":           "Vehicle in transfer",
	"tolls.review_reason.rental_cancelled":      "Rental cancelled",
	"tolls.review_reason.unassigned":            "Assignment removed",
	"export.title.tenant.toll_passes":           "Toll passes export",
	"resources.tenant.toll_passes":              "Toll passes",
}

func init() {
	for k, v := range trTolls {
		trCatalog[k] = v
	}
	for k, v := range enTolls {
		enCatalog[k] = v
	}
}
