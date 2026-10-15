package model

import "github.com/google/uuid"

// AcceptInput is the body of POST …/{uuid}/accept (staff approval queue,
// TEC-708). QuoteUUID accepts a re-priced request at the price of a change
// quote (the answer to QUOTE_PRICE_CHANGED); SendPaymentLink creates a
// pay-by-link link for the prepayment due, valid until the payment window
// ends (needs tenant.payments.create_link and an active payment account).
type AcceptInput struct {
	QuoteUUID       *uuid.UUID `json:"quote_uuid"`
	SendPaymentLink bool       `json:"send_payment_link"`
	Note            string     `json:"note"`
}

// AcceptResult is the answer of an accept: the reservation and the
// payment link created with it (or why none was).
type AcceptResult struct {
	Reservation      Reservation     `json:"reservation"`
	PaymentLink      *PaymentLinkRef `json:"payment_link"`
	PaymentLinkError string          `json:"payment_link_error,omitempty"`
	Warnings         []string        `json:"warnings"`
}
