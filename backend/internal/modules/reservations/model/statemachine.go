package model

// The reservation state machine (docs/design/reservations.md §3).
//
//	request ──submit──▶ awaiting_review | awaiting_payment | confirmed
//	request | awaiting_review ──approve──▶ awaiting_payment | confirmed   (staff accepts)
//	request | awaiting_review ──reject──▶ cancelled                       (staff rejects, reason)
//	expired ──reinstate──▶ confirmed   (system: a late payment found capacity again)
//	awaiting_review | awaiting_payment ──confirm──▶ confirmed
//	request | awaiting_review | awaiting_payment | confirmed ──cancel──▶ cancelled
//	awaiting_review | awaiting_payment ──expire──▶ expired   (system: window over)
//	confirmed ──no_show──▶ no_show                            (after the pick-up time)
//
// cancelled, expired and no_show are terminal for people; the only way out
// of expired is the system reinstate of a late payment (TEC-708,
// docs/design/payments.md §5.3). Capacity follows the state:
// request holds nothing, awaiting_* hold an option (allocation kind hold,
// expires with the window), confirmed holds a firm commitment; leaving to a
// terminal state releases it exactly once.

// Reservation statuses.
const (
	StatusRequest         = "request"
	StatusAwaitingReview  = "awaiting_review"
	StatusAwaitingPayment = "awaiting_payment"
	StatusConfirmed       = "confirmed"
	StatusCancelled       = "cancelled"
	StatusExpired         = "expired"
	StatusNoShow          = "no_show"
)

// Statuses lists the statuses in display order.
var Statuses = []string{
	StatusRequest, StatusAwaitingReview, StatusAwaitingPayment, StatusConfirmed,
	StatusCancelled, StatusExpired, StatusNoShow,
}

// Payment statuses (payments arrive in M3).
const (
	PaymentUnpaid    = "unpaid"
	PaymentPartially = "partially_paid"
	PaymentPaid      = "paid"
	PaymentRefunded  = "refunded"
)

// PaymentStatuses lists the payment statuses in display order.
var PaymentStatuses = []string{PaymentUnpaid, PaymentPartially, PaymentPaid, PaymentRefunded}

// Capacity commitments.
const (
	CommitmentNone = "none"
	CommitmentHold = "hold"
	CommitmentFirm = "firm"
)

// Transition actions.
const (
	ActionSubmit  = "submit"
	ActionApprove = "approve"
	ActionConfirm = "confirm"
	ActionCancel  = "cancel"
	ActionExpire  = "expire"
	ActionNoShow  = "no_show"
	// ActionReject is a staff refusal of a request / review (→ cancelled).
	ActionReject = "reject"
	// ActionReinstate confirms an expired reservation whose late payment
	// found capacity again (system only, recorded with the payment).
	ActionReinstate = "reinstate"
)

// transitions: from → action → allowed targets.
var transitions = map[string]map[string][]string{
	StatusRequest: {
		ActionSubmit:  {StatusAwaitingReview, StatusAwaitingPayment, StatusConfirmed},
		ActionApprove: {StatusAwaitingPayment, StatusConfirmed},
		ActionReject:  {StatusCancelled},
		ActionCancel:  {StatusCancelled},
	},
	StatusAwaitingReview: {
		ActionApprove: {StatusAwaitingPayment, StatusConfirmed},
		ActionReject:  {StatusCancelled},
		ActionConfirm: {StatusConfirmed},
		ActionCancel:  {StatusCancelled},
		ActionExpire:  {StatusExpired},
	},
	StatusAwaitingPayment: {
		ActionConfirm: {StatusConfirmed},
		ActionCancel:  {StatusCancelled},
		ActionExpire:  {StatusExpired},
	},
	StatusConfirmed: {
		ActionCancel: {StatusCancelled},
		ActionNoShow: {StatusNoShow},
	},
	StatusExpired: {
		ActionReinstate: {StatusConfirmed},
	},
}

// CanTransition reports whether action moves a reservation from → to.
func CanTransition(from, action, to string) bool {
	for _, t := range transitions[from][action] {
		if t == to {
			return true
		}
	}
	return false
}

// CanAct reports whether action is possible from the status at all.
func CanAct(from, action string) bool {
	return len(transitions[from][action]) > 0
}

// IsTerminal reports whether the status is final.
func IsTerminal(status string) bool {
	return status == StatusCancelled || status == StatusExpired || status == StatusNoShow
}

// HoldsCapacity reports whether a reservation in the status has an active
// allocation (option or firm).
func HoldsCapacity(status string) bool {
	return status == StatusAwaitingReview || status == StatusAwaitingPayment || status == StatusConfirmed
}

// CommitmentOf is the capacity commitment of a status.
func CommitmentOf(status string) string {
	switch status {
	case StatusAwaitingReview, StatusAwaitingPayment:
		return CommitmentHold
	case StatusConfirmed:
		return CommitmentFirm
	}
	return CommitmentNone
}
