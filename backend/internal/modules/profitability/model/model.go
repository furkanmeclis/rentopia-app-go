// Package model holds the types of the vehicle profitability report
// (TEC-727, docs/design/vehicle-profitability.md).
package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// Resource is the io engine slug of the fleet report export.
const Resource = "tenant.vehicle_profitability"

// Bases: accrual = ledger accruals net of reversals, dated by entry date;
// cash = collections net of voids and refunds, expenses and supplier
// payments dated in the period.
const (
	BasisAccrual = "accrual"
	BasisCash    = "cash"
)

// Bases lists the valid bases.
var Bases = []string{BasisAccrual, BasisCash}

// Allocation methods of the shared overhead (expenses of a branch or of
// the head office that name no vehicle).
const (
	AllocationRentalDays   = "rental_days"
	AllocationVehicleCount = "vehicle_count"
	AllocationRevenue      = "revenue"
)

// Allocations lists the valid allocation methods.
var Allocations = []string{AllocationRentalDays, AllocationVehicleCount, AllocationRevenue}

// Overhead scopes: which shared overhead the caller's report carries.
const (
	// OverheadAll: head office and every branch.
	OverheadAll = "all"
	// OverheadBranches: the caller's branches only (a member limited to
	// some branches never sees head-office expenses).
	OverheadBranches = "branches"
)

// Filter selects the period, the method and the rows of a report.
type Filter struct {
	From       *time.Time
	To         *time.Time
	Basis      string
	Allocation string
	// Rows: home branch, rental group, one vehicle, plate / model search.
	BranchUUIDs      []uuid.UUID
	RentalGroupUUIDs []uuid.UUID
	VehicleUUID      *uuid.UUID
	Q                string
}

// BranchRef names a branch.
type BranchRef struct {
	UUID uuid.UUID `json:"uuid"`
	Code string    `json:"code"`
	Name string    `json:"name"`
}

// VehicleRef names a vehicle.
type VehicleRef struct {
	UUID       uuid.UUID `json:"uuid"`
	Plate      string    `json:"plate"`
	Label      string    `json:"label"`
	HomeBranch BranchRef `json:"home_branch"`
}

// Figures are the money and day figures of one vehicle (or a total, or one
// month). Money is TRY, VAT inclusive, as the ledger and the expenses
// record it.
type Figures struct {
	// Income.
	Rent        money.Money `json:"rent"`
	Extras      money.Money `json:"extras"`
	Charges     money.Money `json:"charges"`
	TollFees    money.Money `json:"toll_fees"`
	GrossIncome money.Money `json:"gross_income"`
	// Deductions.
	PaymentCommission money.Money `json:"payment_commission"`
	ChannelCommission money.Money `json:"channel_commission"`
	Commissions       money.Money `json:"commissions"`
	// Direct costs.
	Expenses    money.Money `json:"expenses"`
	Maintenance money.Money `json:"maintenance"`
	Transfers   money.Money `json:"transfers"`
	DirectCosts money.Money `json:"direct_costs"`
	// Shared overhead allocated to the vehicle.
	Overhead money.Money `json:"overhead"`
	// Gross income − commissions − direct costs − overhead.
	Contribution money.Money `json:"contribution"`
	// Contribution / gross income in percent (one decimal); nil when there
	// is no income.
	Margin *float64 `json:"margin"`
	// Kept apart: never income, never cost.
	Refunded         money.Money `json:"refunded"`
	DepositsHeld     money.Money `json:"deposits_held"`
	DepositsReturned money.Money `json:"deposits_returned"`
	PassThrough      money.Money `json:"pass_through"`
	Capex            money.Money `json:"capex"`
	// Days (Europe/Istanbul, measured in time, two decimals).
	RentalDays    float64 `json:"rental_days"`
	AvailableDays float64 `json:"available_days"`
	// Rented / available in percent (one decimal); nil without available
	// time.
	Occupancy   *float64 `json:"occupancy"`
	RentalCount int      `json:"rental_count"`
}

// Row is one vehicle of the fleet report.
type Row struct {
	Vehicle VehicleRef `json:"vehicle"`
	Figures
}

// TrendPoint is one calendar month (Europe/Istanbul) of the period.
type TrendPoint struct {
	Month string `json:"month"`
	From  string `json:"from"`
	To    string `json:"to"`
	Figures
}

// Method states how the report was computed (shown on screen and in
// exports).
type Method struct {
	Basis      string `json:"basis"`
	Allocation string `json:"allocation"`
	Timezone   string `json:"timezone"`
	Currency   string `json:"currency"`
	// VATInclusive: amounts are VAT inclusive (ledger and expense totals).
	VATInclusive bool `json:"vat_inclusive"`
	// OverheadScope: all (head office + every branch) or branches (the
	// caller's branches only).
	OverheadScope string `json:"overhead_scope"`
	// AllocationFallback: in at least one month the chosen basis had no
	// weight (no rental days / no revenue) in a pool, and that pool was
	// shared by vehicle count.
	AllocationFallback bool `json:"allocation_fallback"`
	// UnallocatedOverhead: overhead of a pool without a vehicle in service
	// (not part of any row).
	UnallocatedOverhead money.Money `json:"unallocated_overhead"`
}

// Report is the fleet report.
type Report struct {
	From   string       `json:"from"`
	To     string       `json:"to"`
	Method Method       `json:"method"`
	Rows   []Row        `json:"rows"`
	Totals Figures      `json:"totals"`
	Trend  []TrendPoint `json:"trend"`
}

// CategoryAmount is the direct expense of one expense category.
type CategoryAmount struct {
	Code   string      `json:"code"`
	Name   string      `json:"name"`
	Amount money.Money `json:"amount"`
}

// VehicleMonths is one vehicle's figures per calendar month of the period
// (the reports of TEC-738 aggregate them by branch and rental group).
type VehicleMonths struct {
	// VehicleID and HomeBranchID are internal ids (never serialized).
	VehicleID    int64
	HomeBranchID int64
	Vehicle      VehicleRef
	// Months follow Breakdown.Months; RentalCount is on Totals only.
	Months []Figures
	Totals Figures
}

// Breakdown is the per-vehicle, per-month result of the engine for the
// rows a report filter selects.
type Breakdown struct {
	From     string
	To       string
	Method   Method
	Months   []TrendPoint
	Vehicles []VehicleMonths
}

// VehicleReport is the profitability of one vehicle (vehicle detail tab).
type VehicleReport struct {
	From       string           `json:"from"`
	To         string           `json:"to"`
	Method     Method           `json:"method"`
	Vehicle    VehicleRef       `json:"vehicle"`
	Figures    Figures          `json:"figures"`
	Trend      []TrendPoint     `json:"trend"`
	Categories []CategoryAmount `json:"categories"`
}
