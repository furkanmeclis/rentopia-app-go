// Package catalog lists the events of the notification engine (TEC-733):
// who receives them, the default rule (ordered channels, fallback,
// reminder offsets) and the default message templates (tr / en).
//
// Messages carry names, references, dates, branch names and a link to the
// customer portal (which asks for a one-time code): never identity,
// licence, signature or financial detail.
package catalog

import (
	"sort"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifyengine/channel"
)

// Event codes.
const (
	SiteLeadCreated           = "site_lead.created"
	ReservationConfirmed      = "reservation.confirmed"
	ReservationRejected       = "reservation.rejected"
	ReservationCancelled      = "reservation.cancelled"
	ReservationPaymentPending = "reservation.payment_pending"
	RentalPickupUpcoming      = "rental.pickup_upcoming"
	RentalReturnUpcoming      = "rental.return_upcoming"
	RentalOverdue             = "rental.overdue"
	InvoiceIssued             = "invoice.issued"
	AccountPaymentDue         = "account.payment_due"
	FieldTaskAssigned         = "field_task.assigned"
	FieldTaskOverdue          = "field_task.overdue"
)

// Groups (UI sections).
const (
	GroupReservations = "reservations"
	GroupRentals      = "rentals"
	GroupFinance      = "finance"
	GroupOperations   = "operations"
)

// Template is a default message (subject = e-mail subject / push title).
type Template struct {
	Subject string
	Body    string
}

// Event is one catalog entry.
type Event struct {
	Code     string
	Group    string
	Audience string
	// Instant events fire when the fact happens; Reminder events fire
	// before a deadline at the rule's reminder offsets (minutes).
	Instant  bool
	Reminder bool
	// Default rule.
	Channels []string
	Offsets  []int32
	// Variables the templates may use ({{name}}).
	Vars      []string
	Templates map[string]Template // language -> template
}

// Languages of templates.
var Languages = []string{"tr", "en"}

// MaxOffsetMinutes caps a reminder offset (30 days).
const MaxOffsetMinutes = 30 * 24 * 60

var customerChannels = []string{channel.SMS, channel.Email}
var staffChannels = []string{channel.Push, channel.Email}

