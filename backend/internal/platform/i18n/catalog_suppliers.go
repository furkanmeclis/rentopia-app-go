package i18n

// Suppliers, supplier invoices and payments (TEC-721): export / import
// labels and enum values. Kept in their own maps (merged at init) so the
// main catalog stays an additive merge.

var trSuppliers = map[string]string{
	"export.title.tenant.suppliers":         "Tedarikçiler dışa aktarma",
	"resources.tenant.suppliers":            "Tedarikçiler",
	"export.title.tenant.supplier_invoices": "Gider faturaları dışa aktarma",
	"resources.tenant.supplier_invoices":    "Gider faturaları",
	"export.title.tenant.supplier_payments": "Tedarikçi ödemeleri dışa aktarma",
	"resources.tenant.supplier_payments":    "Tedarikçi ödemeleri",

	"suppliers.code": "Kod", "suppliers.title": "Unvan", "suppliers.supplier_type": "Tür",
	"suppliers.tax_number": "VKN / TCKN", "suppliers.tax_office": "Vergi dairesi", "suppliers.phone": "Telefon",
	"suppliers.email": "E-posta", "suppliers.website": "Web sitesi", "suppliers.address": "Adres", "suppliers.city": "Şehir",
	"suppliers.billing_address": "Fatura adresi", "suppliers.billing_email": "Fatura e-postası",
	"suppliers.e_invoice": "E-fatura mükellefi", "suppliers.bank_name": "Banka", "suppliers.iban": "IBAN",
	"suppliers.payment_term_days": "Vade (gün)", "suppliers.status": "Durum", "suppliers.note": "Not",
	"suppliers.contacts": "Yetkililer", "suppliers.payable": "Borcumuz", "suppliers.open_amount": "Açık fatura tutarı",
	"suppliers.overdue_amount": "Vadesi geçen", "suppliers.uuid": "UUID",
	"suppliers.type.service": "Servis", "suppliers.type.insurance": "Sigorta",
	"suppliers.type.vehicle_supplier": "Araç tedarikçisi", "suppliers.type.parts": "Yedek parça",
	"suppliers.type.fuel": "Yakıt", "suppliers.type.other": "Diğer",
	"suppliers.status.active": "Aktif", "suppliers.status.archived": "Arşivde",

	"supplier_invoices.invoice_no": "Fatura no", "supplier_invoices.supplier_code": "Tedarikçi kodu",
	"supplier_invoices.supplier": "Tedarikçi", "supplier_invoices.invoice_date": "Fatura tarihi",
	"supplier_invoices.due_date": "Vade tarihi", "supplier_invoices.category": "Kategori",
	"supplier_invoices.branch_code": "Şube kodu", "supplier_invoices.plate": "Plaka",
	"supplier_invoices.net_amount": "Net tutar", "supplier_invoices.vat_amount": "KDV",
	"supplier_invoices.total_amount": "Toplam", "supplier_invoices.paid_amount": "Ödenen",
	"supplier_invoices.open_amount": "Kalan", "supplier_invoices.currency": "Para birimi",
	"supplier_invoices.due_state": "Vade durumu", "supplier_invoices.status": "Durum",
	"supplier_invoices.description": "Açıklama", "supplier_invoices.uuid": "UUID",

	"supplier_payments.payment_date": "Ödeme tarihi", "supplier_payments.supplier_code": "Tedarikçi kodu",
	"supplier_payments.supplier": "Tedarikçi", "supplier_payments.cash_account_code": "Kasa / banka kodu",
	"supplier_payments.method": "Ödeme şekli", "supplier_payments.reference": "Referans",
	"supplier_payments.amount": "Tutar", "supplier_payments.allocated_amount": "Faturalara dağıtılan",
	"supplier_payments.unallocated_amount": "Avans (dağıtılmayan)", "supplier_payments.currency": "Para birimi",
	"supplier_payments.invoices": "Faturalar", "supplier_payments.status": "Durum",
	"supplier_payments.description": "Açıklama", "supplier_payments.uuid": "UUID",

	"finance.due_state.overdue": "Vadesi geçti", "finance.due_state.due_soon": "Vadesi yaklaşıyor",
	"finance.due_state.later": "Vadesi gelmedi", "finance.due_state.settled": "Kapandı",
	"finance.supplier_invoice_status.open": "Açık", "finance.supplier_invoice_status.partially_paid": "Kısmen ödendi",
	"finance.supplier_invoice_status.paid": "Ödendi", "finance.supplier_invoice_status.void": "İptal",
	"finance.supplier_payment_status.posted": "Kayıtlı", "finance.supplier_payment_status.void": "İptal",
	"finance.supplier_payment_method.bank_transfer": "Havale / EFT", "finance.supplier_payment_method.cash": "Nakit",
	"finance.supplier_payment_method.card": "Kart", "finance.supplier_payment_method.cheque": "Çek",
	"finance.supplier_payment_method.other": "Diğer",
	"finance.reason.invoice_voided":         "Fatura iptal edildi", "finance.reason.due_changed": "Vade değişti",
}

