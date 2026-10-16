// Package model holds the types of the reports (TEC-738, §15.1):
// occupancy, income / expense, channel
// performance, receivable / payable aging and the branch comparison.
package model

import (
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// io engine slugs of the report exports.
const (
	ResourceOccupancy     = "tenant.report_occupancy"
	ResourceIncomeExpense = "tenant.report_income_expense"
	ResourceChannels      = "tenant.report_channels"
	ResourceAging         = "tenant.report_aging"
	ResourceBranches      = "tenant.report_branches"
)

// Occupancy groupings.
const (
	GroupRentalGroup = "rental_group"
	GroupBranch      = "branch"
	GroupMonth       = "month"
	GroupCategory    = "category"
)

// OccupancyGroups lists the occupancy groupings.
var OccupancyGroups = []string{GroupRentalGroup, GroupBranch, GroupMonth}

// IncomeExpenseGroups lists the income / expense groupings.
var IncomeExpenseGroups = []string{GroupCategory, GroupBranch, GroupMonth}

// Channel report date bases: booking (reservation created) or pick-up.
const (
	DateBooking = "booking"
	DatePickup  = "pickup"
)

// DateBases lists the channel date bases.
var DateBases = []string{DateBooking, DatePickup}

// Aging parties.
const (
	PartyCustomer = "customer"
	PartySupplier = "supplier"
)

// Parties lists the aging parties.
var Parties = []string{PartyCustomer, PartySupplier}

// Profitability bases and allocations accepted by the branch comparison
// (the vehicle profitability engine's).
var (
	Bases       = []string{"accrual", "cash"}
	Allocations = []string{"rental_days", "vehicle_count", "revenue"}
)

// Filter is the common query of the reports.
type Filter struct {
	From        *time.Time
	To          *time.Time
	BranchUUIDs []uuid.UUID
	// Occupancy.
	GroupBy          string
	RentalGroupUUIDs []uuid.UUID
	// Channels.
	DateBasis string
	// Aging.
	AsOf  *time.Time
	Party string
	// Branch comparison (profitability method).
	Basis      string
	Allocation string
}

// Ref names a branch, a rental group or another grouping key.
type Ref struct {
	UUID *uuid.UUID `json:"uuid"`
	Code string     `json:"code"`
	Name string     `json:"name"`
}

// Method states how a report was computed (shown on screen and in
// exports).
type Method struct {
	Timezone     string `json:"timezone"`
	Currency     string `json:"currency"`
	VATInclusive bool   `json:"vat_inclusive"`
	// BranchScope: all (every branch and the head office) or branches
	// (the caller's branches only; head-office records left out).
	BranchScope string `json:"branch_scope"`
}

// Branch scopes of Method.
const (
	ScopeAll      = "all"
	ScopeBranches = "branches"
)

// --- Occupancy -----------------------------------------------------------

// OccupancyFigures are the time figures of a group (or a month).
type OccupancyFigures struct {
	Vehicles      int     `json:"vehicles"`
	RentalDays    float64 `json:"rental_days"`
	AvailableDays float64 `json:"available_days"`
	// Rented / available in percent (one decimal); nil without available
	// time.
	Occupancy   *float64    `json:"occupancy"`
	RentalCount int         `json:"rental_count"`
	GrossIncome money.Money `json:"gross_income"`
	// Gross income per available day (RevPAD); nil without available time.
	IncomePerAvailableDay *money.Money `json:"income_per_available_day"`
}

// OccupancyPoint is one month of a group.
type OccupancyPoint struct {
	Month         string   `json:"month"`
	RentalDays    float64  `json:"rental_days"`
	AvailableDays float64  `json:"available_days"`
	Occupancy     *float64 `json:"occupancy"`
}

// OccupancyRow is one group (rental group, branch or month).
type OccupancyRow struct {
	Key   string `json:"key"`
	Group Ref    `json:"group"`
	OccupancyFigures
	Trend []OccupancyPoint `json:"trend"`
}

// OccupancyReport is the occupancy report.
type OccupancyReport struct {
	From    string           `json:"from"`
	To      string           `json:"to"`
	GroupBy string           `json:"group_by"`
	Method  Method           `json:"method"`
	Rows    []OccupancyRow   `json:"rows"`
	Totals  OccupancyFigures `json:"totals"`
	Trend   []OccupancyPoint `json:"trend"`
	// MultiGroupVehicles: vehicles counted in more than one rental group.
	MultiGroupVehicles int `json:"multi_group_vehicles"`
}

// --- Income / expense ----------------------------------------------------

// Income / expense row kinds.
const (
	KindIncome      = "income"
	KindExpense     = "expense"
	KindPassThrough = "pass_through"
	KindCapex       = "capex"
	KindGroup       = "group"
)

// IncomeExpenseFigures are the totals of a row, a month or the report.
type IncomeExpenseFigures struct {
	Income  money.Money `json:"income"`
	Expense money.Money `json:"expense"`
	// Income − expense.
	Net money.Money `json:"net"`
	// Kept apart: tolls passed through to the customer (never income) and
	// vehicle procurement (an investment, never an expense).
	PassThrough money.Money `json:"pass_through"`
	Capex       money.Money `json:"capex"`
	// Net / income in percent (one decimal); nil without income.
	Margin *float64 `json:"margin"`
}

// IncomeExpenseRow is one category (income or expense), branch or month.
type IncomeExpenseRow struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
	// Category code: income rent / charge.<kind> / other, or the expense
	// category code; branch code or month otherwise.
	Code string `json:"code"`
	// Display name (expense category / branch name); empty for income
	// categories (the client translates the code).
	Name string `json:"name"`
	IncomeExpenseFigures
	// Share of the row in its kind's total, percent (one decimal).
	Share *float64 `json:"share"`
}

