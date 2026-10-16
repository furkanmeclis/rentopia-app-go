package i18n

// Platform usage and connection health (TEC-739): the failed-work alert
// sent to platform administrators and the health source names it lists.
// A module that registers its own health source adds
// "platform_health.source.<code>" in its own catalog file.

var trPlatformHealth = map[string]string{
	"platform_health.alert.title":                 "Bağlantı sağlığı: {count} yeni kritik sorun",
	"platform_health.alert.body":                  "Yeni sorunlar: {details}. Ayrıntılar platform yönetimindeki kullanım ve sağlık panosunda.",
	"platform_health.source.integration_accounts": "Entegrasyon hesapları",
	"platform_health.source.integration_jobs":     "Entegrasyon işleri",
}

var enPlatformHealth = map[string]string{
	"platform_health.alert.title":                 "Connection health: {count} new critical issues",
	"platform_health.alert.body":                  "New issues: {details}. See the usage and health dashboard in platform management.",
	"platform_health.source.integration_accounts": "Integration accounts",
	"platform_health.source.integration_jobs":     "Integration jobs",
}

func init() {
	for k, v := range trPlatformHealth {
		trCatalog[k] = v
	}
	for k, v := range enPlatformHealth {
		enCatalog[k] = v
	}
}
