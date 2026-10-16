package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/channels/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/channels"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/pkg/money"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// processResult is what one message did.
type processResult struct {
	status      string
	code        string
	message     string
	reservation uuid.UUID
}

// pull reads the account's messages after its cursor, stores and applies
// them, then acknowledges the final results.
func (s *Service) pull(ctx context.Context, orgID, accountID int64, trigger string) (uuid.UUID, error) {
	sess, err := s.begin(ctx, orgID, accountID, model.DirectionPull, trigger)
	if err != nil {
		return uuid.Nil, err
	}
	res := runResult{status: model.RunFailed, details: map[string]any{}}
	defer func() { s.finish(ctx, sess, res) }()
	if !sess.provider.Capabilities().Pull {
		res.err = ErrUnsupported.Error()
		s.savePull(ctx, sess, model.RunFailed, res.err, false, sess.row.PullCursor)
		return sess.runUUID, ErrUnsupported
	}
	cursor := sess.row.PullCursor
	counts := map[string]int{}
	for page := 0; page < maxPullPages; page++ {
		batch, err := sess.provider.PullMessages(ctx, sess.account, cursor)
		if err != nil {
			res.err = err.Error()
			res.details["statuses"] = counts
			s.savePull(ctx, sess, model.RunFailed, res.err, false, cursor)
			return sess.runUUID, err
		}
		for _, m := range batch.Messages {
			r, err := s.receive(ctx, sess, m, model.SourcePull)
			if err != nil {
				res.err = err.Error()
				res.details["statuses"] = counts
				s.savePull(ctx, sess, model.RunFailed, res.err, false, cursor)
				return sess.runUUID, err
			}
			res.total++
			counts[r.status]++
			if r.status == model.MessageFailed {
				res.failed++
			} else {
				res.ok++
			}
		}
		if batch.Cursor != "" {
			cursor = batch.Cursor
		}
		if !batch.More {
			break
		}
	}
	res.details["statuses"] = counts
	res.details["cursor"] = cursor
	acked, ackErr := s.sendAcks(ctx, sess.orgID, sess.row.IntegrationAccountID, sess.account, sess.provider)
	res.details["acknowledged"] = acked
	if ackErr != nil {
		res.details["ack_error"] = ackErr.Error()
	}
	res.status = model.RunSucceeded
	if res.failed > 0 {
		res.status = model.RunPartial
	}
	s.savePull(ctx, sess, res.status, "", true, cursor)
	return sess.runUUID, nil
}

func (s *Service) savePull(ctx context.Context, sess *session, status, errText string, ok bool, cursor string) {
	if err := s.q.SaveChannelPullState(context.WithoutCancel(ctx), db.SaveChannelPullStateParams{
		OrganizationID: sess.orgID, IntegrationAccountID: sess.row.IntegrationAccountID, At: ts(s.clock()), Status: status,
		Error: truncate(errText, 2000), Ok: ok, PullCursor: cursor,
	}); err != nil {
		s.log.Warn("channel_pull_state_failed", "organization_id", sess.orgID, "error", err)
	}
}

func payloadAAD(orgID, accountID int64, key string) string {
	return "channel_messages.payload|org:" + strconv.FormatInt(orgID, 10) + "|account:" + strconv.FormatInt(accountID, 10) + "|key:" + key
}

// maskName keeps the first letter of each word ("D*** K***").
func maskName(first, last string) string {
	out := []string{}
	for _, w := range strings.Fields(first + " " + last) {
		r := []rune(w)
		out = append(out, string(r[0])+"***")
	}
	return strings.Join(out, " ")
}

// summaryOf holds the non-sensitive facts of a message for display.
func summaryOf(m channels.Message) map[string]any {
	extras := []string{}
	for _, x := range m.Extras {
		extras = append(extras, fmt.Sprintf("%s×%d", x.Code, max(x.Quantity, 1)))
	}
	out := map[string]any{
		"status": m.Status, "product_code": m.ProductCode, "pickup_location_code": m.PickupLocationCode,
		"return_location_code": m.ReturnLocationCode, "offer_ref": m.OfferRef, "currency": m.Currency,
		"total": m.Total.String(), "commission": m.Commission.String(), "prepaid": m.Prepaid.String(),
		"collected_by": m.CollectedBy, "customer": maskName(m.Customer.FirstName, m.Customer.LastName), "extras": extras,
	}
	if !m.StartsAt.IsZero() {
		out["starts_at"] = m.StartsAt.UTC()
	}
	if !m.EndsAt.IsZero() {
		out["ends_at"] = m.EndsAt.UTC()
	}
	return out
}

