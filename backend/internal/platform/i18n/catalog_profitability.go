package i18n

// Vehicle profitability report (TEC-727): export columns, method labels
// and the method notes printed above the exported table. Kept in its own
// file and merged into the catalogs at init so parallel modules do not
// edit the same map literal.

var trProfitability = map[string]string{
	"vehicle_profitability.plate":                    "Plaka",
	"vehicle_profitability.home_branch":              "Bağlı şube",
	"vehicle_profitability.rental_days":              "Kiralama günü",
	"vehicle_profitability.occupancy":                "Doluluk (%)",
	"vehicle_profitability.gross_income":             "Brüt gelir",
	"vehicle_profitability.commissions":              "Komisyonlar",
	"vehicle_profitability.direct_costs":             "Doğrudan gider",
	"vehicle_profitability.overhead":                 "Paylaştırılan gider",
	"vehicle_profitability.contribution":             "Operasyonel katkı",
	"vehicle_profitability.margin":                   "Katkı marjı (%)",
	"vehicle_profitability.refunded":                 "İade edilen tahsilat",
	"vehicle_profitability.deposits_held":            "Alınan depozito",
	"vehicle_profitability.pass_through":             "Yansıtılan masraf (HGS)",
	"vehicle_profitability.basis":                    "Hesap yöntemi",
	"vehicle_profitability.allocation":               "Paylaştırma yöntemi",
	"vehicle_profitability.period":                   "Dönem",
	"vehicle_profitability.total":                    "TOPLAM",
	"vehicle_profitability.basis_label":              "Hesap yöntemi",
	"vehicle_profitability.allocation_label":         "Ortak gider paylaştırma",
	"vehicle_profitability.basis.accrual":            "Tahakkuk esaslı",
	"vehicle_profitability.basis.cash":               "Nakit esaslı",
	"vehicle_profitability.allocation.rental_days":   "Kiralama gününe göre",
	"vehicle_profitability.allocation.vehicle_count": "Araç sayısına göre",
	"vehicle_profitability.allocation.revenue":       "Gelir payına göre",
	"vehicle_profitability.note.amounts": "Tutarlar TRY ve KDV dahildir; günler Europe/Istanbul takvimine göredir. " +
		"Araç değişimi olan kiralamaların geliri araçlara kullanım sürelerine göre bölünür.",
	"vehicle_profitability.note.basis.accrual": "Tahakkuk esaslı: dönem içinde kayda giren kira ve ek ücret tahakkukları, " +
		"ters kayıtlar düşülerek; giderler belge tarihine göre.",
	"vehicle_profitability.note.basis.cash": "Nakit esaslı: dönem içindeki tahsilatlar (iptal ve iadeler düşülerek, kiralamanın " +
		"tahakkuk dağılımına göre sınıflandırılır), dönem içindeki giderler ve tedarikçi ödemeleri.",
	"vehicle_profitability.note.allocation.rental_days": "Ortak giderler (araca bağlanmamış şube ve merkez giderleri) her ay, " +
		"giderin şubesindeki (merkez giderleri tüm filodaki) araçlara kiralama günlerine göre paylaştırılır.",
	"vehicle_profitability.note.allocation.vehicle_count": "Ortak giderler (araca bağlanmamış şube ve merkez giderleri) her ay, " +
		"giderin şubesindeki (merkez giderleri tüm filodaki) hizmetteki araçlara eşit paylaştırılır.",
	"vehicle_profitability.note.allocation.revenue": "Ortak giderler (araca bağlanmamış şube ve merkez giderleri) her ay, " +
		"giderin şubesindeki (merkez giderleri tüm filodaki) araçlara brüt gelir paylarına göre paylaştırılır.",
	"vehicle_profitability.note.kept_apart": "İade edilen tahsilatlar, depozitolar, müşteriye yansıtılan HGS geçiş bedelleri " +
		"ve araç edinim giderleri gelir ya da gider sayılmaz; ayrı gösterilir.",
	"vehicle_profitability.note.overhead_branches": "Yalnızca yetkili olduğunuz şubelerin ortak giderleri dahildir; merkez giderleri dahil değildir.",
	"vehicle_profitability.note.fallback":          "Seçilen yöntemin ağırlığı olmayan aylarda ortak giderler araç sayısına göre paylaştırıldı.",
	"vehicle_profitability.note.unallocated":       "Hizmette aracı olmayan şubelerin paylaştırılamayan ortak gideri:",
}

