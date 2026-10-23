package usecase

import (
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/payments"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// The refund guard's decision table (TEC-962). Only a mismatch refuses;
// an unreadable provider (unverified) lets the refund go ahead with an
// item for staff.
func TestDecideRefund(t *testing.T) {
	t.Parallel()
	st := func(v string) *payments.RefundState { return &payments.RefundState{Refunded: money.MustParse(v)} }
	cases := []struct {
		name  string
		ours  string
		state *payments.RefundState
		err   error
		want  RefundGuard
	}{
		{"adapter cannot tell", "0", nil, integrations.ErrUnsupported, GuardSkipped},
		{"wrapped unsupported", "0", nil, errors.Join(errors.New("x"), integrations.ErrUnsupported), GuardSkipped},
		{"provider unreachable: refund goes ahead, item kept", "0", nil, &payments.ProviderError{Code: "network", Temporary: true}, GuardUnverified},
		{"unreadable answer: refund goes ahead, item kept", "0", nil, payments.ErrMalformed, GuardUnverified},
		{"reporting refused (non-temporary): refund goes ahead", "0", nil, &payments.ProviderError{Code: "1001"}, GuardUnverified},
		{"nothing refunded anywhere", "0", st("0"), nil, GuardProceed},
		{"in line", "150", st("150"), nil, GuardProceed},
		{"provider shows a refund we lack (restore)", "0", st("200"), nil, GuardMismatch},
		{"provider shows more than we have", "100", st("150.50"), nil, GuardMismatch},
		{"we show more than the provider (lagging report)", "200", st("100"), nil, GuardProceed},
		{"no state, no error", "0", nil, nil, GuardSkipped},
	}
	for _, c := range cases {
		if got := DecideRefund(money.MustParse(c.ours), c.state, c.err); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// Orphan matching accepts only a paid checkout that is the notified one,
// carries one of our order references and the reservation's currency.
func TestCheckOrphanMatch(t *testing.T) {
	t.Parallel()
	order := uuid.New()
	facts := OrphanFacts{ConversationID: order.String(), ProviderPaymentID: "2001"}
	ok := payments.Result{
		Status: payments.StatusSucceeded, PaymentID: "2001", ConversationID: order.String(), BasketID: order.String(),
		Currency: "TRY", Price: money.MustParse("500"), PaidPrice: money.MustParse("500"),
	}
	id, amount, err := CheckOrphanMatch(facts, ok, "try")
	if err != nil || id != order || !amount.Equal(money.MustParse("500")) {
		t.Fatalf("valid match: %v %s %v", id, amount, err)
	}
	mut := func(f func(r *payments.Result)) payments.Result { r := ok; f(&r); return r }
	cases := map[string]struct {
		res      payments.Result
		currency string
		code     string
	}{
		"not paid":           {mut(func(r *payments.Result) { r.Status = payments.StatusPending }), "TRY", "orphan_not_paid"},
		"failed":             {mut(func(r *payments.Result) { r.Status = payments.StatusFailed }), "TRY", "orphan_not_paid"},
		"another payment":    {mut(func(r *payments.Result) { r.PaymentID = "9999" }), "TRY", "orphan_mismatch"},
		"no payment id":      {mut(func(r *payments.Result) { r.PaymentID = "" }), "TRY", "orphan_mismatch"},
		"another order":      {mut(func(r *payments.Result) { r.ConversationID = uuid.NewString() }), "TRY", "orphan_mismatch"},
		"foreign basket":     {mut(func(r *payments.Result) { r.BasketID = "B67832" }), "TRY", "orphan_unmatchable"},
		"currency":           {ok, "EUR", "currency_mismatch"},
		"no amount":          {mut(func(r *payments.Result) { r.Price = money.Zero() }), "TRY", "orphan_mismatch"},
		"nil basket uuid":    {mut(func(r *payments.Result) { r.BasketID = uuid.Nil.String() }), "TRY", "orphan_unmatchable"},
		"basket not a uuid ": {mut(func(r *payments.Result) { r.BasketID = "" }), "TRY", "orphan_unmatchable"},
	}
	for name, c := range cases {
		_, _, err := CheckOrphanMatch(facts, c.res, c.currency)
		var se *StateError
		if !errors.As(err, &se) || se.Code != c.code {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
	}
	// An orphan without a payment id (token-only notification) still
	// matches on the order reference.
	if _, _, err := CheckOrphanMatch(OrphanFacts{ConversationID: order.String()}, ok, "TRY"); err != nil {
		t.Fatalf("token-only orphan: %v", err)
	}
}
