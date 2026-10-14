package i18n

import "strings"

// Locale is a supported BCP-47 short code.
type Locale string

const (
	LocaleTR Locale = "tr"
	LocaleEN Locale = "en"
)

// Normalize returns a supported locale or the fallback.
func Normalize(locale string) Locale {
	switch strings.ToLower(strings.TrimSpace(locale)) {
	case "en":
		return LocaleEN
	default:
		return LocaleTR
	}
}

var catalogs = map[Locale]map[string]string{
	LocaleTR: trCatalog,
	LocaleEN: enCatalog,
}

// Translate resolves a dotted key for the locale with en fallback.
func Translate(locale Locale, key string) string {
	if v, ok := catalogs[locale][key]; ok && v != "" {
		return v
	}
	if locale != LocaleEN {
		if v, ok := catalogs[LocaleEN][key]; ok {
			return v
		}
	}
	return key
}

var trCatalog = map[string]string{
	"users.uuid": "UUID", "users.email": "E-posta", "users.name": "Ad",
	"users.surname": "Soyad", "users.status": "Durum", "users.created_at": "Oluşturulma",
	"users.locale": "Dil", "users.role_slugs": "Roller", "users.role": "Rol",
	"roles.uuid": "UUID", "roles.name": "Ad", "roles.slug": "Slug",
	"roles.is_system": "Sistem rolü", "roles.created_at": "Oluşturulma",
	"roles.description": "Açıklama", "roles.permission_slugs": "İzinler",
	"organizations.uuid": "UUID", "organizations.name": "Ad", "organizations.slug": "Slug",
	"organizations.status": "Durum", "organizations.access_ends_at": "Erişim bitişi",
	"organizations.created_at": "Oluşturulma",
	"notifications.uuid":       "UUID", "notifications.channel": "Kanal",
	"notifications.status": "Durum", "notifications.priority": "Öncelik",
	"notifications.title": "Başlık", "notifications.created_at": "Oluşturulma",
	"notifications.read_at": "Okunma", "notifications.unread": "Okunmamış",
	"activity.action": "İşlem", "activity.resource": "Kaynak",
	"activity.actor": "Kullanıcı", "activity.created_at": "Tarih",
	"common.yes": "Evet", "common.no": "Hayır",
	"users.status.active": "Aktif", "users.status.disabled": "Pasif", "users.status.pending": "Beklemede",
	"export.document":                     "Dışa aktarma",
	"export.generated_at":                 "Oluşturulma",
	"export.title.platform.users":         "Kullanıcılar dışa aktarma",
	"export.title.platform.roles":         "Roller dışa aktarma",
	"export.title.platform.notifications": "Bildirimler dışa aktarma",
	"export.title.platform.activity":      "Etkinlik günlüğü dışa aktarma",
	"resources.platform.users":            "Kullanıcılar",
	"resources.platform.roles":            "Roller",
	"resources.platform.notifications":    "Bildirimler",
	"resources.platform.activity":         "Etkinlik günlüğü",
	"branches.uuid":                       "UUID", "branches.code": "Kod", "branches.name": "Ad",
	"branches.city": "İl", "branches.district": "İlçe", "branches.address": "Adres",
	"branches.phone": "Telefon", "branches.email": "E-posta", "branches.latitude": "Enlem",
	"branches.longitude": "Boylam", "branches.prep_minutes": "Hazırlık süresi (dk)",
	"branches.is_active": "Aktif", "branches.created_at": "Oluşturulma",
	"export.title.tenant.branches": "Şubeler dışa aktarma",
	"resources.tenant.branches":    "Şubeler",
	"fleet.uuid":                   "UUID", "fleet.code": "Kod", "fleet.name": "Ad", "fleet.title": "Başlık",
	"fleet.description": "Açıklama", "fleet.sort_order": "Sıra", "fleet.groups_count": "Kiralama grubu",
	"fleet.products_count": "Ürün", "fleet.is_active": "Aktif", "fleet.created_at": "Oluşturulma",
	"fleet.segment": "Segment", "fleet.transmission": "Vites", "fleet.fuel": "Yakıt",
	"fleet.body_type": "Kasa tipi", "fleet.min_seats": "En az koltuk", "fleet.min_luggage": "En az bagaj",
	"fleet.air_conditioning": "Klima", "fleet.acriss_code": "ACRISS", "fleet.catalog_models": "Katalog modelleri",
	"fleet.catalog_types": "Katalog tipleri", "fleet.status": "Durum", "fleet.guarantee_type": "Taahhüt",
	"fleet.display_text": "Gösterim metni", "fleet.current_version": "Sürüm",
	"fleet.deposit_amount": "Depozito", "fleet.min_driver_age": "En az sürücü yaşı",
	"fleet.min_license_years": "En az ehliyet yılı", "fleet.included_km_per_day": "Günlük km",
	"fleet.extra_km_fee": "Ek km ücreti", "fleet.fuel_policy": "Yakıt politikası",
	"export.title.tenant.segments":        "Segmentler dışa aktarma",
	"export.title.tenant.rental_groups":   "Kiralama grupları dışa aktarma",
	"export.title.tenant.rental_products": "Ürünler dışa aktarma",
	"resources.tenant.segments":           "Segmentler",
	"resources.tenant.rental_groups":      "Kiralama grupları",
	"resources.tenant.rental_products":    "Ürünler",
	"vehicles.uuid":                       "UUID", "vehicles.plate": "Plaka", "vehicles.vin": "Şasi no",
	"vehicles.brand": "Marka", "vehicles.model": "Model", "vehicles.type_name": "Tip",
	"vehicles.type_code": "Kasko kodu", "vehicles.model_year": "Model yılı", "vehicles.color": "Renk",
	"vehicles.transmission": "Vites", "vehicles.fuel": "Yakıt", "vehicles.body_type": "Kasa tipi",
	"vehicles.seats": "Koltuk", "vehicles.luggage": "Bagaj", "vehicles.branch": "Şube",
	"vehicles.home_branch": "Bağlı şube", "vehicles.current_branch": "Bulunduğu şube",
	"vehicles.fleet_pool": "Filo havuzu", "vehicles.ownership": "Mülkiyet", "vehicles.supplier_name": "Tedarikçi",
	"vehicles.odometer_km": "Km", "vehicles.fuel_level": "Yakıt seviyesi (%)", "vehicles.status": "Durum",
	"vehicles.kasko_value": "Kasko değeri", "vehicles.kasko_period": "Kasko dönemi",
	"vehicles.rental_groups": "Kiralama grupları", "vehicles.in_service_date": "Hizmete giriş",
	"vehicles.out_of_service_date": "Hizmetten çıkış", "vehicles.is_active": "Aktif",
	"vehicles.created_at":          "Oluşturulma",
	"export.title.tenant.vehicles": "Araçlar dışa aktarma",
	"resources.tenant.vehicles":    "Araçlar",
	"export.format.pdf":            "PDF",
	"export.format.xlsx":           "Excel",
	"export.format.csv":            "CSV",
	"export.format.json":           "JSON",
}