var events = []Event{
	{Code: SiteLeadCreated, Group: GroupOperations, Audience: channel.AudienceStaff, Instant: true,
		Channels: []string{channel.Push}, Vars: []string{"kind", "link"}, Templates: map[string]Template{
			"tr": {Subject: "Yeni site başvurusu", Body: "Yeni bir {{kind}} başvurusu alındı."},
			"en": {Subject: "New site enquiry", Body: "A new {{kind}} enquiry was received."},
		}},
	{
		Code: ReservationConfirmed, Group: GroupReservations, Audience: channel.AudienceCustomer, Instant: true,
		Channels: customerChannels,
		Vars:     []string{"customer_name", "organization_name", "reference", "pickup_at", "pickup_branch", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Rezervasyonunuz onaylandı ({{reference}})",
				Body: "Sayın {{customer_name}}, {{reference}} numaralı rezervasyonunuz onaylandı. Teslim: {{pickup_at}}, {{pickup_branch}}. Ayrıntılar: {{link}}"},
			"en": {Subject: "Your reservation is confirmed ({{reference}})",
				Body: "Dear {{customer_name}}, your reservation {{reference}} is confirmed. Pickup: {{pickup_at}}, {{pickup_branch}}. Details: {{link}}"},
		},
	},
	{
		Code: ReservationRejected, Group: GroupReservations, Audience: channel.AudienceCustomer, Instant: true,
		Channels: customerChannels,
		Vars:     []string{"customer_name", "organization_name", "reference", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Rezervasyon talebiniz hakkında ({{reference}})",
				Body: "Sayın {{customer_name}}, {{reference}} numaralı rezervasyon talebiniz maalesef onaylanamadı. Bilgi için: {{link}}"},
			"en": {Subject: "About your reservation request ({{reference}})",
				Body: "Dear {{customer_name}}, unfortunately your reservation request {{reference}} could not be confirmed. More: {{link}}"},
		},
	},
	{
		Code: ReservationCancelled, Group: GroupReservations, Audience: channel.AudienceCustomer, Instant: true,
		Channels: customerChannels,
		Vars:     []string{"customer_name", "organization_name", "reference", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Rezervasyonunuz iptal edildi ({{reference}})",
				Body: "Sayın {{customer_name}}, {{reference}} numaralı rezervasyonunuz iptal edildi. Bilgi için: {{link}}"},
			"en": {Subject: "Your reservation was cancelled ({{reference}})",
				Body: "Dear {{customer_name}}, your reservation {{reference}} was cancelled. More: {{link}}"},
		},
	},
	{
		Code: ReservationPaymentPending, Group: GroupReservations, Audience: channel.AudienceCustomer, Instant: true, Reminder: true,
		Channels: customerChannels, Offsets: []int32{120},
		Vars: []string{"customer_name", "organization_name", "reference", "payment_deadline", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Ödemeniz bekleniyor ({{reference}})",
				Body: "Sayın {{customer_name}}, {{reference}} numaralı rezervasyonunuz için ödeme bekleniyor. Son ödeme: {{payment_deadline}}. Ödeme için: {{link}}"},
			"en": {Subject: "Payment pending ({{reference}})",
				Body: "Dear {{customer_name}}, the payment for your reservation {{reference}} is pending. Deadline: {{payment_deadline}}. Pay here: {{link}}"},
		},
	},
	{
		Code: RentalPickupUpcoming, Group: GroupRentals, Audience: channel.AudienceCustomer, Reminder: true,
		Channels: customerChannels, Offsets: []int32{1440, 120},
		Vars: []string{"customer_name", "organization_name", "reference", "pickup_at", "pickup_branch", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Araç teslimi yaklaşıyor ({{reference}})",
				Body: "Sayın {{customer_name}}, aracınızın teslimi {{pickup_at}} tarihinde {{pickup_branch}} şubesinde. Ayrıntılar: {{link}}"},
			"en": {Subject: "Your pickup is coming up ({{reference}})",
				Body: "Dear {{customer_name}}, your vehicle pickup is on {{pickup_at}} at {{pickup_branch}}. Details: {{link}}"},
		},
	},
	{
		Code: RentalReturnUpcoming, Group: GroupRentals, Audience: channel.AudienceCustomer, Reminder: true,
		Channels: customerChannels, Offsets: []int32{1440, 120},
		Vars: []string{"customer_name", "organization_name", "reference", "return_at", "return_branch", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Araç iadesi yaklaşıyor ({{reference}})",
				Body: "Sayın {{customer_name}}, aracınızın iadesi {{return_at}} tarihinde {{return_branch}} şubesinde. Uzatma için: {{link}}"},
			"en": {Subject: "Your return is coming up ({{reference}})",
				Body: "Dear {{customer_name}}, your vehicle is due back on {{return_at}} at {{return_branch}}. To extend: {{link}}"},
		},
	},
	{
		Code: RentalOverdue, Group: GroupRentals, Audience: channel.AudienceCustomer, Instant: true,
		Channels: customerChannels,
		Vars:     []string{"customer_name", "organization_name", "reference", "return_branch", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Araç iadesi gecikti ({{reference}})",
				Body: "Sayın {{customer_name}}, {{reference}} numaralı kiralamanın iade zamanı geçti. Lütfen {{return_branch}} şubesiyle iletişime geçin: {{link}}"},
			"en": {Subject: "Your return is overdue ({{reference}})",
				Body: "Dear {{customer_name}}, the return time of rental {{reference}} has passed. Please contact {{return_branch}}: {{link}}"},
		},
	},
	{
		Code: InvoiceIssued, Group: GroupFinance, Audience: channel.AudienceCustomer, Instant: true,
		Channels: []string{channel.Email, channel.SMS},
		Vars:     []string{"customer_name", "organization_name", "invoice_number", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Faturanız düzenlendi ({{invoice_number}})",
				Body: "Sayın {{customer_name}}, {{invoice_number}} numaralı faturanız düzenlendi. Görüntülemek için: {{link}}"},
			"en": {Subject: "Your invoice is issued ({{invoice_number}})",
				Body: "Dear {{customer_name}}, your invoice {{invoice_number}} has been issued. View it here: {{link}}"},
		},
	},
	{
		Code: AccountPaymentDue, Group: GroupFinance, Audience: channel.AudienceCustomer, Reminder: true,
		Channels: []string{channel.Email, channel.SMS}, Offsets: []int32{4320, 0},
		Vars: []string{"customer_name", "organization_name", "due_date", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Ödeme vadesi hatırlatması",
				Body: "Sayın {{customer_name}}, cari hesabınızda {{due_date}} vadeli ödemeniz bulunuyor. Ayrıntılar için {{organization_name}} ile iletişime geçebilirsiniz."},
			"en": {Subject: "Payment due reminder",
				Body: "Dear {{customer_name}}, a payment on your account falls due on {{due_date}}. Contact {{organization_name}} for details."},
		},
	},
	{
		Code: FieldTaskAssigned, Group: GroupOperations, Audience: channel.AudienceStaff, Instant: true,
		Channels: staffChannels,
		Vars:     []string{"staff_name", "organization_name", "task_type", "due_at", "branch", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Yeni saha işi: {{task_type}}",
				Body: "{{staff_name}}, size yeni bir saha işi atandı: {{task_type}}, {{due_at}}, {{branch}}. {{link}}"},
			"en": {Subject: "New field task: {{task_type}}",
				Body: "{{staff_name}}, a new field task was assigned to you: {{task_type}}, {{due_at}}, {{branch}}. {{link}}"},
		},
	},
	{
		Code: FieldTaskOverdue, Group: GroupOperations, Audience: channel.AudienceStaff, Instant: true,
		Channels: staffChannels,
		Vars:     []string{"staff_name", "organization_name", "task_type", "due_at", "branch", "link"},
		Templates: map[string]Template{
			"tr": {Subject: "Geciken saha işi: {{task_type}}",
				Body: "{{staff_name}}, {{due_at}} zamanlı {{task_type}} işi gecikti ({{branch}}). {{link}}"},
			"en": {Subject: "Overdue field task: {{task_type}}",
				Body: "{{staff_name}}, the {{task_type}} task due {{due_at}} is overdue ({{branch}}). {{link}}"},
		},
	},
}