// IncomeExpensePoint is one month.
type IncomeExpensePoint struct {
	Month string `json:"month"`
	IncomeExpenseFigures
}

// IncomeExpenseReport is the income / expense report.
type IncomeExpenseReport struct {
	From    string               `json:"from"`
	To      string               `json:"to"`
	GroupBy string               `json:"group_by"`
	Method  Method               `json:"method"`
	Rows    []IncomeExpenseRow   `json:"rows"`
	Totals  IncomeExpenseFigures `json:"totals"`
	Trend   []IncomeExpensePoint `json:"trend"`
}

// --- Channels ------------------------------------------------------------

// ChannelFigures are the figures of one channel (or the total).
type ChannelFigures struct {
	Reservations int `json:"reservations"`
	Confirmed    int `json:"confirmed"`
	Pending      int `json:"pending"`
	Cancelled    int `json:"cancelled"`
	Expired      int `json:"expired"`
	NoShow       int `json:"no_show"`
	// Cancelled / reservations in percent (one decimal); nil without
	// reservations.
	CancellationRate *float64 `json:"cancellation_rate"`
	// Revenue: total of confirmed reservations (VAT inclusive sold price).
	Revenue    money.Money `json:"revenue"`
	Commission money.Money `json:"commission"`
	// Revenue − commission.
	NetRevenue money.Money `json:"net_revenue"`
	// Commission / revenue in percent; nil without revenue.
	CommissionRate *float64 `json:"commission_rate"`
	// Revenue collected by the channel itself (collecting_party=channel).
	ChannelCollected money.Money `json:"channel_collected"`
	// Revenue / confirmed reservations; nil without confirmed ones.
	AverageRevenue *money.Money `json:"average_revenue"`
	// Average length of confirmed reservations in days (two decimals).
	AverageDays float64 `json:"average_days"`
	// Share of the channel in the total revenue, percent.
	RevenueShare *float64 `json:"revenue_share"`
}

// ChannelRow is one sales channel.
type ChannelRow struct {
	Channel string `json:"channel"`
	ChannelFigures
}

