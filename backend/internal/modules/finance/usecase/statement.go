package usecase

import (
	"context"
	"strings"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/model"
	paymodel "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/model"
	smsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sms/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/rbac"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
)

// ErrMessagingUnavailable: no SMS / pay-link plumbing is wired.
var ErrMessagingUnavailable = stateErrValue("messaging_unavailable", "statement messages are not available")

func stateErrValue(code, msg string) *StateError { return &StateError{Code: code, Message: msg} }

// FormatTRY renders an amount the Turkish way: 12.345,50.
func FormatTRY(v money.Money) string {
	s := v.String() // 12345.50
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	out := b.String() + "," + frac
	if neg {
		out = "-" + out
	}
	return out
}

// SendStatement texts the customer the balance of their account and, when
// asked (tenant.payments.create_link), a pay link: the existing pay-by-link
// of a reservation with an open balance (the chosen rental's, else the one
// with the latest open balance), for at most the account balance.
func (s *Service) SendStatement(ctx context.Context, orgID int64, id uuid.UUID, in model.StatementSendInput) (model.StatementSent, error) {
	if s.sms == nil {
		return model.StatementSent{}, ErrMessagingUnavailable
	}
	row, acc, err := s.account(ctx, orgID, id)
	if err != nil {
		return model.StatementSent{}, err
	}
	if !row.CustomerID.Valid {
		return model.StatementSent{}, stateErr("not_customer", "statements are sent to customer accounts")
	}
	c, err := s.q.GetCustomerDisplay(ctx, db.GetCustomerDisplayParams{OrganizationID: orgID, ID: row.CustomerID.Int64})
	if err != nil {
		return model.StatementSent{}, err
	}
	if strings.TrimSpace(c.Phone) == "" || c.AnonymizedAt.Valid {
		return model.StatementSent{}, validation("phone", "missing", "the customer has no phone number")
	}
	balance := acc.Balance.Balance
	out := model.StatementSent{Balance: balance}
	body := []string{"Sayın " + customerName(c.CustomerType, c.FirstName, c.LastName, c.CompanyTitle, false) + ","}
	body = append(body, "cari hesap özetiniz ("+row.Code+"): bakiye "+FormatTRY(balance)+" "+row.Currency+".")
	if acc.Balance.Overdue.IsPositive() {
		body = append(body, "Vadesi geçen: "+FormatTRY(acc.Balance.Overdue)+" "+row.Currency+".")
	}
	redacted := strings.Join(body, " ")
	if in.PayLink {
		if !has(ctx, rbac.PermTenantPaymentsCreateLink) {
			return model.StatementSent{}, ErrForbidden
		}
		if s.links == nil {
			return model.StatementSent{}, ErrMessagingUnavailable
		}
		if !balance.IsPositive() {
			return model.StatementSent{}, stateErr("nothing_due", "the account has nothing due")
		}
		link, err := s.statementLink(ctx, orgID, row, balance, in.RentalUUID)
		if err != nil {
			return model.StatementSent{}, err
		}
		out.PayLinkURL = link.URL
		out.PayLinkUUID = &link.UUID
		out.PayLinkAmount = &link.Amount
		out.PayLinkExpiresAt = &link.ExpiresAt
		body = append(body, "Ödeme ("+FormatTRY(link.Amount)+" "+link.Currency+"): "+link.URL)
		redacted += " Ödeme (" + FormatTRY(link.Amount) + " " + link.Currency + "): [ödeme bağlantısı]"
	}
	text := strings.Join(body, " ")
	send := smsusecase.SendInput{
		Class: sms.ClassTransactional, Purpose: "finance.statement", Phone: c.Phone, Body: text,
		CustomerID: &c.ID, RefType: "finance_account", RefUUID: &row.Uuid,
	}
	if in.PayLink {
		send.RedactedBody = redacted
	}
	sent, err := s.sms.Send(ctx, orgID, send)
	if err != nil {
		return model.StatementSent{}, err
	}
	out.PhoneMasked, out.MessageUUID = sent.PhoneMasked, sent.MessageUUID
	return out, nil
}

// statementLink creates the pay link of a statement.
func (s *Service) statementLink(ctx context.Context, orgID int64, acc db.GetAccountRowRow, balance money.Money, rental *uuid.UUID) (paymodel.Link, error) {
	open, err := s.q.OpenReservationBalances(ctx, db.OpenReservationBalancesParams{OrganizationID: orgID, AccountID: acc.ID})
	if err != nil {
		return paymodel.Link{}, err
	}
	var target *db.OpenReservationBalancesRow
	if rental != nil {
		r, err := s.rentalRef(ctx, orgID, *rental)
		if err != nil {
			return paymodel.Link{}, validation("rental_uuid", "not_found", "unknown rental")
		}
		for i := range open {
			if open[i].Reference == r.Reference {
				target = &open[i]
			}
		}
		if target == nil {
			return paymodel.Link{}, validation("rental_uuid", "nothing_due", "the rental has no open balance on this account")
		}
	} else {
		for i := range open {
			if open[i].Status == "confirmed" {
				target = &open[i]
				break
			}
		}
	}
	if target == nil {
		return paymodel.Link{}, stateErr("no_payable_reservation", "no reservation of the account can take a pay link")
	}
	amount := m(target.Balance).Min(balance)
	link, err := s.links.CreateLink(ctx, orgID, target.Uuid, paymodel.CreateLinkInput{Amount: &amount})
	if err != nil {
		// The reservation's own remaining balance may be lower than its
		// ledger balance (charges are billed on the account): let the
		// payments module pick its default amount.
		var lerr error
		link, lerr = s.links.CreateLink(ctx, orgID, target.Uuid, paymodel.CreateLinkInput{})
		if lerr != nil {
			return paymodel.Link{}, err
		}
	}
	return link, nil
}