var byCode = func() map[string]Event {
	m := make(map[string]Event, len(events))
	for _, e := range events {
		m[e.Code] = e
	}
	return m
}()

// All returns the catalog in display order.
func All() []Event {
	out := make([]Event, len(events))
	copy(out, events)
	return out
}

// Get returns one event.
func Get(code string) (Event, bool) {
	e, ok := byCode[code]
	return e, ok
}

// Codes returns every event code, sorted.
func Codes() []string {
	out := make([]string, 0, len(events))
	for _, e := range events {
		out = append(out, e.Code)
	}
	sort.Strings(out)
	return out
}

// SampleVars are preview values for every variable.
func SampleVars(lang string) map[string]string {
	if lang == "en" {
		return map[string]string{
			"customer_name": "Jane Doe", "staff_name": "Alex Smith", "organization_name": "Rentopia Demo",
			"reference": "R-2026-000123", "pickup_at": "18.10.2026 10:00", "pickup_branch": "Airport",
			"return_at": "21.10.2026 10:00", "return_branch": "Airport", "payment_deadline": "17.10.2026 18:00",
			"invoice_number": "RNT2026000000042", "due_date": "25.10.2026", "task_type": "Handover",
			"due_at": "18.10.2026 09:30", "branch": "Airport", "link": "https://app.example.com/r/demo",
		}
	}
	return map[string]string{
		"customer_name": "Ayşe Yılmaz", "staff_name": "Mehmet Kaya", "organization_name": "Rentopia Demo",
		"reference": "R-2026-000123", "pickup_at": "18.10.2026 10:00", "pickup_branch": "Havalimanı",
		"return_at": "21.10.2026 10:00", "return_branch": "Havalimanı", "payment_deadline": "17.10.2026 18:00",
		"invoice_number": "RNT2026000000042", "due_date": "25.10.2026", "task_type": "Teslim",
		"due_at": "18.10.2026 09:30", "branch": "Havalimanı", "link": "https://app.example.com/r/demo",
	}
}

// TaskTypeLabel names a field task type in a message.
func TaskTypeLabel(taskType, lang string) string {
	tr := map[string]string{
		"preparation": "Hazırlık", "handover": "Teslim", "return": "İade", "cleaning": "Temizlik",
		"maintenance": "Bakım", "deposit": "Depozito", "transfer": "Transfer", "other": "Diğer",
	}
	en := map[string]string{
		"preparation": "Preparation", "handover": "Handover", "return": "Return", "cleaning": "Cleaning",
		"maintenance": "Maintenance", "deposit": "Deposit", "transfer": "Transfer", "other": "Other",
	}
	m := tr
	if lang == "en" {
		m = en
	}
	if v, ok := m[taskType]; ok {
		return v
	}
	return taskType
}