var enProfitability = map[string]string{
	"vehicle_profitability.plate":                    "Plate",
	"vehicle_profitability.home_branch":              "Home branch",
	"vehicle_profitability.rental_days":              "Rental days",
	"vehicle_profitability.occupancy":                "Occupancy (%)",
	"vehicle_profitability.gross_income":             "Gross income",
	"vehicle_profitability.commissions":              "Commissions",
	"vehicle_profitability.direct_costs":             "Direct costs",
	"vehicle_profitability.overhead":                 "Allocated overhead",
	"vehicle_profitability.contribution":             "Operating contribution",
	"vehicle_profitability.margin":                   "Contribution margin (%)",
	"vehicle_profitability.refunded":                 "Refunded collections",
	"vehicle_profitability.deposits_held":            "Deposits taken",
	"vehicle_profitability.pass_through":             "Pass-through charges (tolls)",
	"vehicle_profitability.basis":                    "Basis",
	"vehicle_profitability.allocation":               "Allocation method",
	"vehicle_profitability.period":                   "Period",
	"vehicle_profitability.total":                    "TOTAL",
	"vehicle_profitability.basis_label":              "Basis",
	"vehicle_profitability.allocation_label":         "Overhead allocation",
	"vehicle_profitability.basis.accrual":            "Accrual basis",
	"vehicle_profitability.basis.cash":               "Cash basis",
	"vehicle_profitability.allocation.rental_days":   "By rental days",
	"vehicle_profitability.allocation.vehicle_count": "By vehicle count",
	"vehicle_profitability.allocation.revenue":       "By revenue share",
	"vehicle_profitability.note.amounts": "Amounts are TRY, VAT inclusive; days follow the Europe/Istanbul calendar. " +
		"A rental with a vehicle swap splits its income across the vehicles by time on each leg.",
	"vehicle_profitability.note.basis.accrual": "Accrual basis: rent and charge accruals posted in the period, net of reversals; " +
		"expenses by document date.",
	"vehicle_profitability.note.basis.cash": "Cash basis: collections in the period (net of voids and refunds, classified by the " +
		"rental's accruals), expenses and supplier payments in the period.",
	"vehicle_profitability.note.allocation.rental_days": "Shared overhead (branch and head-office expenses without a vehicle) is " +
		"allocated each month to the vehicles of the expense's branch (the whole fleet for head office) by rental days.",
	"vehicle_profitability.note.allocation.vehicle_count": "Shared overhead (branch and head-office expenses without a vehicle) is " +
		"allocated each month equally to the vehicles in service of the expense's branch (the whole fleet for head office).",
	"vehicle_profitability.note.allocation.revenue": "Shared overhead (branch and head-office expenses without a vehicle) is " +
		"allocated each month to the vehicles of the expense's branch (the whole fleet for head office) by gross income share.",
	"vehicle_profitability.note.kept_apart": "Refunded collections, deposits, tolls re-billed to customers and vehicle " +
		"procurement are neither income nor cost; they are shown apart.",
	"vehicle_profitability.note.overhead_branches": "Only the shared overhead of your branches is included; head-office overhead is not.",
	"vehicle_profitability.note.fallback":          "In months where the chosen method had no weight, overhead was allocated by vehicle count.",
	"vehicle_profitability.note.unallocated":       "Overhead of branches without a vehicle in service, not allocated:",
}

func init() {
	for k, v := range trProfitability {
		trCatalog[k] = v
	}
	for k, v := range enProfitability {
		enCatalog[k] = v
	}
}