var enCatalog = map[string]string{
	"users.uuid": "UUID", "users.email": "Email", "users.name": "First name",
	"users.surname": "Last name", "users.status": "Status", "users.created_at": "Created",
	"users.locale": "Locale", "users.role_slugs": "Roles", "users.role": "Role",
	"roles.uuid": "UUID", "roles.name": "Name", "roles.slug": "Slug",
	"roles.is_system": "System role", "roles.created_at": "Created",
	"roles.description": "Description", "roles.permission_slugs": "Permissions",
	"organizations.uuid": "UUID", "organizations.name": "Name", "organizations.slug": "Slug",
	"organizations.status": "Status", "organizations.access_ends_at": "Access ends",
	"organizations.created_at": "Created",
	"notifications.uuid":       "UUID", "notifications.channel": "Channel",
	"notifications.status": "Status", "notifications.priority": "Priority",
	"notifications.title": "Title", "notifications.created_at": "Created",
	"notifications.read_at": "Read at", "notifications.unread": "Unread",
	"activity.action": "Action", "activity.resource": "Resource",
	"activity.actor": "User", "activity.created_at": "Date",
	"common.yes": "Yes", "common.no": "No",
	"users.status.active": "Active", "users.status.disabled": "Disabled", "users.status.pending": "Pending",
	"export.document":                     "Export",
	"export.generated_at":                 "Generated",
	"export.title.platform.users":         "Users export",
	"export.title.platform.roles":         "Roles export",
	"export.title.platform.notifications": "Notifications export",
	"export.title.platform.activity":      "Activity log export",
	"resources.platform.users":            "Users",
	"resources.platform.roles":            "Roles",
	"resources.platform.notifications":    "Notifications",
	"resources.platform.activity":         "Activity log",
	"branches.uuid":                       "UUID", "branches.code": "Code", "branches.name": "Name",
	"branches.city": "City", "branches.district": "District", "branches.address": "Address",
	"branches.phone": "Phone", "branches.email": "Email", "branches.latitude": "Latitude",
	"branches.longitude": "Longitude", "branches.prep_minutes": "Preparation time (min)",
	"branches.is_active": "Active", "branches.created_at": "Created",
	"export.title.tenant.branches": "Branches export",
	"resources.tenant.branches":    "Branches",
	"fleet.uuid":                   "UUID", "fleet.code": "Code", "fleet.name": "Name", "fleet.title": "Title",
	"fleet.description": "Description", "fleet.sort_order": "Order", "fleet.groups_count": "Rental groups",
	"fleet.products_count": "Products", "fleet.is_active": "Active", "fleet.created_at": "Created",
	"fleet.segment": "Segment", "fleet.transmission": "Transmission", "fleet.fuel": "Fuel",
	"fleet.body_type": "Body type", "fleet.min_seats": "Min. seats", "fleet.min_luggage": "Min. luggage",
	"fleet.air_conditioning": "Air conditioning", "fleet.acriss_code": "ACRISS", "fleet.catalog_models": "Catalog models",
	"fleet.catalog_types": "Catalog types", "fleet.status": "Status", "fleet.guarantee_type": "Guarantee",
	"fleet.display_text": "Display text", "fleet.current_version": "Version",
	"fleet.deposit_amount": "Deposit", "fleet.min_driver_age": "Min. driver age",
	"fleet.min_license_years": "Min. license years", "fleet.included_km_per_day": "Km per day",
	"fleet.extra_km_fee": "Extra km fee", "fleet.fuel_policy": "Fuel policy",
	"export.title.tenant.segments":        "Segments export",
	"export.title.tenant.rental_groups":   "Rental groups export",
	"export.title.tenant.rental_products": "Products export",
	"resources.tenant.segments":           "Segments",
	"resources.tenant.rental_groups":      "Rental groups",
	"resources.tenant.rental_products":    "Products",
	"vehicles.uuid":                       "UUID", "vehicles.plate": "Plate", "vehicles.vin": "VIN",
	"vehicles.brand": "Brand", "vehicles.model": "Model", "vehicles.type_name": "Type",
	"vehicles.type_code": "Kasko code", "vehicles.model_year": "Model year", "vehicles.color": "Color",
	"vehicles.transmission": "Transmission", "vehicles.fuel": "Fuel", "vehicles.body_type": "Body type",
	"vehicles.seats": "Seats", "vehicles.luggage": "Luggage", "vehicles.branch": "Branch",
	"vehicles.home_branch": "Home branch", "vehicles.current_branch": "Current branch",
	"vehicles.fleet_pool": "Fleet pool", "vehicles.ownership": "Ownership", "vehicles.supplier_name": "Supplier",
	"vehicles.odometer_km": "Km", "vehicles.fuel_level": "Fuel level (%)", "vehicles.status": "Status",
	"vehicles.kasko_value": "Kasko value", "vehicles.kasko_period": "Kasko period",
	"vehicles.rental_groups": "Rental groups", "vehicles.in_service_date": "In service from",
	"vehicles.out_of_service_date": "Out of service from", "vehicles.is_active": "Active",
	"vehicles.created_at":          "Created",
	"export.title.tenant.vehicles": "Vehicles export",
	"resources.tenant.vehicles":    "Vehicles",
	"export.format.pdf":            "PDF",
	"export.format.xlsx":           "Excel",
	"export.format.csv":            "CSV",
	"export.format.json":           "JSON",
}