// ChannelPoint is one channel in one month.
type ChannelPoint struct {
	Month        string      `json:"month"`
	Channel      string      `json:"channel"`
	Reservations int         `json:"reservations"`
	Cancelled    int         `json:"cancelled"`
	Revenue      money.Money `json:"revenue"`
}

// ChannelReport is the channel performance report.
type ChannelReport struct {
	From      string `json:"from"`
	To        string `json:"to"`
	DateBasis string `json:"date_basis"`
	Method    Method `json:"method"`
	// Source: the reservation field the channels come from.
	Source string         `json:"source"`
	Rows   []ChannelRow   `json:"rows"`
	Totals ChannelFigures `json:"totals"`
	Trend  []ChannelPoint `json:"trend"`
}

// --- Aging ---------------------------------------------------------------

// AgingFigures are the open amounts of an account (or the total) by days
// past due as of the report date.
type AgingFigures struct {
	Current money.Money `json:"current"`
	Days30  money.Money `json:"days_1_30"`
	Days60  money.Money `json:"days_31_60"`
	Days90  money.Money `json:"days_61_90"`
	Over90  money.Money `json:"days_90_plus"`
	// Overdue = every bucket past due; Open = overdue + current.
	Overdue money.Money `json:"overdue"`
	Open    money.Money `json:"open"`
	// Settlements not applied to any charge (the party's credit).
	Credit money.Money `json:"credit"`
}

// AgingRow is one account.
type AgingRow struct {
	UUID            uuid.UUID   `json:"uuid"`
	Code            string      `json:"code"`
	Name            string      `json:"name"`
	PartyKind       string      `json:"party_kind"`
	PaymentTermDays int32       `json:"payment_term_days"`
	Balance         money.Money `json:"balance"`
	AgingFigures
	LastEntry *string `json:"last_entry"`
}

// AgingBucket is one bucket of the totals (chart).
type AgingBucket struct {
	Key    string      `json:"key"`
	Amount money.Money `json:"amount"`
}

// AgingReport is the receivable (customer) or payable (supplier) aging.
type AgingReport struct {
	AsOf    string        `json:"as_of"`
	Party   string        `json:"party"`
	Method  Method        `json:"method"`
	Rows    []AgingRow    `json:"rows"`
	Totals  AgingFigures  `json:"totals"`
	Buckets []AgingBucket `json:"buckets"`
}

// --- Branch comparison ---------------------------------------------------

// BranchFigures are the figures of one branch (vehicles' home branch).
type BranchFigures struct {
	Vehicles          int          `json:"vehicles"`
	RentalCount       int          `json:"rental_count"`
	RentalDays        float64      `json:"rental_days"`
	AvailableDays     float64      `json:"available_days"`
	Occupancy         *float64     `json:"occupancy"`
	GrossIncome       money.Money  `json:"gross_income"`
	Commissions       money.Money  `json:"commissions"`
	DirectCosts       money.Money  `json:"direct_costs"`
	Overhead          money.Money  `json:"overhead"`
	Contribution      money.Money  `json:"contribution"`
	Margin            *float64     `json:"margin"`
	IncomePerVehicle  *money.Money `json:"income_per_vehicle"`
	ContributionShare *float64     `json:"contribution_share"`
}

// BranchRow is one branch.
type BranchRow struct {
	Branch Ref `json:"branch"`
	BranchFigures
}

// BranchMethod adds the profitability method to the common one.
type BranchMethod struct {
	Method
	Basis               string      `json:"basis"`
	Allocation          string      `json:"allocation"`
	OverheadScope       string      `json:"overhead_scope"`
	AllocationFallback  bool        `json:"allocation_fallback"`
	UnallocatedOverhead money.Money `json:"unallocated_overhead"`
}

// BranchReport is the branch comparison.
type BranchReport struct {
	From   string        `json:"from"`
	To     string        `json:"to"`
	Method BranchMethod  `json:"method"`
	Rows   []BranchRow   `json:"rows"`
	Totals BranchFigures `json:"totals"`
}
