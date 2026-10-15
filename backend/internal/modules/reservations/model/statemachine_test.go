package model

import "testing"

// Every (from, action, to) triple: allowed exactly when listed.
func TestStateMachineTable(t *testing.T) {
	allowed := map[[3]string]bool{
		{StatusRequest, ActionSubmit, StatusAwaitingReview}:          true,
		{StatusRequest, ActionSubmit, StatusAwaitingPayment}:         true,
		{StatusRequest, ActionSubmit, StatusConfirmed}:               true,
		{StatusRequest, ActionCancel, StatusCancelled}:               true,
		{StatusRequest, ActionApprove, StatusAwaitingPayment}:        true,
		{StatusRequest, ActionApprove, StatusConfirmed}:              true,
		{StatusRequest, ActionReject, StatusCancelled}:               true,
		{StatusAwaitingReview, ActionReject, StatusCancelled}:        true,
		{StatusExpired, ActionReinstate, StatusConfirmed}:            true,
		{StatusAwaitingReview, ActionApprove, StatusAwaitingPayment}: true,
		{StatusAwaitingReview, ActionApprove, StatusConfirmed}:       true,
		{StatusAwaitingReview, ActionConfirm, StatusConfirmed}:       true,
		{StatusAwaitingReview, ActionCancel, StatusCancelled}:        true,
		{StatusAwaitingReview, ActionExpire, StatusExpired}:          true,
		{StatusAwaitingPayment, ActionConfirm, StatusConfirmed}:      true,
		{StatusAwaitingPayment, ActionCancel, StatusCancelled}:       true,
		{StatusAwaitingPayment, ActionExpire, StatusExpired}:         true,
		{StatusConfirmed, ActionCancel, StatusCancelled}:             true,
		{StatusConfirmed, ActionNoShow, StatusNoShow}:                true,
	}
	actions := []string{ActionSubmit, ActionApprove, ActionConfirm, ActionCancel, ActionExpire, ActionNoShow, ActionReject, ActionReinstate}
	for _, from := range Statuses {
		for _, action := range actions {
			for _, to := range Statuses {
				want := allowed[[3]string{from, action, to}]
				if got := CanTransition(from, action, to); got != want {
					t.Errorf("%s -%s-> %s = %v, want %v", from, action, to, got, want)
				}
			}
		}
	}
	for _, s := range []string{StatusCancelled, StatusExpired, StatusNoShow} {
		if !IsTerminal(s) || HoldsCapacity(s) || CommitmentOf(s) != CommitmentNone {
			t.Errorf("%s must be terminal without capacity", s)
		}
		for _, a := range actions {
			if a == ActionReinstate && s == StatusExpired {
				continue // system: late payment (TEC-708)
			}
			if CanAct(s, a) {
				t.Errorf("terminal %s allows %s", s, a)
			}
		}
	}
	if CommitmentOf(StatusAwaitingReview) != CommitmentHold || CommitmentOf(StatusConfirmed) != CommitmentFirm || CommitmentOf(StatusRequest) != CommitmentNone {
		t.Error("commitments of the waiting / confirmed states")
	}
}