// receive stores a message once and applies it when it is new (or was
// stored but never processed: the process stopped in between). A repeat of
// a processed message only counts a delivery.
func (s *Service) receive(ctx context.Context, sess *session, m channels.Message, source string) (processResult, error) {
	m.ExternalRef = strings.TrimSpace(m.ExternalRef)
	key := strings.TrimSpace(m.ID)
	if key == "" {
		key = fmt.Sprintf("%s:%s:%d", m.Type, m.ExternalRef, m.Version)
	}
	if len(key) > 128 {
		key = key[:128]
	}
	if !model.Contains(model.MessageTypes, m.Type) {
		m.Type = channels.MessageBooking
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return processResult{}, err
	}
	sealed, err := s.sealer.Seal(string(raw), payloadAAD(sess.orgID, sess.row.IntegrationAccountID, key))
	if err != nil {
		return processResult{}, err
	}
	summary, _ := json.Marshal(summaryOf(m))
	var occurred pgtype.Timestamptz
	if !m.OccurredAt.IsZero() {
		occurred = ts(m.OccurredAt.UTC())
	}
	row, err := s.q.InsertChannelMessage(ctx, db.InsertChannelMessageParams{
		OrganizationID: sess.orgID, IntegrationAccountID: sess.row.IntegrationAccountID, Channel: sess.row.Channel,
		MessageKey: key, ExternalRef: truncate(m.ExternalRef, 64), MessageType: m.Type, Version: m.Version,
		OccurredAt: occurred, Source: source, PayloadEnc: sealed, Summary: summary,
	})
	if err != nil {
		return processResult{}, err
	}
	if !row.Inserted && row.Status != model.MessageReceived {
		s.publish(ctx, model.ResourceMessages, "updated", row.Uuid)
		return processResult{status: row.Status}, nil
	}
	r, err := s.process(ctx, sess.orgID, sess.row, row.ID, m)
	if err != nil {
		return r, err
	}
	s.publish(ctx, model.ResourceMessages, "created", row.Uuid)
	return r, nil
}

