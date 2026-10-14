package tsbcatalog

import (
	"strings"
	"unicode"
)

// Brand categories (match the catalog_brands CHECK).
const (
	CategoryPassenger       = "passenger"
	CategoryLightCommercial = "light_commercial"
	CategoryOther           = "other"
)

// BrandDefault is the initial display name / category for a TSB brand name.
// Applied only when a brand is first created; admin edits are kept.
type BrandDefault struct {
	DisplayName string
	Category    string
}

// knownBrands maps TSB brand names (upper case, as in the list) to their
// display name and category. Brands missing here get a title-cased name and
// the passenger category.
var knownBrands = map[string]BrandDefault{
	// TSB-specific spellings
	`TOFAS-FIAT`:        {"Fiat", CategoryPassenger},
	`RENAULT (OYAK)`:    {"Renault", CategoryPassenger},
	`FORD /USA`:         {"Ford", CategoryPassenger},
	`DODGE/USA`:         {"Dodge", CategoryPassenger},
	`VOLVO-TR`:          {"Volvo", CategoryPassenger},
	`OTOYOL\IVECO\FIAT`: {"Iveco", CategoryLightCommercial},
	`MERCEDES`:          {"Mercedes-Benz", CategoryPassenger},
	`KGMOBILITY`:        {"KG Mobility", CategoryPassenger},
	`LYNK-CO`:           {"Lynk & Co", CategoryPassenger},
	`TRUMPCHI/GAC`:      {"GAC", CategoryPassenger},
	`OTOKAR/MAGIRUS`:    {"Otokar", CategoryOther},
	`DONG FENG`:         {"Dongfeng", CategoryPassenger},
	`MCLAREN`:           {"McLaren", CategoryPassenger},
	`FARIZON AUTO`:      {"Farizon", CategoryLightCommercial},
	`TADANO FAUN`:       {"Tadano Faun", CategoryOther},
	// acronyms / stylized names
	`BMW`:  {"BMW", CategoryPassenger},
	`MG`:   {"MG", CategoryPassenger},
	`BYD`:  {"BYD", CategoryPassenger},
	`DS`:   {"DS", CategoryPassenger},
	`GMC`:  {"GMC", CategoryPassenger},
	`JAC`:  {"JAC", CategoryPassenger},
	`FAW`:  {"FAW", CategoryPassenger},
	`DFM`:  {"DFM", CategoryPassenger},
	`IKCO`: {"IKCO", CategoryPassenger},
	`LEVC`: {"LEVC", CategoryPassenger},
	`SWM`:  {"SWM", CategoryPassenger},
	`XEV`:  {"XEV", CategoryPassenger},
	`MINI`: {"MINI", CategoryPassenger},
	`SEAT`: {"SEAT", CategoryPassenger},
	`TOGG`: {"Togg", CategoryPassenger},
	`ORA`:  {"ORA", CategoryPassenger},
	`ROX`:  {"ROX", CategoryPassenger},
	// light commercial
	`DFSK`:     {"DFSK", CategoryLightCommercial},
	`MAXUS`:    {"Maxus", CategoryLightCommercial},
	`ISUZU`:    {"Isuzu", CategoryLightCommercial},
	`KARSAN`:   {"Karsan", CategoryLightCommercial},
	`CENNTRO`:  {"Cenntro", CategoryLightCommercial},
	`PIAGGIO`:  {"Piaggio", CategoryLightCommercial},
	`ALKE`:     {"Alke", CategoryLightCommercial},
	`MULTICAR`: {"Multicar", CategoryLightCommercial},
	`GAZ`:      {"GAZ", CategoryLightCommercial},
	`FOTON`:    {"Foton", CategoryLightCommercial},
	`TATA`:     {"Tata", CategoryLightCommercial},
	// motorcycles, tractors, caravans / motorhomes
	`MOTORSIKLET`:   {"Motosiklet", CategoryOther},
	`ZIRAI TRAKTOR`: {"Zirai Traktör", CategoryOther},
	`HYMER`:         {"Hymer", CategoryOther},
	`KNAUS`:         {"Knaus", CategoryOther},
	`ADRIA`:         {"Adria", CategoryOther},
	`HOBBY`:         {"Hobby", CategoryOther},
	`CARTHAGO`:      {"Carthago", CategoryOther},
	`LAIKA`:         {"Laika", CategoryOther},
	`WEINSBERG`:     {"Weinsberg", CategoryOther},
	`ETRUSCO`:       {"Etrusco", CategoryOther},
	`MILLER`:        {"Miller", CategoryOther},
	// buses, trucks, construction machinery
	`SCANIA`:         {"Scania", CategoryOther},
	`MAN`:            {"MAN", CategoryOther},
	`TEMSA`:          {"Temsa", CategoryOther},
	`SETRA`:          {"Setra", CategoryOther},
	`NEOPLAN`:        {"Neoplan", CategoryOther},
	`DAF`:            {"DAF", CategoryOther},
	`KAMAZ`:          {"Kamaz", CategoryOther},
	`MAZ`:            {"MAZ", CategoryOther},
	`BMC`:            {"BMC", CategoryOther},
	`GULERYUZ`:       {"Güleryüz", CategoryOther},
	`IRIZAR`:         {"Irizar", CategoryOther},
	`BREDAMENARIBUS`: {"BredaMenarinibus", CategoryOther},
	`MENARINIBUS`:    {"Menarinibus", CategoryOther},
	`SOLARIS`:        {"Solaris", CategoryOther},
	`BOZANKAYA`:      {"Bozankaya", CategoryOther},
	`ISOBUS`:         {"Isobus", CategoryOther},
	`KENWORTH`:       {"Kenworth", CategoryOther},
	`SINOTRUK`:       {"Sinotruk", CategoryOther},
	`SITRAK`:         {"Sitrak", CategoryOther},
	`TATRA`:          {"Tatra", CategoryOther},
	`GROVE`:          {"Grove", CategoryOther},
	`SANY`:           {"Sany", CategoryOther},
	`ZOOMLION`:       {"Zoomlion", CategoryOther},
	`PIMAKINA`:       {"Pimakina", CategoryOther},
	`CRRC`:           {"CRRC", CategoryOther},
	`AVIA`:           {"Avia", CategoryOther},
	`TCV`:            {"TCV", CategoryOther},
	`AKIA`:           {"Akia", CategoryOther},
	`ASTRA`:          {"Astra", CategoryOther},
	`SCHMIDT`:        {"Schmidt", CategoryOther},
	`HABAS`:          {"Habaş", CategoryOther},
	`TEZELLER`:       {"Tezeller", CategoryOther},
	`TURKKAR`:        {"Türkkar", CategoryOther},
	`TURKAR`:         {"Turkar", CategoryOther},
	`EMT`:            {"EMT", CategoryOther},
	`ULTRAND`:        {"Ultrand", CategoryOther},
	`HISCAR`:         {"Hiscar", CategoryOther},
	`SSANGYONG`:      {"SsangYong", CategoryPassenger},
}

// DefaultsForBrand returns the display name and category for a TSB brand.
func DefaultsForBrand(tsbName string) BrandDefault {
	key := strings.Join(strings.Fields(upper(tsbName)), " ")
	if d, ok := knownBrands[key]; ok {
		return d
	}
	return BrandDefault{DisplayName: TitleCase(tsbName), Category: CategoryPassenger}
}

// TitleCase turns "ALFA ROMEO" into "Alfa Romeo" and "ROLLS-ROYCE" into
// "Rolls-Royce" using Turkish-aware lower casing of ASCII letters.
func TitleCase(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	var b strings.Builder
	upperNext := true
	for _, r := range s {
		if unicode.IsLetter(r) {
			if upperNext {
				b.WriteRune(unicode.ToUpper(r))
			} else {
				b.WriteRune(unicode.ToLower(r))
			}
			upperNext = false
			continue
		}
		b.WriteRune(r)
		upperNext = r == ' ' || r == '-' || r == '/' || r == '(' || r == '&'
	}
	return b.String()
}