var enSuppliers = map[string]string{
	"export.title.tenant.suppliers":         "Suppliers export",
	"resources.tenant.suppliers":            "Suppliers",
	"export.title.tenant.supplier_invoices": "Supplier invoices export",
	"resources.tenant.supplier_invoices":    "Supplier invoices",
	"export.title.tenant.supplier_payments": "Supplier payments export",
	"resources.tenant.supplier_payments":    "Supplier payments",

	"suppliers.code": "Code", "suppliers.title": "Legal name", "suppliers.supplier_type": "Type",
	"suppliers.tax_number": "Tax ID (VKN / TCKN)", "suppliers.tax_office": "Tax office", "suppliers.phone": "Phone",
	"suppliers.email": "E-mail", "suppliers.website": "Website", "suppliers.address": "Address", "suppliers.city": "City",
	"suppliers.billing_address": "Billing address", "suppliers.billing_email": "Billing e-mail",
	"suppliers.e_invoice": "E-invoice taxpayer", "suppliers.bank_name": "Bank", "suppliers.iban": "IBAN",
	"suppliers.payment_term_days": "Payment terms (days)", "suppliers.status": "Status", "suppliers.note": "Note",
	"suppliers.contacts": "Contacts", "suppliers.payable": "We owe", "suppliers.open_amount": "Open invoices",
	"suppliers.overdue_amount": "Overdue", "suppliers.uuid": "UUID",
	"suppliers.type.service": "Service", "suppliers.type.insurance": "Insurance",
	"suppliers.type.vehicle_supplier": "Vehicle supplier", "suppliers.type.parts": "Parts",
	"suppliers.type.fuel": "Fuel", "suppliers.type.other": "Other",
	"suppliers.status.active": "Active", "suppliers.status.archived": "Archived",

	"supplier_invoices.invoice_no": "Invoice no", "supplier_invoices.supplier_code": "Supplier code",
	"supplier_invoices.supplier": "Supplier", "supplier_invoices.invoice_date": "Invoice date",
	"supplier_invoices.due_date": "Due date", "supplier_invoices.category": "Category",
	"supplier_invoices.branch_code": "Branch code", "supplier_invoices.plate": "Plate",
	"supplier_invoices.net_amount": "Net amount", "supplier_invoices.vat_amount": "VAT",
	"supplier_invoices.total_amount": "Total", "supplier_invoices.paid_amount": "Paid",
	"supplier_invoices.open_amount": "Open", "supplier_invoices.currency": "Currency",
	"supplier_invoices.due_state": "Due state", "supplier_invoices.status": "Status",
	"supplier_invoices.description": "Description", "supplier_invoices.uuid": "UUID",

	"supplier_payments.payment_date": "Payment date", "supplier_payments.supplier_code": "Supplier code",
	"supplier_payments.supplier": "Supplier", "supplier_payments.cash_account_code": "Cash / bank account code",
	"supplier_payments.method": "Method", "supplier_payments.reference": "Reference",
	"supplier_payments.amount": "Amount", "supplier_payments.allocated_amount": "Allocated to invoices",
	"supplier_payments.unallocated_amount": "Advance (unallocated)", "supplier_payments.currency": "Currency",
	"supplier_payments.invoices": "Invoices", "supplier_payments.status": "Status",
	"supplier_payments.description": "Description", "supplier_payments.uuid": "UUID",

	"finance.due_state.overdue": "Overdue", "finance.due_state.due_soon": "Due soon",
	"finance.due_state.later": "Not due yet", "finance.due_state.settled": "Settled",
	"finance.supplier_invoice_status.open": "Open", "finance.supplier_invoice_status.partially_paid": "Partially paid",
	"finance.supplier_invoice_status.paid": "Paid", "finance.supplier_invoice_status.void": "Void",
	"finance.supplier_payment_status.posted": "Posted", "finance.supplier_payment_status.void": "Void",
	"finance.supplier_payment_method.bank_transfer": "Bank transfer", "finance.supplier_payment_method.cash": "Cash",
	"finance.supplier_payment_method.card": "Card", "finance.supplier_payment_method.cheque": "Cheque",
	"finance.supplier_payment_method.other": "Other",
	"finance.reason.invoice_voided":         "Invoice voided", "finance.reason.due_changed": "Due date changed",
}

func init() {
	for k, v := range trSuppliers {
		trCatalog[k] = v
	}
	for k, v := range enSuppliers {
		enCatalog[k] = v
	}
}