// process applies one stored message to its channel booking, serialised
// per external booking: a version at or below the applied one changes
// nothing (duplicate / stale); otherwise the booking is reconciled to the
// message's snapshot. A refusal goes to the error queue with its code.
func (s *Service) process(ctx context.Context, orgID int64, st db.GetChannelAccountStateRow, msgID int64, m channels.Message) (processResult, error) {
	if m.ExternalRef == "" || len(m.ExternalRef) > 64 {
		r := processResult{status: model.MessageFailed, code: model.ErrCodeInvalidMessage, message: "the booking number is missing or too long"}
		return r, s.saveMessage(ctx, s.q, orgID, msgID, r, nil)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return processResult{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	q := s.q.WithTx(tx)
	if err := q.ChannelBookingLock(ctx, fmt.Sprintf("channel:%d:%s:%s", orgID, st.Channel, m.ExternalRef)); err != nil {
		return processResult{}, err
	}
	if err := q.InsertChannelReservation(ctx, db.InsertChannelReservationParams{
		OrganizationID: orgID, IntegrationAccountID: st.IntegrationAccountID, Channel: st.Channel, ExternalRef: m.ExternalRef,
	}); err != nil {
		return processResult{}, err
	}
	cr, err := q.GetChannelReservationByExternal(ctx, db.GetChannelReservationByExternalParams{
		OrganizationID: orgID, Channel: st.Channel, ExternalRef: m.ExternalRef,
	})
	if err != nil {
		return processResult{}, err
	}
	if err := q.UpdateChannelReservationReceived(ctx, db.UpdateChannelReservationReceivedParams{
		OrganizationID: orgID, ID: cr.ID, Version: m.Version, At: ts(s.clock()),
	}); err != nil {
		return processResult{}, err
	}
	var r processResult
	if cr.AppliedVersion > 0 && m.Version <= cr.AppliedVersion {
		r = processResult{status: model.MessageStale, message: fmt.Sprintf("version %d is older than the applied version %d", m.Version, cr.AppliedVersion)}
		if m.Version == cr.AppliedVersion {
			r = processResult{status: model.MessageDuplicate, message: "this version was applied already"}
		}
		if cr.ReservationUuid.Valid {
			r.reservation = cr.ReservationUuid.Bytes
		}
		if err := s.saveMessage(ctx, q, orgID, msgID, r, &cr.ID); err != nil {
			return r, err
		}
		return r, tx.Commit(ctx)
	}
	out, applyErr := s.apply(systemContext(ctx, orgID), orgID, st, cr, m)
	if applyErr != nil {
		var be *BookingError
		if !errors.As(applyErr, &be) {
			be = &BookingError{Code: model.ErrCodeInternal, Message: applyErr.Error()}
			s.log.Error("channel_message_failed", "organization_id", orgID, "external_ref", m.ExternalRef, "error", applyErr)
		}
		r = processResult{status: model.MessageFailed, code: be.Code, message: truncate(be.Message, 1000)}
		if err := q.FailChannelReservation(ctx, db.FailChannelReservationParams{OrganizationID: orgID, ID: cr.ID, ErrorCode: be.Code}); err != nil {
			return r, err
		}
		if err := s.saveMessage(ctx, q, orgID, msgID, r, &cr.ID); err != nil {
			return r, err
		}
		return r, tx.Commit(ctx)
	}
	if err := q.ApplyChannelReservation(ctx, out.params(orgID, cr.ID, m)); err != nil {
		return processResult{}, err
	}
	r = processResult{status: model.MessageApplied, reservation: out.reservation}
	if err := s.saveMessage(ctx, q, orgID, msgID, r, &cr.ID); err != nil {
		return r, err
	}
	if err := tx.Commit(ctx); err != nil {
		return r, err
	}
	// Capacity moved: every channel account of the organization pushes.
	s.requestPush(ctx, orgID)
	return r, nil
}

func (s *Service) saveMessage(ctx context.Context, q *db.Queries, orgID, msgID int64, r processResult, crID *int64) error {
	p := db.SaveChannelMessageResultParams{
		OrganizationID: orgID, ID: msgID, Status: r.status, ErrorCode: r.code, ErrorMessage: r.message, At: ts(s.clock()),
	}
	if r.status == model.MessageApplied {
		p.ErrorMessage = ""
	}
	if crID != nil {
		p.ChannelReservationID = int8Of(*crID)
	}
	return q.SaveChannelMessageResult(ctx, p)
}

// applied is the state a message left the channel booking in.
type applied struct {
	reservation   uuid.UUID
	reservationID int64
	customerID    int64
	productID     int64
	status        string
	collectedBy   string
	commission    money.Money
	localTotal    *money.Money
}

func (a applied) params(orgID, id int64, m channels.Message) db.ApplyChannelReservationParams {
	cur := strings.ToUpper(strings.TrimSpace(m.Currency))
	if cur == "" {
		cur = "TRY"
	}
	p := db.ApplyChannelReservationParams{
		OrganizationID: orgID, ID: id, ReservationID: int8Of(a.reservationID), CustomerID: int8Of(a.customerID),
		Status: a.status, ChannelStatus: m.Status, AppliedVersion: m.Version, ProductCode: truncate(m.ProductCode, 64),
		PickupCode: truncate(m.PickupLocationCode, 64), ReturnCode: truncate(m.ReturnLocationCode, 64),
		OfferRef: truncate(m.OfferRef, 64), RentalProductID: int8Of(a.productID), StartsAt: tsPtr(&m.StartsAt),
		EndsAt: tsPtr(&m.EndsAt), CollectedBy: a.collectedBy, Currency: cur, GrossAmount: m.Total.Numeric(),
		CommissionAmount: a.commission.Numeric(), NetAmount: m.Total.Sub(a.commission).Numeric(),
		PrepaidAmount: m.Prepaid.Numeric(),
	}
	if a.localTotal != nil {
		p.LocalTotal = a.localTotal.Numeric()
		if m.Total.IsPositive() {
			p.PriceDifference = m.Total.Sub(*a.localTotal).Numeric()
		}
	}
	return p
}

// terminalStatuses are reservation statuses that hold no capacity.
var terminalStatuses = map[string]bool{"cancelled": true, "expired": true, "no_show": true, "completed": true}

// apply reconciles the channel booking with the message snapshot through
// the Booker: cancel, create, or change (dates / places / product /
// extras) and record the commission.
func (s *Service) apply(ctx context.Context, orgID int64, st db.GetChannelAccountStateRow, cr db.GetChannelReservationByExternalRow, m channels.Message) (applied, error) {
	out := applied{collectedBy: m.CollectedBy, productID: cr.RentalProductID.Int64, customerID: cr.CustomerID.Int64}
	if out.collectedBy != channels.CollectedByChannel && out.collectedBy != channels.CollectedBySupplier {
		out.collectedBy = st.CollectedBy
	}
	out.commission = m.Commission
	if out.commission.IsZero() && m.Total.IsPositive() {
		if rate, err := money.RateFromNumeric(st.CommissionRate); err == nil && rate > 0 {
			out.commission = m.Total.MulRate(rate)
		}
	}
	if cr.ReservationID.Valid {
		out.reservationID = cr.ReservationID.Int64
		out.reservation = cr.ReservationUuid.Bytes
	}
	if m.Status == channels.StatusCancelled {
		out.status = model.BookingCancelled
		if cr.ReservationID.Valid && !terminalStatuses[textOr(cr.ReservationStatus)] {
			if _, err := s.booker.Cancel(ctx, orgID, out.reservation, "Kanal iptali ("+st.Channel+" "+m.ExternalRef+")"); err != nil {
				return out, err
			}
		}
		return out, nil
	}
	if m.Status != channels.StatusConfirmed {
		return out, &BookingError{Code: model.ErrCodeInvalidMessage, Message: "unknown booking status " + m.Status}
	}
	if m.StartsAt.IsZero() || !m.EndsAt.After(m.StartsAt) {
		return out, &BookingError{Code: model.ErrCodeInvalidMessage, Message: "the booking needs a pick-up and a later return time"}
	}
	spec, productID, err := s.resolve(ctx, orgID, st, m)
	if err != nil {
		return out, err
	}
	spec.Commission, spec.CollectedBy = out.commission, out.collectedBy
	out.productID = productID
	if cr.ReservationID.Valid && terminalStatuses[textOr(cr.ReservationStatus)] {
		return out, &BookingError{Code: model.ErrCodeReinstate, Message: "the reservation of this booking is " +
			textOr(cr.ReservationStatus) + "; a confirmed message cannot reopen it (contact the channel)"}
	}
	var booking Booking
	if !cr.ReservationID.Valid {
		var customer uuid.UUID
		if cr.CustomerUuid.Valid {
			customer = cr.CustomerUuid.Bytes
		} else {
			c, err := s.booker.CreateCustomer(ctx, orgID, m.Customer, st.Channel, m.ExternalRef)
			if err != nil {
				return out, err
			}
			customer = c
			id, err := s.q.ChannelCustomerIDByUUID(ctx, db.ChannelCustomerIDByUUIDParams{OrganizationID: orgID, Uuid: c})
			if err != nil {
				return out, err
			}
			out.customerID = id
		}
		spec.CustomerUUID = customer
		booking, err = s.booker.Book(ctx, orgID, spec)
		var be *BookingError
		if errors.As(err, &be) && be.Taken {
			// Created by an earlier attempt whose link was not stored.
			existing, lerr := s.q.ChannelReservationByExternalReference(ctx, db.ChannelReservationByExternalReferenceParams{
				OrganizationID: orgID, Channel: st.Channel, ExternalReference: pgtype.Text{String: m.ExternalRef, Valid: true},
			})
			if lerr != nil {
				return out, err
			}
			out.reservation, out.reservationID = existing.Uuid, existing.ID
			booking, err = s.change(ctx, orgID, existing.Uuid, spec)
		}
		if err != nil {
			return out, err
		}
		if out.reservationID == 0 {
			existing, err := s.q.ChannelReservationByExternalReference(ctx, db.ChannelReservationByExternalReferenceParams{
				OrganizationID: orgID, Channel: st.Channel, ExternalReference: pgtype.Text{String: m.ExternalRef, Valid: true},
			})
			if err != nil {
				return out, err
			}
			out.reservationID = existing.ID
		}
		out.reservation = booking.UUID
	} else {
		booking, err = s.change(ctx, orgID, out.reservation, spec)
		if err != nil {
			return out, err
		}
	}
	if out.collectedBy == channels.CollectedByChannel && !booking.Commission.Equal(out.commission) {
		if err := s.booker.SetCommission(ctx, orgID, booking.UUID, out.commission); err != nil {
			return out, err
		}
	}
	total := booking.Total
	out.localTotal = &total
	out.status = model.BookingActive
	return out, nil
}

// change applies the spec to an existing reservation when anything the
// channel controls differs (dates, places, product, extras).
func (s *Service) change(ctx context.Context, orgID int64, id uuid.UUID, spec BookingSpec) (Booking, error) {
	cur, err := s.booker.Get(ctx, orgID, id)
	if err != nil {
		return Booking{}, err
	}
	if sameBooking(cur, spec) {
		return cur, nil
	}
	return s.booker.Change(ctx, orgID, id, spec)
}

func sameBooking(b Booking, spec BookingSpec) bool {
	if b.ProductUUID != spec.ProductUUID || b.PickupBranch != spec.PickupBranch || b.ReturnBranch != spec.ReturnBranch ||
		!b.StartsAt.Equal(spec.StartsAt.Truncate(time.Minute)) || !b.EndsAt.Equal(spec.EndsAt.Truncate(time.Minute)) {
		return false
	}
	want := map[uuid.UUID]int{}
	for _, x := range spec.Extras {
		want[x.ExtraUUID] += max(x.Quantity, 1)
	}
	have := map[uuid.UUID]int{}
	for _, x := range b.Extras {
		have[x.ExtraUUID] += max(x.Quantity, 1)
	}
	if len(want) != len(have) {
		return false
	}
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

// resolve maps the external codes of a message to internal records: the
// locations by branch mapping, the product by the offer reference (the
// pushed item it echoes) or by the product mapping of the code at the
// pick-up location; several products on one code are resolved from the
// offer pushed for that day, never from the group name.
func (s *Service) resolve(ctx context.Context, orgID int64, st db.GetChannelAccountStateRow, m channels.Message) (BookingSpec, int64, error) {
	rows, err := s.q.ListActiveChannelMappings(ctx, db.ListActiveChannelMappingsParams{OrganizationID: orgID, IntegrationAccountID: st.IntegrationAccountID})
	if err != nil {
		return BookingSpec{}, 0, err
	}
	branches := map[string]db.ListActiveChannelMappingsRow{}
	extras := map[string]db.ListActiveChannelMappingsRow{}
	for _, r := range rows {
		switch r.EntityType {
		case model.EntityBranch:
			branches[r.ExternalCode] = r
		case model.EntityExtra:
			extras[r.ExternalCode] = r
		}
	}
	pickup, ok := branches[strings.TrimSpace(m.PickupLocationCode)]
	if !ok {
		return BookingSpec{}, 0, &BookingError{Code: model.ErrCodeUnknownLocation, Message: "no branch is mapped to location " + m.PickupLocationCode}
	}
	ret := pickup
	if code := strings.TrimSpace(m.ReturnLocationCode); code != "" && code != m.PickupLocationCode {
		if ret, ok = branches[code]; !ok {
			return BookingSpec{}, 0, &BookingError{Code: model.ErrCodeUnknownLocation, Message: "no branch is mapped to return location " + code}
		}
	}
	spec := BookingSpec{
		Channel: st.Channel, ExternalRef: m.ExternalRef, PickupBranch: pickup.BranchUuid.Bytes, ReturnBranch: ret.BranchUuid.Bytes,
		StartsAt: m.StartsAt.UTC(), EndsAt: m.EndsAt.UTC(), Extras: []ExtraRequest{}, Note: bookingNote(st.Channel, m),
	}
	var productID int64
	if ref := strings.TrimSpace(m.OfferRef); ref != "" {
		offer, err := s.q.GetChannelOffer(ctx, db.GetChannelOfferParams{OrganizationID: orgID, IntegrationAccountID: st.IntegrationAccountID, OfferRef: ref})
		switch {
		case err == nil && offer.BranchID == pickup.BranchID.Int64:
			spec.ProductUUID, productID = offer.ProductUuid, offer.RentalProductID
		case err == nil:
			return BookingSpec{}, 0, &BookingError{Code: model.ErrCodeUnknownProduct, Message: "the offer " + ref + " belongs to another location"}
		case !notFound(err):
			return BookingSpec{}, 0, err
		}
	}
	if productID == 0 {
		candidates := []db.ListActiveChannelMappingsRow{}
		for _, r := range rows {
			if r.EntityType == model.EntityProduct && r.ExternalCode == strings.TrimSpace(m.ProductCode) &&
				(!r.BranchID.Valid || r.BranchID.Int64 == pickup.BranchID.Int64) {
				candidates = append(candidates, r)
			}
		}
		switch len(candidates) {
		case 0:
			return BookingSpec{}, 0, &BookingError{Code: model.ErrCodeUnknownProduct, Message: "no product is mapped to code " + m.ProductCode + " at " + m.PickupLocationCode}
		case 1:
			spec.ProductUUID, productID = candidates[0].ProductUuid.Bytes, candidates[0].RentalProductID.Int64
		default:
			day := m.StartsAt.In(istanbul)
			offer, err := s.q.LatestChannelOffer(ctx, db.LatestChannelOfferParams{
				OrganizationID: orgID, IntegrationAccountID: st.IntegrationAccountID, ExternalCode: m.ProductCode,
				LocationCode: m.PickupLocationCode,
				StayDate:     pgtype.Date{Time: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), Valid: true},
			})
			if notFound(err) {
				return BookingSpec{}, 0, &BookingError{Code: model.ErrCodeAmbiguous, Message: "several products share code " + m.ProductCode +
					" and no offer was pushed for that day; the booking must carry its offer reference"}
			}
			if err != nil {
				return BookingSpec{}, 0, err
			}
			spec.ProductUUID, productID = offer.ProductUuid, offer.RentalProductID
		}
	}
	for _, x := range m.Extras {
		e, ok := extras[strings.TrimSpace(x.Code)]
		if !ok {
			return BookingSpec{}, 0, &BookingError{Code: model.ErrCodeUnknownExtra, Message: "no extra is mapped to code " + x.Code}
		}
		spec.Extras = append(spec.Extras, ExtraRequest{ExtraUUID: e.ExtraUuid.Bytes, Quantity: max(x.Quantity, 1)})
	}
	return spec, productID, nil
}

func bookingNote(channel string, m channels.Message) string {
	parts := []string{channel + " " + m.ExternalRef}
	if m.FlightNumber != "" {
		parts = append(parts, "Uçuş: "+m.FlightNumber)
	}
	if n := strings.TrimSpace(m.Note); n != "" {
		parts = append(parts, n)
	}
	return truncate(strings.Join(parts, " · "), 2000)
}

// sendAcks tells the channel the final results it has not heard yet.
// Adapters without acknowledgements mark them as told.
func (s *Service) sendAcks(ctx context.Context, orgID, accountID int64, account integrations.Account, p channels.Provider) (int, error) {
	rows, err := s.q.ListChannelMessagesToAck(ctx, db.ListChannelMessagesToAckParams{OrganizationID: orgID, IntegrationAccountID: accountID})
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	now := s.clock()
	mark := func(id int64, ackErr string) {
		p := db.SaveChannelMessageAckParams{OrganizationID: orgID, ID: id, AckError: truncate(ackErr, 1000)}
		if ackErr == "" {
			p.AckedAt = ts(now)
		}
		_ = s.q.SaveChannelMessageAck(context.WithoutCancel(ctx), p)
	}
	if !p.Capabilities().Acknowledge {
		for _, r := range rows {
			mark(r.ID, "")
		}
		return len(rows), nil
	}
	acks := make([]channels.Ack, 0, len(rows))
	for _, r := range rows {
		a := channels.Ack{MessageID: r.MessageKey, ExternalRef: r.ExternalRef, Accepted: r.Status != model.MessageIgnored,
			Reference: textOr(r.ReservationReference)}
		if r.Status == model.MessageIgnored {
			a.Code, a.Message = r.ErrorCode, strings.TrimSpace(r.ResolutionNote)
			if a.Message == "" {
				a.Message = r.ErrorMessage
			}
		}
		acks = append(acks, a)
	}
	if err := p.Acknowledge(ctx, account, acks); err != nil {
		if errors.Is(err, integrations.ErrUnsupported) {
			for _, r := range rows {
				mark(r.ID, "")
			}
			return len(rows), nil
		}
		for _, r := range rows {
			mark(r.ID, err.Error())
		}
		return 0, err
	}
	for _, r := range rows {
		mark(r.ID, "")
	}
	return len(rows), nil
}

// openMessage decrypts a stored message.
func (s *Service) openMessage(orgID int64, row db.GetChannelMessageForProcessingRow) (channels.Message, error) {
	plain, err := s.sealer.Open(row.PayloadEnc, payloadAAD(orgID, row.IntegrationAccountID, row.MessageKey))
	if err != nil {
		return channels.Message{}, err
	}
	var m channels.Message
	if err := json.Unmarshal([]byte(plain), &m); err != nil {
		return channels.Message{}, err
	}
	return m, nil
}

// RetryMessage applies a failed message again (after a mapping or capacity
// fix). A newer version applied meanwhile makes it stale.
func (s *Service) RetryMessage(ctx context.Context, orgID int64, id uuid.UUID) (model.MessageDetail, error) {
	if err := requireAllBranches(ctx); err != nil {
		return model.MessageDetail{}, err
	}
	ref, err := s.q.GetChannelMessageIDByUUID(ctx, db.GetChannelMessageIDByUUIDParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.MessageDetail{}, ErrMessageNotFound
	}
	if err != nil {
		return model.MessageDetail{}, err
	}
	row, err := s.q.GetChannelMessageForProcessing(ctx, db.GetChannelMessageForProcessingParams{OrganizationID: orgID, ID: ref.ID})
	if err != nil {
		return model.MessageDetail{}, err
	}
	if row.Status != model.MessageFailed {
		return model.MessageDetail{}, ErrNotRetryable
	}
	st, err := s.q.GetChannelAccountState(ctx, db.GetChannelAccountStateParams{OrganizationID: orgID, IntegrationAccountID: row.IntegrationAccountID})
	if err != nil {
		return model.MessageDetail{}, err
	}
	m, err := s.openMessage(orgID, row)
	if err != nil {
		return model.MessageDetail{}, err
	}
	if _, err := s.process(ctx, orgID, st, row.ID, m); err != nil {
		return model.MessageDetail{}, err
	}
	s.ackAfterAction(ctx, orgID, row.IntegrationAccountID)
	s.publish(ctx, model.ResourceMessages, "updated", id)
	return s.GetMessage(ctx, orgID, id)
}

// IgnoreMessage dismisses a failed message with a note; the channel is told
// it was refused.
func (s *Service) IgnoreMessage(ctx context.Context, orgID int64, id uuid.UUID, note string) (model.MessageDetail, error) {
	if err := requireAllBranches(ctx); err != nil {
		return model.MessageDetail{}, err
	}
	note = strings.TrimSpace(note)
	if len(note) < 3 || len(note) > 500 {
		return model.MessageDetail{}, validation("note", "required", "a note is required (3-500 characters)")
	}
	ref, err := s.q.GetChannelMessageIDByUUID(ctx, db.GetChannelMessageIDByUUIDParams{OrganizationID: orgID, Uuid: id})
	if notFound(err) {
		return model.MessageDetail{}, ErrMessageNotFound
	}
	if err != nil {
		return model.MessageDetail{}, err
	}
	n, err := s.q.IgnoreChannelMessage(ctx, db.IgnoreChannelMessageParams{
		OrganizationID: orgID, ID: ref.ID, ResolvedBy: actorID(ctx), At: ts(s.clock()), Note: note,
	})
	if err != nil {
		return model.MessageDetail{}, err
	}
	if n != 1 {
		return model.MessageDetail{}, ErrNotRetryable
	}
	s.ackAfterAction(ctx, orgID, ref.IntegrationAccountID)
	s.publish(ctx, model.ResourceMessages, "updated", id)
	return s.GetMessage(ctx, orgID, id)
}

// ackAfterAction acknowledges results after a staff action (best effort;
// the next pull retries).
func (s *Service) ackAfterAction(ctx context.Context, orgID, accountID int64) {
	account, err := s.accounts.AccountByID(ctx, orgID, accountID)
	if err != nil {
		return
	}
	p, ok := s.providers[account.Provider]
	if !ok {
		return
	}
	if _, err := s.sendAcks(ctx, orgID, accountID, account, p); err != nil {
		s.log.Warn("channel_ack_failed", "organization_id", orgID, "error", err)
	}
}

// Simulate queues a booking message on a development adapter and pulls at
// once (the panel's "simulate booking").
func (s *Service) Simulate(ctx context.Context, orgID int64, id uuid.UUID, in model.SimulateInput) (model.SyncRun, error) {
	r, err := s.accountForAction(ctx, orgID, id)
	if err != nil {
		return model.SyncRun{}, err
	}
	sim, ok := s.providers[r.Provider].(channels.Simulator)
	if !ok {
		return model.SyncRun{}, ErrNotSimulator
	}
	m, err := simulated(in)
	if err != nil {
		return model.SyncRun{}, err
	}
	account, err := s.accounts.AccountByID(ctx, orgID, r.ID)
	if err != nil {
		return model.SyncRun{}, err
	}
	sim.Simulate(account, m)
	return s.PullNow(ctx, orgID, id)
}

func simulated(in model.SimulateInput) (channels.Message, error) {
	ve := &ValidationError{}
	m := channels.Message{
		Type: strings.TrimSpace(in.Type), ExternalRef: strings.TrimSpace(in.ExternalRef), ProductCode: strings.TrimSpace(in.ProductCode),
		PickupLocationCode: strings.TrimSpace(in.PickupLocationCode), ReturnLocationCode: strings.TrimSpace(in.ReturnLocationCode),
		OfferRef: strings.TrimSpace(in.OfferRef), Customer: in.Customer, Extras: in.Extras, Currency: "TRY",
		CollectedBy: strings.TrimSpace(in.CollectedBy), Total: money.Zero(), Commission: money.Zero(), Prepaid: money.Zero(),
	}
	if m.Type == "" {
		m.Type = channels.MessageBooking
	}
	if !model.Contains(model.MessageTypes, m.Type) {
		ve.add("type", "invalid", "type must be booking, modification or cancellation")
	}
	if m.Type != channels.MessageBooking && m.ExternalRef == "" {
		ve.add("external_ref", "required", "a modification or cancellation names its booking")
	}
	if len(m.ExternalRef) > 64 {
		ve.add("external_ref", "too_long", "external_ref is at most 64 characters")
	}
	if m.Type != channels.MessageCancellation {
		if m.ProductCode == "" {
			ve.add("product_code", "required", "product_code is required")
		}
		if m.PickupLocationCode == "" {
			ve.add("pickup_location_code", "required", "pickup_location_code is required")
		}
		if in.StartsAt == nil || in.EndsAt == nil || !in.EndsAt.After(*in.StartsAt) {
			ve.add("ends_at", "invalid", "starts_at and a later ends_at are required")
		}
	}
	if in.StartsAt != nil {
		m.StartsAt = in.StartsAt.UTC()
	}
	if in.EndsAt != nil {
		m.EndsAt = in.EndsAt.UTC()
	}
	for field, raw := range map[string]string{"total": in.Total, "commission": in.Commission} {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		v, err := money.ParseRound(strings.TrimSpace(raw))
		if err != nil || v.IsNegative() {
			ve.add(field, "invalid", field+" must be a non-negative amount")
			continue
		}
		if field == "total" {
			m.Total = v
		} else {
			m.Commission = v
		}
	}
	if m.CollectedBy != "" && m.CollectedBy != channels.CollectedByChannel && m.CollectedBy != channels.CollectedBySupplier {
		ve.add("collected_by", "invalid", "collected_by must be channel or us")
	}
	if m.CollectedBy == channels.CollectedByChannel {
		m.Prepaid = m.Total
	}
	if err := ve.orNil(); err != nil {
		return m, err
	}
	m.Status = channels.StatusConfirmed
	if m.Type == channels.MessageCancellation {
		m.Status = channels.StatusCancelled
	}
	return m, nil
}
